package auth

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/platform/cache"
	"github.com/sidarth-23/dinchy/internal/platform/transform"
)

// SSOProviderOut is a single configured SSO provider in an API response.
type SSOProviderOut struct {
	ID   string `json:"id" doc:"Provider identifier"`
	Name string `json:"name" doc:"Provider display name"`
}

// SSOProvidersOut is the response body listing available SSO providers.
type SSOProvidersOut struct {
	Body []SSOProviderOut
}

// SSOStartIn is the request to begin an SSO login flow with a given provider.
type SSOStartIn struct {
	ProviderID       string `path:"provider_id" minLength:"1" example:"google" doc:"Configured SSO provider identifier"`
	ReturnTo         string `query:"return_to" doc:"Relative path to return to after login"`
	OrganizationSlug string `query:"organization_slug" maxLength:"64" doc:"Organization slug to scope the login to"`
}

// Resolve trims the organization slug; ReturnTo is left for the open-redirect guard.
func (in *SSOStartIn) Resolve(huma.Context) []error {
	transform.ApplyTo(transform.SpecTrim, &in.OrganizationSlug)
	return nil
}

// SSOStartOut redirects the client to the provider's authorization endpoint and sets the state cookie.
type SSOStartOut struct {
	Status    int           `status:"302"`
	Location  string        `header:"Location"`
	SetCookie []http.Cookie `header:"Set-Cookie"`
}

// SSOCallbackIn is the request the provider redirects back to after authorization.
type SSOCallbackIn struct {
	ProviderID  string `path:"provider_id" minLength:"1" example:"google" doc:"Configured SSO provider identifier"`
	Code        string `query:"code" doc:"Authorization code returned by the provider (absent on error callbacks)"`
	State       string `query:"state" doc:"Opaque state token echoed back by the provider (absent on error callbacks)"`
	Error       string `query:"error" doc:"Error code returned by the provider when login fails"`
	ErrorDetail string `query:"error_description" doc:"Human-readable error detail from the provider"`
}

// SSOCallbackOut redirects the client after the callback and sets the resulting session cookie.
type SSOCallbackOut struct {
	Status    int           `status:"302"`
	Location  string        `header:"Location"`
	SetCookie []http.Cookie `header:"Set-Cookie"`
}

type ssoRegistry struct {
	stateCookieName string
	stateLifetime   time.Duration
	envProviders    map[string]config.SSOProviderConfig
	cacheKeyer      cache.Keyer
}

type ssoCacheState struct {
	ProviderID       string `json:"provider_id"`
	ReturnTo         string `json:"return_to"`
	OrganizationSlug string `json:"organization_slug"`
	State            string `json:"state"`
	ProviderSession  string `json:"provider_session"`
}

func (s ssoCacheState) MarshalBinary() ([]byte, error) { return json.Marshal(s) }

func (s *ssoCacheState) UnmarshalBinary(data []byte) error { return json.Unmarshal(data, s) }
