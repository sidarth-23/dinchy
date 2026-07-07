package auth

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/platform/redis"
	"github.com/sidarth-23/dinchy/internal/platform/transform"
)

type SSOProviderOut struct {
	ID   string `json:"id" doc:"Provider identifier"`
	Name string `json:"name" doc:"Provider display name"`
}

type SSOProvidersOut struct {
	Body []SSOProviderOut
}

type SSOStartIn struct {
	ProviderID       string `path:"provider_id" minLength:"1" example:"google" doc:"Configured SSO provider identifier"`
	ReturnTo         string `query:"return_to" doc:"Relative path to return to after login"`
	OrganisationSlug string `query:"organisation_slug" maxLength:"64" doc:"Organisation slug to scope the login to"`
}

// Resolve trims the organisation slug; ReturnTo is left for the open-redirect guard.
func (in *SSOStartIn) Resolve(huma.Context) []error {
	transform.ApplyTo(transform.SpecTrim, &in.OrganisationSlug)
	return nil
}

type SSOStartOut struct {
	Status    int           `status:"302"`
	Location  string        `header:"Location"`
	SetCookie []http.Cookie `header:"Set-Cookie"`
}

type SSOCallbackIn struct {
	ProviderID  string `path:"provider_id" minLength:"1" example:"google" doc:"Configured SSO provider identifier"`
	Code        string `query:"code" doc:"Authorization code returned by the provider (absent on error callbacks)"`
	State       string `query:"state" doc:"Opaque state token echoed back by the provider (absent on error callbacks)"`
	Error       string `query:"error" doc:"Error code returned by the provider when login fails"`
	ErrorDetail string `query:"error_description" doc:"Human-readable error detail from the provider"`
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
	cacheKeyer      redis.Keyer
}

type ssoCacheState struct {
	ProviderID       string `json:"provider_id"`
	ReturnTo         string `json:"return_to"`
	OrganisationSlug string `json:"organisation_slug"`
	State            string `json:"state"`
	Session          string `json:"session"`
}

func (s ssoCacheState) MarshalBinary() ([]byte, error) { return json.Marshal(s) }

func (s *ssoCacheState) UnmarshalBinary(data []byte) error { return json.Unmarshal(data, s) }
