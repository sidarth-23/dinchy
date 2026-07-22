package auth

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
	"github.com/sidarth-23/dinchy/internal/foundation/id"
	"github.com/sidarth-23/dinchy/internal/foundation/security"
	"github.com/sidarth-23/dinchy/internal/foundation/transform"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
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

func (s *Service) startSSO(ctx context.Context, providerID, returnTo, organizationSlug string) (string, []http.Cookie, error) {
	providerConfig, ok := s.effectiveSSOProviderConfig(providerID)
	if !ok || !providerConfig.Enabled {
		return "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAccountAuthSSOProviderNotFound, i18n.P("provider", providerID)))
	}
	redisClient := s.RedisClient
	if redisClient == nil {
		return "", nil, apperrors.Internal(i18n.Msg(i18n.CodeAccountAuthSSOCacheRequired))
	}
	provider, err := newGothProviderForSSO(providerConfig)
	if err != nil {
		return "", nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLoginSSOStart), apperrors.WithCause(err))
	}
	stateToken, err := security.RandomToken(32)
	if err != nil {
		return "", nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLoginGenerateToken), apperrors.WithCause(err))
	}
	session, err := provider.BeginAuth(stateToken)
	if err != nil {
		return "", nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLoginSSOStart), apperrors.WithCause(err))
	}
	authURL, err := session.GetAuthURL()
	if err != nil {
		return "", nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLoginSSOStart), apperrors.WithCause(err))
	}
	transactionID, err := security.RandomToken(32)
	if err != nil {
		return "", nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLoginGenerateToken), apperrors.WithCause(err))
	}
	cacheState := ssoCacheState{ProviderID: providerID, ReturnTo: internalReturnPath(returnTo), OrganizationSlug: organizationSlug, State: stateToken, ProviderSession: session.Marshal()}
	if err := redisClient.Set(ctx, s.sso.cacheKey(transactionID), cacheState, s.sso.stateLifetime).Err(); err != nil {
		return "", nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLoginSSOStart), apperrors.WithCause(err))
	}
	return authURL, []http.Cookie{{Name: s.sso.stateCookieName, Value: transactionID, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: int(s.sso.stateLifetime.Seconds()), Expires: s.Clock.Now().Add(s.sso.stateLifetime)}}, nil
}

func (s *Service) completeSSO(ctx context.Context, providerID, queryState, code, transactionID, ip, userAgent string) (string, string, []http.Cookie, error) {
	providerConfig, ok := s.effectiveSSOProviderConfig(providerID)
	if !ok || !providerConfig.Enabled {
		return "", "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAccountAuthSSOProviderNotFound, i18n.P("provider", providerID)))
	}
	redisClient := s.RedisClient
	if redisClient == nil || transactionID == "" {
		return "", "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAccountAuthSSOInvalidState))
	}
	var cached ssoCacheState
	if err := redisClient.Get(ctx, s.sso.cacheKey(transactionID)).Scan(&cached); err != nil {
		return "", "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAccountAuthSSOInvalidState))
	}
	if cached.ProviderID != providerID || cached.State != queryState {
		return "", "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAccountAuthSSOInvalidState))
	}
	provider, err := newGothProviderForSSO(providerConfig)
	if err != nil {
		return "", "", nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLoginSSOCallback), apperrors.WithCause(err))
	}
	session, err := provider.UnmarshalSession(cached.ProviderSession)
	if err != nil {
		return "", "", nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLoginSSOCallback), apperrors.WithCause(err))
	}
	if err := validateSSOState(session, queryState); err != nil {
		return "", "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAccountAuthSSOInvalidState))
	}
	if _, err := session.Authorize(provider, url.Values{"code": []string{code}, "state": []string{queryState}}); err != nil {
		return "", "", nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLoginSSOCallback), apperrors.WithCause(err))
	}
	gothUser, err := provider.FetchUser(session)
	if err != nil {
		return "", "", nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLoginSSOCallback), apperrors.WithCause(err))
	}
	userRow, err := s.store.FindUserByProviderAccount(ctx, sqlcgen.FindUserByProviderAccountParams{Provider: providerID, ProviderAccountID: gothUser.UserID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLoginFindUser), apperrors.WithCause(err))
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
				return "", "", nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAccountAuthSSOLoginFailed))
			}
			return "", "", nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLoginFindUser), apperrors.WithCause(emailErr))
		}
		user = userFromFindUserRow(emailRow)
		if user == nil || !user.EmailVerified {
			return "", "", nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAccountAuthSSOLoginFailed))
		}
		if err := s.store.InsertAccount(ctx, sqlcgen.InsertAccountParams{ID: id.MustParse(s.IDGenerator.New()), UserID: id.MustParse(user.ID), Provider: providerID, ProviderAccountID: gothUser.UserID, CreatedAt: sqltype.Timestamptz(s.Clock.Now()), UpdatedAt: sqltype.Timestamptz(s.Clock.Now())}); err != nil {
			return "", "", nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLoginFindAccount), apperrors.WithCause(err))
		}
	}
	if user == nil {
		return "", "", nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAccountAuthSSOLoginFailed))
	}
	organization, err := s.resolveLoginOrganization(ctx, user.ID, cached.OrganizationSlug)
	if err != nil {
		return "", "", nil, err
	}
	token, err := s.sessions.Create(ctx, user.ID, organization.ID, ip, userAgent)
	if err != nil {
		return "", "", nil, err
	}
	if err := redisClient.Del(ctx, s.sso.cacheKey(transactionID)).Err(); err != nil {
		return "", "", nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLoginSSOCallback), apperrors.WithCause(err))
	}
	return cached.ReturnTo, token, s.clearSSOCookies(), nil
}
