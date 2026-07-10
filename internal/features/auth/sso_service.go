package auth

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/security"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
	"github.com/sidarth-23/dinchy/internal/platform/transform"
)

func (s *Service) listSSOProviders(_ context.Context) ([]SSOProviderOut, error) {
	out := make([]SSOProviderOut, 0, len(s.sso.envProviders))
	for _, provider := range config.SupportedSSOProviders() {
		providerConfig, ok := s.sso.envProviders[string(provider.ID)]
		if !ok || !providerConfig.Enabled {
			continue
		}
		out = append(out, SSOProviderOut{ID: string(providerConfig.ID), Name: providerConfig.Name})
	}
	return out, nil
}

func (s *Service) effectiveSSOProviderConfig(providerID string) (config.SSOProviderConfig, bool) {
	providerConfig, ok := s.sso.envProviders[providerID]
	return providerConfig, ok
}

func (s *Service) startSSO(ctx context.Context, providerID, returnTo, organisationSlug string) (string, []http.Cookie, error) {
	providerConfig, ok := s.effectiveSSOProviderConfig(providerID)
	if !ok || !providerConfig.Enabled {
		return "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOProviderNotFound, i18n.P("provider", providerID)))
	}
	if s.redis == nil {
		return "", nil, apperrors.Internal(i18n.Msg(i18n.CodeAuthSSOCacheRequired))
	}
	provider, err := newGothProviderForSSO(providerConfig)
	if err != nil {
		return "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOStart))
	}
	stateToken, err := security.RandomToken(32)
	if err != nil {
		return "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageGenerateToken))
	}
	session, err := provider.BeginAuth(stateToken)
	if err != nil {
		return "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOStart))
	}
	authURL, err := session.GetAuthURL()
	if err != nil {
		return "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOStart))
	}
	transactionID, err := security.RandomToken(32)
	if err != nil {
		return "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageGenerateToken))
	}
	cacheState := ssoCacheState{ProviderID: providerID, ReturnTo: internalReturnPath(returnTo), OrganisationSlug: organisationSlug, State: stateToken, ProviderSession: session.Marshal()}
	if err := s.redis.Set(ctx, s.sso.cacheKey(transactionID), cacheState, s.sso.stateLifetime).Err(); err != nil {
		return "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOStart))
	}
	return authURL, []http.Cookie{{Name: s.sso.stateCookieName, Value: transactionID, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: int(s.sso.stateLifetime.Seconds()), Expires: s.Clock().Now().Add(s.sso.stateLifetime)}}, nil
}

func (s *Service) completeSSO(ctx context.Context, providerID, queryState, code, transactionID, ip, userAgent string) (string, string, []http.Cookie, error) {
	providerConfig, ok := s.effectiveSSOProviderConfig(providerID)
	if !ok || !providerConfig.Enabled {
		return "", "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOProviderNotFound, i18n.P("provider", providerID)))
	}
	if s.redis == nil || transactionID == "" {
		return "", "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOInvalidState))
	}
	var cached ssoCacheState
	if err := s.redis.Get(ctx, s.sso.cacheKey(transactionID)).Scan(&cached); err != nil {
		return "", "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOInvalidState))
	}
	if cached.ProviderID != providerID || cached.State != queryState {
		return "", "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOInvalidState))
	}
	provider, err := newGothProviderForSSO(providerConfig)
	if err != nil {
		return "", "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOCallback))
	}
	session, err := provider.UnmarshalSession(cached.ProviderSession)
	if err != nil {
		return "", "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOCallback))
	}
	if err := validateSSOState(session, queryState); err != nil {
		return "", "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOInvalidState))
	}
	if _, err := session.Authorize(provider, url.Values{"code": []string{code}, "state": []string{queryState}}); err != nil {
		return "", "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOCallback))
	}
	gothUser, err := provider.FetchUser(session)
	if err != nil {
		return "", "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOCallback))
	}
	userRow, err := s.store.FindUserByProviderAccount(ctx, sqlcgen.FindUserByProviderAccountParams{Provider: providerID, ProviderAccountID: gothUser.UserID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageFindUser))
	}
	var user *User
	if userRow.ID != uuid.Nil {
		user = &User{ID: userRow.ID.String(), Email: userRow.Email, DisplayName: userRow.DisplayName, EmailVerified: userRow.EmailVerifiedAt.Valid}
	}
	if user == nil && gothUser.Email != "" {
		providerEmail := gothUser.Email
		transform.ApplyTo(transform.SpecEmail, &providerEmail)
		emailRow, emailErr := s.store.FindUserByEmail(ctx, providerEmail)
		if emailErr != nil {
			if errors.Is(emailErr, pgx.ErrNoRows) {
				return "", "", nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthSSOLoginFailed))
			}
			return "", "", nil, apperrors.Annotate(emailErr, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageFindUser))
		}
		user = userFromFindUserRow(emailRow)
		if user == nil || !user.EmailVerified {
			return "", "", nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthSSOLoginFailed))
		}
		if err := s.store.InsertAccount(ctx, sqlcgen.InsertAccountParams{ID: id.MustParse(s.IDGenerator().New()), UserID: id.MustParse(user.ID), Provider: providerID, ProviderAccountID: gothUser.UserID, CreatedAt: sqltype.Timestamptz(s.Clock().Now()), UpdatedAt: sqltype.Timestamptz(s.Clock().Now())}); err != nil {
			return "", "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageFindAccount))
		}
	}
	if user == nil {
		return "", "", nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthSSOLoginFailed))
	}
	organization, err := s.resolveLoginOrganisation(ctx, user.ID, cached.OrganisationSlug)
	if err != nil {
		return "", "", nil, err
	}
	token, err := s.sessions.Create(ctx, user.ID, organization.ID, ip, userAgent)
	if err != nil {
		return "", "", nil, err
	}
	if err := s.redis.Del(ctx, s.sso.cacheKey(transactionID)).Err(); err != nil {
		return "", "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOCallback))
	}
	return cached.ReturnTo, token, s.clearSSOCookies(), nil
}
