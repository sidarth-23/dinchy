package config

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

var supportedSSOProviderDefinitions = []SSOProviderDefinition{
	{ID: SSOProviderGoogle, Name: "Google"},
	{ID: SSOProviderGitHub, Name: "GitHub"},
	{ID: SSOProviderGitLab, Name: "GitLab"},
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
	GoogleClientID string `env:"DINCHY_GOOGLE_CLIENT_ID" mod:"trim"`
	// GoogleSecret is the Google OAuth client secret.
	GoogleSecret string `env:"DINCHY_GOOGLE_CLIENT_SECRET" mod:"trim"`
	// GoogleCallbackURL is the absolute Google OAuth callback URL.
	GoogleCallbackURL string `env:"DINCHY_GOOGLE_CALLBACK_URL" mod:"trim" validate:"omitempty,url"`
	// GitHubClientID is the GitHub OAuth client ID; GitHub SSO is enabled only when ID, secret, and callback URL are set.
	GitHubClientID string `env:"DINCHY_GITHUB_CLIENT_ID" mod:"trim"`
	// GitHubSecret is the GitHub OAuth client secret.
	GitHubSecret string `env:"DINCHY_GITHUB_CLIENT_SECRET" mod:"trim"`
	// GitHubCallbackURL is the absolute GitHub OAuth callback URL.
	GitHubCallbackURL string `env:"DINCHY_GITHUB_CALLBACK_URL" mod:"trim" validate:"omitempty,url"`
	// GitLabClientID is the GitLab OAuth client ID; GitLab SSO is enabled only when ID, secret, and callback URL are set.
	GitLabClientID string `env:"DINCHY_GITLAB_CLIENT_ID" mod:"trim"`
	// GitLabSecret is the GitLab OAuth client secret.
	GitLabSecret string `env:"DINCHY_GITLAB_CLIENT_SECRET" mod:"trim"`
	// GitLabCallbackURL is the absolute GitLab OAuth callback URL.
	GitLabCallbackURL string `env:"DINCHY_GITLAB_CALLBACK_URL" mod:"trim" validate:"omitempty,url"`
}

func SupportedSSOProviders() []SSOProviderDefinition {
	return append([]SSOProviderDefinition(nil), supportedSSOProviderDefinitions...)
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
	providers := make([]SSOProviderConfig, 0, len(supportedSSOProviderDefinitions))
	for _, definition := range supportedSSOProviderDefinitions {
		var provider SSOProviderConfig
		provider.ID = definition.ID
		provider.Name = definition.Name
		switch definition.ID {
		case SSOProviderGoogle:
			provider.ClientID = cfg.SSO.GoogleClientID
			provider.Secret = cfg.SSO.GoogleSecret
			provider.CallbackURL = cfg.SSO.GoogleCallbackURL
		case SSOProviderGitHub:
			provider.ClientID = cfg.SSO.GitHubClientID
			provider.Secret = cfg.SSO.GitHubSecret
			provider.CallbackURL = cfg.SSO.GitHubCallbackURL
		case SSOProviderGitLab:
			provider.ClientID = cfg.SSO.GitLabClientID
			provider.Secret = cfg.SSO.GitLabSecret
			provider.CallbackURL = cfg.SSO.GitLabCallbackURL
		}
		provider.Enabled = provider.ClientID != "" && provider.Secret != "" && provider.CallbackURL != ""
		if provider.ClientID != "" || provider.Secret != "" || provider.CallbackURL != "" {
			providers = append(providers, provider)
		}
	}
	return providers
}
