package auth

import (
	"net/http"
	"time"

	cachecore "github.com/sidarth-23/dinchy/internal/cache/core"
	"github.com/sidarth-23/dinchy/internal/config"
)

type SSOProviderOut struct {
	ID   string `json:"id" doc:"Provider identifier"`
	Name string `json:"name" doc:"Provider display name"`
}

type SSOProvidersOut struct {
	Body []SSOProviderOut
}

type SSOProviderFieldOut struct {
	Set    bool   `json:"set"`
	Source string `json:"source,omitempty" enum:"env,db"`
}

type SSOProviderSettingOut struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	ClientID      SSOProviderFieldOut `json:"client_id"`
	ClientSecret  SSOProviderFieldOut `json:"client_secret"`
	CallbackURL   SSOProviderFieldOut `json:"callback_url"`
	Enabled       bool                `json:"enabled"`
	EnabledSource string              `json:"enabled_source,omitempty" enum:"env,db"`
}

type SSOProviderSettingsOut struct {
	Body []SSOProviderSettingOut
}

type SSOProviderSettingUpdateIn struct {
	ProviderID string `path:"provider_id"`
	Body       SSOProviderSettingUpdateBody
}

type SSOProviderSettingUpdateBody struct {
	ClientID     *string `json:"client_id,omitempty"`
	ClientSecret *string `json:"client_secret,omitempty"`
	CallbackURL  *string `json:"callback_url,omitempty"`
	Enabled      *bool   `json:"enabled,omitempty"`
}

type SSOProviderSettingUpdateOut struct {
	Body SSOProviderSettingOut
}

type SSOStartIn struct {
	ProviderID       string `path:"provider_id"`
	ReturnTo         string `query:"return_to"`
	OrganisationSlug string `query:"organisation_slug"`
}

type SSOStartOut struct {
	Status    int           `status:"302"`
	Location  string        `header:"Location"`
	SetCookie []http.Cookie `header:"Set-Cookie"`
}

type SSOCallbackIn struct {
	ProviderID  string `path:"provider_id"`
	Code        string `query:"code"`
	State       string `query:"state"`
	Error       string `query:"error"`
	ErrorDetail string `query:"error_description"`
}

type SSOCallbackOut struct {
	Status    int           `status:"302"`
	Location  string        `header:"Location"`
	SetCookie []http.Cookie `header:"Set-Cookie"`
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

type mergedSSOProviderSetting struct {
	ID                string
	Name              string
	ClientID          string
	ClientIDSource    string
	Secret            string
	SecretSource      string
	CallbackURL       string
	CallbackURLSource string
	Enabled           bool
	EnabledSource     string
}
