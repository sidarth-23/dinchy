package config

import "strings"

type SSOProviderID string

const (
	SSOProviderGoogle SSOProviderID = "google"
	SSOProviderGitHub SSOProviderID = "github"
	SSOProviderGitLab SSOProviderID = "gitlab"
)

type SSOProviderDefinition struct {
	ID   SSOProviderID
	Name string
}

type SSOProviderConfig struct {
	// ID identifies the SSO provider internally.
	ID SSOProviderID
	// Name is the display name shown to users.
	Name string
	// ClientID is the provider OAuth client ID.
	ClientID string
	// Secret is the provider OAuth client secret.
	Secret string
	// CallbackURL is the absolute OAuth callback URL registered with the provider.
	CallbackURL string
	// Enabled is true when the provider has all required credentials.
	Enabled bool
}

type SSOEnvConfig struct {
	// GoogleClientID is the Google OAuth client ID; Google SSO is enabled only when ID, secret, and callback URL are set.
	GoogleClientID string `env:"DINCHY_GOOGLE_CLIENT_ID"`
	// GoogleSecret is the Google OAuth client secret.
	GoogleSecret string `env:"DINCHY_GOOGLE_CLIENT_SECRET"`
	// GoogleCallbackURL is the absolute Google OAuth callback URL.
	GoogleCallbackURL string `env:"DINCHY_GOOGLE_CALLBACK_URL" validate:"omitempty,url"`
	// GitHubClientID is the GitHub OAuth client ID; GitHub SSO is enabled only when ID, secret, and callback URL are set.
	GitHubClientID string `env:"DINCHY_GITHUB_CLIENT_ID"`
	// GitHubSecret is the GitHub OAuth client secret.
	GitHubSecret string `env:"DINCHY_GITHUB_CLIENT_SECRET"`
	// GitHubCallbackURL is the absolute GitHub OAuth callback URL.
	GitHubCallbackURL string `env:"DINCHY_GITHUB_CALLBACK_URL" validate:"omitempty,url"`
	// GitLabClientID is the GitLab OAuth client ID; GitLab SSO is enabled only when ID, secret, and callback URL are set.
	GitLabClientID string `env:"DINCHY_GITLAB_CLIENT_ID"`
	// GitLabSecret is the GitLab OAuth client secret.
	GitLabSecret string `env:"DINCHY_GITLAB_CLIENT_SECRET"`
	// GitLabCallbackURL is the absolute GitLab OAuth callback URL.
	GitLabCallbackURL string `env:"DINCHY_GITLAB_CALLBACK_URL" validate:"omitempty,url"`
}

func SupportedSSOProviders() []SSOProviderDefinition {
	return []SSOProviderDefinition{
		{ID: SSOProviderGoogle, Name: "Google"},
		{ID: SSOProviderGitHub, Name: "GitHub"},
		{ID: SSOProviderGitLab, Name: "GitLab"},
	}
}

func IsSupportedSSOProvider(providerID string) bool {
	for _, provider := range SupportedSSOProviders() {
		if string(provider.ID) == providerID {
			return true
		}
	}
	return false
}

func configuredSSOProviders(cfg Config) []SSOProviderConfig {
	candidates := []SSOProviderConfig{
		{ID: SSOProviderGoogle, Name: "Google", ClientID: cfg.SSO.GoogleClientID, Secret: cfg.SSO.GoogleSecret, CallbackURL: cfg.SSO.GoogleCallbackURL},
		{ID: SSOProviderGitHub, Name: "GitHub", ClientID: cfg.SSO.GitHubClientID, Secret: cfg.SSO.GitHubSecret, CallbackURL: cfg.SSO.GitHubCallbackURL},
		{ID: SSOProviderGitLab, Name: "GitLab", ClientID: cfg.SSO.GitLabClientID, Secret: cfg.SSO.GitLabSecret, CallbackURL: cfg.SSO.GitLabCallbackURL},
	}
	providers := make([]SSOProviderConfig, 0, len(candidates))
	for _, provider := range candidates {
		provider.ClientID = strings.TrimSpace(provider.ClientID)
		provider.Secret = strings.TrimSpace(provider.Secret)
		provider.CallbackURL = strings.TrimSpace(provider.CallbackURL)
		provider.Enabled = provider.ClientID != "" && provider.Secret != "" && provider.CallbackURL != ""
		if provider.ClientID != "" || provider.Secret != "" || provider.CallbackURL != "" {
			providers = append(providers, provider)
		}
	}
	return providers
}
