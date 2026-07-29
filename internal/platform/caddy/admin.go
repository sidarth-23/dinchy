package caddy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// AdminClient talks to Caddy's admin API.
//
// LoadConfig replaces the whole configuration and is used only to converge Caddy at
// startup. PutRoute and DeleteRoute address a single route by its "@id" so an
// individual change leaves the rest of the configuration — and every connection it is
// serving — untouched.
type AdminClient interface {
	LoadConfig(ctx context.Context, cfg Config) error
	PutRoute(ctx context.Context, route ServerRoute) error
	DeleteRoute(ctx context.Context, routeID string) error
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

// LoadConfig replaces Caddy's entire configuration. Caddy validates and provisions the
// new configuration before swapping, and rolls back to the previous one if that fails,
// so a rejected document leaves the running proxy untouched.
func (c *httpAdminClient) LoadConfig(ctx context.Context, cfg Config) error {
	body, err := json.Marshal(cfg)
	if err != nil {
		return apperrors.Internal(
			i18n.Msg(i18n.CodeDiagnosticsCaddyBuildConfig),
			apperrors.WithCause(fmt.Errorf("marshal Caddy configuration: %w", err)),
		)
	}
	return c.do(ctx, http.MethodPost, "/load", body, i18n.CodeDiagnosticsCaddyLoadRequest, i18n.CodePlatformRoutingReloadFailed)
}

// PutRoute creates or replaces a single route.
//
// PATCH targets the existing object at /id/<id>; when no such object exists Caddy
// reports not-found and the route is appended to the server's route array instead.
// Routes are kept mutually exclusive by validateNoWildcardShadowing, so appending
// cannot change which route wins a request.
func (c *httpAdminClient) PutRoute(ctx context.Context, route ServerRoute) error {
	body, err := json.Marshal(route)
	if err != nil {
		return apperrors.Internal(
			i18n.Msg(i18n.CodeDiagnosticsCaddyBuildConfig),
			apperrors.WithCause(fmt.Errorf("marshal Caddy route %q: %w", route.ID, err)),
		)
	}
	patchErr := c.do(ctx, http.MethodPatch, "/id/"+route.ID, body, i18n.CodeDiagnosticsCaddyPutRouteRequest, i18n.CodePlatformRoutingReloadFailed)
	if patchErr == nil {
		return nil
	}
	if !isNotFound(patchErr) {
		return patchErr
	}
	appendPath := "/config/apps/http/servers/" + ServerName + "/routes"
	return c.do(ctx, http.MethodPost, appendPath, body, i18n.CodeDiagnosticsCaddyPutRouteRequest, i18n.CodePlatformRoutingReloadFailed)
}

// DeleteRoute removes the route with the given "@id". A route that is already absent
// is not an error: the desired end state is reached either way.
func (c *httpAdminClient) DeleteRoute(ctx context.Context, routeID string) error {
	err := c.do(ctx, http.MethodDelete, "/id/"+routeID, nil, i18n.CodeDiagnosticsCaddyDeleteRouteRequest, i18n.CodePlatformRoutingReloadFailed)
	if err != nil && isNotFound(err) {
		return nil
	}
	return err
}

// Ping reports whether the admin API is reachable.
func (c *httpAdminClient) Ping(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/config/", nil, i18n.CodeDiagnosticsCaddyReadConfig, i18n.CodePlatformRoutingUnavailable)
}

// notFoundError marks a 404/not-found response so PutRoute can fall back to appending.
type notFoundError struct{ detail string }

func (e notFoundError) Error() string { return e.detail }

func isNotFound(err error) bool {
	var notFound notFoundError
	return errors.As(err, &notFound)
}

// do issues one admin API request. A transport failure maps to the unavailable message
// so an operator sees "the proxy is not reachable" rather than "the reload failed".
func (c *httpAdminClient) do(ctx context.Context, method, path string, body []byte, diagnostic, userFacing i18n.Code) error {
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
	if response.StatusCode == http.StatusNotFound || strings.Contains(message, "unknown object ID") {
		return apperrors.Internal(
			i18n.Msg(diagnostic),
			apperrors.WithCause(notFoundError{detail: fmt.Sprintf("%s %q returned %d: %s", method, path, response.StatusCode, message)}),
		)
	}
	code := userFacing
	if response.StatusCode == http.StatusBadRequest {
		code = i18n.CodePlatformRoutingConfigRejected
	}
	return apperrors.Internal(
		i18n.Msg(code),
		apperrors.WithCause(fmt.Errorf("%s %q returned %d: %s", method, path, response.StatusCode, message)),
	)
}
