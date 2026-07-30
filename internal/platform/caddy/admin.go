package caddy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
)

// maxErrorBody bounds how much of Caddy's error response is kept as a diagnostic cause.
const maxErrorBody = 4096

// traversalFailure is the fragment Caddy's error carries when a configuration path has no parent
// to write into. It separates "the edge is not set up the way this deployment needs" from "the
// object being written is wrong", which are different failures with different fixes.
const traversalFailure = "invalid traversal path"

// AdminClient talks to Caddy's admin API.
//
// Every write addresses one object by its "@id", so a push replaces what this deployment owns and
// leaves every other tenant's routes and policies in place. An object that is not there yet is
// appended to its parent array or object, which the edge's base configuration has to provide
// already: Caddy does not create missing parents.
type AdminClient interface {
	// ApplyRoute upserts this deployment's route inside the named server on the edge.
	ApplyRoute(ctx context.Context, edgeServerName string, route ServerRoute) error
	// ApplyTLSPolicy upserts this deployment's certificate automation policy.
	ApplyTLSPolicy(ctx context.Context, policy AutomationPolicy) error
	// Ping reports whether the admin API is reachable.
	Ping(ctx context.Context) error
}

// httpAdminClient is the AdminClient backed by Caddy's HTTP admin API.
type httpAdminClient struct {
	baseURL string
	client  *http.Client
}

// NewAdminClient returns an AdminClient for the configured admin endpoint.
func NewAdminClient(cfg config.CaddyConfig) AdminClient {
	return &httpAdminClient{
		baseURL: "http://" + cfg.AdminEndpoint,
		client:  &http.Client{Timeout: cfg.AdminTimeout},
	}
}

// ApplyRoute upserts the route object inside the edge's server.
//
// The parent path names the server the edge owns, so a server this deployment was pointed at but
// which does not exist surfaces as a base-configuration failure rather than a mystery.
func (c *httpAdminClient) ApplyRoute(ctx context.Context, edgeServerName string, route ServerRoute) error {
	parent := "/config/apps/http/servers/" + edgeServerName + "/routes"
	return c.applyObject(ctx, route.ID, parent, route)
}

// ApplyTLSPolicy upserts the certificate automation policy in the edge's shared policy array.
func (c *httpAdminClient) ApplyTLSPolicy(ctx context.Context, policy AutomationPolicy) error {
	return c.applyObject(ctx, policy.ID, "/config/apps/tls/automation/policies", policy)
}

// applyObject writes one addressable object, replacing it when it is already there and appending it
// to its parent when it is not.
//
// The existence probe is a separate request because Caddy answers an unknown "@id" and a rejected
// object with different statuses but the same shape of body, and only the first means "append
// instead". Discriminating in one place keeps that reading of Caddy's behavior in one place.
func (c *httpAdminClient) applyObject(ctx context.Context, id, parentPath string, object any) error {
	body, err := json.Marshal(object)
	if err != nil {
		return apperrors.Internal(
			i18n.Msg(i18n.CodeDiagnosticsCaddyBuildConfig),
			apperrors.WithCause(fmt.Errorf("marshal Caddy object %q: %w", id, err)),
		)
	}

	exists, err := c.objectExists(ctx, id)
	if err != nil {
		return err
	}
	if exists {
		return c.write(ctx, http.MethodPatch, "/id/"+id, body)
	}
	return c.write(ctx, http.MethodPost, parentPath, body)
}

// write applies one configuration object, classifying a rejection as such.
//
// Caddy provisions the resulting whole configuration before swapping and rolls back if that fails,
// so a rejected write leaves the running proxy — and every other tenant on it — untouched.
func (c *httpAdminClient) write(ctx context.Context, method, path string, body []byte) error {
	return c.do(ctx, method, path, body,
		i18n.CodeDiagnosticsCaddyApplyObject, i18n.CodePlatformRoutingApplyFailed, true)
}

// objectExists reports whether an "@id" is already present in the running configuration.
//
// Caddy answers 404 with "unknown object ID" for one that is not, which is the only status that
// means absent — every other failure on this path, including a malformed object and a missing
// parent, is a 500.
func (c *httpAdminClient) objectExists(ctx context.Context, id string) (bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/id/"+id, http.NoBody)
	if err != nil {
		return false, apperrors.Internal(
			i18n.Msg(i18n.CodeDiagnosticsCaddyLookupObject),
			apperrors.WithCause(fmt.Errorf("build lookup request for Caddy object %q: %w", id, err)),
		)
	}

	response, err := c.client.Do(request)
	if err != nil {
		return false, apperrors.Internal(
			i18n.Msg(i18n.CodePlatformRoutingUnavailable),
			apperrors.WithCause(fmt.Errorf("look up Caddy object %q: %w", id, err)),
		)
	}
	defer func() { _ = response.Body.Close() }()

	detail, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBody))
	switch {
	case response.StatusCode == http.StatusNotFound:
		return false, nil
	case response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices:
		return true, nil
	}
	return false, apperrors.Internal(
		i18n.Msg(i18n.CodeDiagnosticsCaddyLookupObject),
		apperrors.WithCause(fmt.Errorf("look up Caddy object %q returned %d: %s",
			id, response.StatusCode, strings.TrimSpace(string(detail)))),
	)
}

// Ping reports whether the admin API is reachable.
func (c *httpAdminClient) Ping(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/config/", nil,
		i18n.CodeDiagnosticsCaddyReadConfig, i18n.CodePlatformRoutingUnavailable, false)
}

// do issues one admin API request. A transport failure maps to the unavailable message
// so an operator sees "the proxy is not reachable" rather than "the change failed".
//
// rejectable says whether a refusal means Caddy rejected a configuration, which only a write can
// mean. Reading a configuration and failing is not a rejection, and reporting it as one would send
// an operator looking for a bad route that does not exist.
func (c *httpAdminClient) do(ctx context.Context, method, path string, body []byte, diagnostic, userFacing i18n.Code, rejectable bool) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return apperrors.Internal(
			i18n.Msg(diagnostic),
			apperrors.WithCause(fmt.Errorf("build %s %q request: %w", method, path, err)),
		)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.client.Do(request)
	if err != nil {
		return apperrors.Internal(
			i18n.Msg(i18n.CodePlatformRoutingUnavailable),
			apperrors.WithCause(fmt.Errorf("%s %q to Caddy admin API: %w", method, path, err)),
		)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxErrorBody))
		return nil
	}

	detail, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBody))
	message := strings.TrimSpace(string(detail))
	return apperrors.Internal(
		i18n.Msg(failureCode(message, userFacing, rejectable)),
		apperrors.WithCause(fmt.Errorf("%s %q returned %d: %s", method, path, response.StatusCode, message)),
	)
}

// failureCode classifies a refused write.
//
// The status does not distinguish these on its own: a scoped write answers 500 for a malformed
// payload, for an object Caddy refused to provision, and for a path with no parent alike. Only the
// last is the operator's configuration of the edge rather than the object being written, and it
// earns its own message because the fix is in a different file entirely.
func failureCode(message string, userFacing i18n.Code, rejectable bool) i18n.Code {
	if !rejectable {
		return userFacing
	}
	if strings.Contains(message, traversalFailure) {
		return i18n.CodePlatformRoutingBaseConfigInvalid
	}
	return i18n.CodePlatformRoutingConfigRejected
}
