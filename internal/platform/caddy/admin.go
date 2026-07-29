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

// AdminClient talks to Caddy's admin API.
//
// LoadConfig replaces the whole configuration, which is the only way Dinchy changes it
// today. Addressing an individual route is what avoids closing the connections the other
// routes are serving, so it belongs here once there are routes that come and go.
type AdminClient interface {
	LoadConfig(ctx context.Context, cfg Config) error
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

// Ping reports whether the admin API is reachable.
func (c *httpAdminClient) Ping(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/config/", nil, i18n.CodeDiagnosticsCaddyReadConfig, i18n.CodePlatformRoutingUnavailable)
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
	code := userFacing
	if response.StatusCode == http.StatusBadRequest {
		code = i18n.CodePlatformRoutingConfigRejected
	}
	return apperrors.Internal(
		i18n.Msg(code),
		apperrors.WithCause(fmt.Errorf("%s %q returned %d: %s", method, path, response.StatusCode, message)),
	)
}
