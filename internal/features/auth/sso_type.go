package auth

import (
	"net/http"
	"time"

	"github.com/sidarth-23/dinchy/internal/config"
	cachecore "github.com/sidarth-23/dinchy/internal/platform/cache/core"
)

type SSOProviderOut struct {
	ID   string `json:"id" doc:"Provider identifier"`
	Name string `json:"name" doc:"Provider display name"`
}

type SSOProvidersOut struct {
	Body []SSOProviderOut
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
