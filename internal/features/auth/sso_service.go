package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/markbates/goth"
	"github.com/markbates/goth/providers/github"
	"github.com/markbates/goth/providers/gitlab"
	"github.com/markbates/goth/providers/google"
	"github.com/markbates/goth/providers/microsoftonline"

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/transform"
)

type ssoRegistry struct {
	stateCookieName   string
	sessionCookieName string
	stateLifetime     time.Duration
	providers         map[string]goth.Provider
	summaries         []SSOProviderOut
}

type ssoState struct {
	ProviderID       string    `json:"provider_id"`
	ReturnTo         string    `json:"return_to"`
	OrganisationSlug string    `json:"organisation_slug"`
	ExpiresAt        time.Time `json:"expires_at"`
}

const ssoSessionCookieName = "dinchy_sso_session"

func newSSORegistry(authConfig config.AuthConfig, configs []config.SSOProviderConfig) (*ssoRegistry, error) {
	registry := &ssoRegistry{
		stateCookieName:   authConfig.SSOStateCookieName,
		sessionCookieName: ssoSessionCookieName,
		stateLifetime:     authConfig.SSOStateLifetime,
		providers:         map[string]goth.Provider{},
	}
	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		provider, err := newGothProvider(cfg)
		if err != nil {
			return nil, err
		}
		registry.providers[string(cfg.ID)] = provider
		registry.summaries = append(registry.summaries, SSOProviderOut{ID: string(cfg.ID), Name: cfg.Name})
	}
	return registry, nil
}

func newGothProvider(cfg config.SSOProviderConfig) (goth.Provider, error) {
	switch cfg.ID {
	case config.SSOProviderGoogle:
		return google.New(cfg.ClientID, cfg.Secret, cfg.CallbackURL, "email", "profile"), nil
	case config.SSOProviderGitHub:
		return github.New(cfg.ClientID, cfg.Secret, cfg.CallbackURL, "user:email"), nil
	case config.SSOProviderMicrosoft:
		return microsoftonline.New(cfg.ClientID, cfg.Secret, cfg.CallbackURL, "common", "openid", "email", "profile"), nil
	case config.SSOProviderGitLab:
		return gitlab.New(cfg.ClientID, cfg.Secret, cfg.CallbackURL, "read_user"), nil
	default:
		return nil, fmt.Errorf("unsupported sso provider %q", cfg.ID)
	}
}

func (s *Service) listSSOProviders() []SSOProviderOut {
	if s == nil || s.sso == nil {
		return nil
	}
	return append([]SSOProviderOut(nil), s.sso.summaries...)
}

func (s *Service) startSSO(ctx context.Context, providerID, returnTo, organisationSlug string) (string, []http.Cookie, error) {
	provider, ok := s.sso.providers[providerID]
	if !ok {
		return "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOProviderNotFound, i18n.P("provider", providerID)))
	}
	stateToken, err := newRandomToken(16)
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
	now := s.clock.Now()
	state := ssoState{
		ProviderID:       providerID,
		ReturnTo:         transform.InternalReturnPath(returnTo),
		OrganisationSlug: strings.TrimSpace(organisationSlug),
		ExpiresAt:        now.Add(s.sso.stateLifetime),
	}
	encodedState, err := encodeSSOState(state)
	if err != nil {
		return "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOStart))
	}
	encodedSession, err := encodeSSOSession(session.Marshal())
	if err != nil {
		return "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOStart))
	}
	return authURL, []http.Cookie{
		{
			Name:     s.sso.stateCookieName,
			Value:    encodedState,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Expires:  state.ExpiresAt,
			MaxAge:   int(time.Until(state.ExpiresAt).Seconds()),
		},
		{
			Name:     s.sso.sessionCookieName,
			Value:    encodedSession,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Expires:  state.ExpiresAt,
			MaxAge:   int(time.Until(state.ExpiresAt).Seconds()),
		},
	}, nil
}

func (s *Service) completeSSO(ctx context.Context, providerID, queryState, code, stateCookieValue, sessionCookieValue, ip, userAgent string) (string, string, []http.Cookie, error) {
	provider, ok := s.sso.providers[providerID]
	if !ok {
		return "", "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOProviderNotFound, i18n.P("provider", providerID)))
	}
	state, err := decodeSSOState(stateCookieValue)
	if err != nil || state.ProviderID != providerID || s.clock.Now().After(state.ExpiresAt) {
		return "", "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOInvalidState))
	}
	sessionRaw, err := decodeSSOSession(sessionCookieValue)
	if err != nil {
		return "", "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOInvalidState))
	}
	session, err := provider.UnmarshalSession(sessionRaw)
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
	organisation, err := s.resolveLoginOrganisation(ctx, user.ID, state.OrganisationSlug)
	if err != nil {
		return "", "", nil, err
	}
	token, err := s.newSession(ctx, user.ID, organisation.ID, ip, userAgent)
	if err != nil {
		return "", "", nil, err
	}
	return state.ReturnTo, token, s.clearSSOCookies(), nil
}

func encodeSSOState(state ssoState) (string, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeSSOState(raw string) (ssoState, error) {
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return ssoState{}, err
	}
	var state ssoState
	if err := json.Unmarshal(data, &state); err != nil {
		return ssoState{}, err
	}
	return state, nil
}

func encodeSSOSession(raw string) (string, error) {
	return base64.RawURLEncoding.EncodeToString([]byte(raw)), nil
}

func decodeSSOSession(raw string) (string, error) {
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", err
	}
	return string(data), nil
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
	return []http.Cookie{
		{
			Name:     s.sso.stateCookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		},
		{
			Name:     s.sso.sessionCookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		},
	}
}
