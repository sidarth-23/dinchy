package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/markbates/goth"
	"github.com/markbates/goth/providers/github"
	"github.com/markbates/goth/providers/gitlab"
	"github.com/markbates/goth/providers/google"

	cachecore "github.com/sidarth-23/dinchy/internal/cache/core"
	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/transform"
)

const (
	ssoProviderGoogle = "google"
	ssoProviderGitHub = "github"
	ssoProviderGitLab = "gitlab"
)

var supportedSSOProviders = []SSOProviderOut{
	{ID: ssoProviderGoogle, Name: "Google"},
	{ID: ssoProviderGitHub, Name: "GitHub"},
	{ID: ssoProviderGitLab, Name: "GitLab"},
}

type ssoRegistry struct {
	stateCookieName string
	stateLifetime   time.Duration
	envProviders    map[string]config.SSOProviderConfig
	cacheKeyer      cachecore.Keyer
}

type ssoCacheState struct {
	ProviderID       string `json:"provider_id"`
	ReturnTo         string `json:"return_to"`
	OrganisationSlug string `json:"organisation_slug"`
	State            string `json:"state"`
	Session          string `json:"session"`
}

func newSSORegistry(authConfig config.AuthConfig, configs []config.SSOProviderConfig, cacheKeyer cachecore.Keyer) (*ssoRegistry, error) {
	registry := &ssoRegistry{
		stateCookieName: authConfig.SSOStateCookieName,
		stateLifetime:   authConfig.SSOStateLifetime,
		envProviders:    map[string]config.SSOProviderConfig{},
		cacheKeyer:      cacheKeyer,
	}
	for _, providerConfig := range configs {
		if !supportedSSOProvider(string(providerConfig.ID)) {
			return nil, fmt.Errorf("unsupported sso provider %q", providerConfig.ID)
		}
		registry.envProviders[string(providerConfig.ID)] = providerConfig
	}
	return registry, nil
}

func newGothProvider(cfg config.SSOProviderConfig) (goth.Provider, error) {
	switch cfg.ID {
	case config.SSOProviderGoogle:
		return google.New(cfg.ClientID, cfg.Secret, cfg.CallbackURL, "email", "profile"), nil
	case config.SSOProviderGitHub:
		return github.New(cfg.ClientID, cfg.Secret, cfg.CallbackURL, "user:email"), nil
	case config.SSOProviderGitLab:
		return gitlab.New(cfg.ClientID, cfg.Secret, cfg.CallbackURL, "read_user"), nil
	default:
		return nil, fmt.Errorf("unsupported sso provider %q", cfg.ID)
	}
}

var newGothProviderForSSO = newGothProvider

func (s *Service) listSSOProviders(ctx context.Context) ([]SSOProviderOut, error) {
	configs, err := s.effectiveSSOProviderConfigs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SSOProviderOut, 0, len(configs))
	for _, providerConfig := range configs {
		if !providerConfig.Enabled {
			continue
		}
		out = append(out, SSOProviderOut{ID: string(providerConfig.ID), Name: providerConfig.Name})
	}
	return out, nil
}

func (s *Service) startSSO(ctx context.Context, providerID, returnTo, organisationSlug string) (string, []http.Cookie, error) {
	providerConfig, ok, err := s.effectiveSSOProviderConfig(ctx, providerID)
	if err != nil {
		return "", nil, err
	}
	if !ok || !providerConfig.Enabled {
		return "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOProviderNotFound, i18n.P("provider", providerID)))
	}
	if s.cache == nil {
		return "", nil, apperrors.Internal(i18n.Msg(i18n.CodeAuthSSOCacheRequired))
	}
	provider, err := newGothProviderForSSO(providerConfig)
	if err != nil {
		return "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOStart))
	}
	stateToken, err := newRandomToken(32)
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
	transactionID, err := newRandomToken(32)
	if err != nil {
		return "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageGenerateToken))
	}
	cacheState := ssoCacheState{
		ProviderID:       providerID,
		ReturnTo:         transform.InternalReturnPath(returnTo),
		OrganisationSlug: strings.TrimSpace(organisationSlug),
		State:            stateToken,
		Session:          session.Marshal(),
	}
	if err := cachecore.SetJSON(ctx, s.cache, s.sso.cacheKey(transactionID), cacheState, s.sso.stateLifetime); err != nil {
		return "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOStart))
	}
	return authURL, []http.Cookie{{
		Name:     s.sso.stateCookieName,
		Value:    transactionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.sso.stateLifetime.Seconds()),
		Expires:  s.clock.Now().Add(s.sso.stateLifetime),
	}}, nil
}

func (s *Service) completeSSO(ctx context.Context, providerID, queryState, code, transactionID, ip, userAgent string) (string, string, []http.Cookie, error) {
	providerConfig, ok, err := s.effectiveSSOProviderConfig(ctx, providerID)
	if err != nil {
		return "", "", nil, err
	}
	if !ok || !providerConfig.Enabled {
		return "", "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOProviderNotFound, i18n.P("provider", providerID)))
	}
	if s.cache == nil || transactionID == "" {
		return "", "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOInvalidState))
	}
	var cached ssoCacheState
	if err := cachecore.GetJSON(ctx, s.cache, s.sso.cacheKey(transactionID), &cached); err != nil {
		return "", "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOInvalidState))
	}
	if cached.ProviderID != providerID || cached.State != queryState {
		return "", "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOInvalidState))
	}
	provider, err := newGothProviderForSSO(providerConfig)
	if err != nil {
		return "", "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOCallback))
	}
	session, err := provider.UnmarshalSession(cached.Session)
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
	user, err := s.store.FindUserByProviderAccount(ctx, providerID, gothUser.UserID)
	if err != nil {
		return "", "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageFindUser))
	}
	if user == nil && gothUser.Email != "" {
		user, err = s.store.FindUserByEmail(ctx, transform.Email(gothUser.Email))
		if err != nil {
			return "", "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageFindUser))
		}
	}
	if user == nil {
		return "", "", nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthSSOLoginFailed))
	}
	organisation, err := s.resolveLoginOrganisation(ctx, user.ID, cached.OrganisationSlug)
	if err != nil {
		return "", "", nil, err
	}
	token, err := s.newSession(ctx, user.ID, organisation.ID, ip, userAgent)
	if err != nil {
		return "", "", nil, err
	}
	if err := s.cache.Delete(ctx, s.sso.cacheKey(transactionID)); err != nil {
		return "", "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOCallback))
	}
	return cached.ReturnTo, token, s.clearSSOCookies(), nil
}

func validateSSOState(session goth.Session, queryState string) error {
	authURL, err := session.GetAuthURL()
	if err != nil {
		return err
	}
	parsedURL, err := url.Parse(authURL)
	if err != nil {
		return err
	}
	if parsedURL.Query().Get("state") != queryState {
		return fmt.Errorf("state token mismatch")
	}
	return nil
}

func (s *Service) clearSSOCookies() []http.Cookie {
	return []http.Cookie{{
		Name:     s.sso.stateCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}}
}

func (r *ssoRegistry) cacheKey(transactionID string) string {
	return r.cacheKeyer.Key("sso", "state", transactionID)
}

func supportedSSOProvider(providerID string) bool {
	return slices.ContainsFunc(supportedSSOProviders, func(provider SSOProviderOut) bool {
		return provider.ID == providerID
	})
}
