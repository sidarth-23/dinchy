package auth

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/markbates/goth"
	"github.com/markbates/goth/providers/github"
	"github.com/markbates/goth/providers/gitlab"
	"github.com/markbates/goth/providers/google"

	cachecore "github.com/sidarth-23/dinchy/internal/cache/core"
	"github.com/sidarth-23/dinchy/internal/config"
)

func newSSORegistry(authConfig config.AuthConfig, configs []config.SSOProviderConfig, cacheKeyer cachecore.Keyer) (*ssoRegistry, error) {
	registry := &ssoRegistry{
		stateCookieName: authConfig.SSOStateCookieName,
		stateLifetime:   authConfig.SSOStateLifetime,
		envProviders:    map[string]config.SSOProviderConfig{},
		cacheKeyer:      cacheKeyer,
	}
	for _, providerConfig := range configs {
		if !config.IsSupportedSSOProvider(string(providerConfig.ID)) {
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
	return []http.Cookie{*clearCookie(s.sso.stateCookieName, false)}
}

func (r *ssoRegistry) cacheKey(transactionID string) string {
	return r.cacheKeyer.Key("sso", "state", transactionID)
}
