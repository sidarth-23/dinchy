package transport_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/testutil"
	transport "github.com/sidarth-23/dinchy/internal/transport"
)

// setupTestServer creates a fully wired handler backed by a real in-process SQLite database.
func setupTestServer(t *testing.T) http.Handler {
	t.Helper()
	s := testutil.OpenTestDB(t)
	clk := testutil.NewFakeClock(fixedTime)
	authSvc := auth.NewService(s, id.NewGenerator(), clk)

	dist := fstest.MapFS{"index.html": {Data: []byte("<html></html>")}}
	srv := transport.New(":0", dist, authSvc, s, false, false, "")
	return srv.Handler
}

// doRequest is a convenience helper for making a JSON request to the test server.
func doRequest(t *testing.T, handler http.Handler, method, path, body string, headers map[string]string) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req := httptest.NewRequestWithContext(context.Background(), method, "http://example.test"+path, bodyReader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr.Result()
}

// csrfToken fetches a CSRF token by making a GET request and extracting the cookie.
func csrfToken(t *testing.T, handler http.Handler) (token, cookieHeader string) {
	t.Helper()
	resp := doRequest(t, handler, http.MethodGet, "/api/bootstrap", "", nil)
	defer func() { _ = resp.Body.Close() }()
	for _, c := range resp.Cookies() {
		if c.Name == "dinchy_csrf" {
			return c.Value, c.Name + "=" + c.Value
		}
	}
	t.Fatal("no CSRF cookie in response")
	return "", ""
}

func TestBootstrap_SetupRequired(t *testing.T) {
	t.Parallel()
	handler := setupTestServer(t)

	resp := doRequest(t, handler, http.MethodGet, "/api/bootstrap", "", nil)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, true, body["setup_required"])
	assert.Equal(t, false, body["authenticated"])
}

func TestSetupFirstUser_GoldenPath(t *testing.T) {
	t.Parallel()
	handler := setupTestServer(t)

	csrf, csrfCookie := csrfToken(t, handler)

	resp := doRequest(t, handler, http.MethodPost, "/api/setup/first-user",
		`{"email":"admin@example.com","display_name":"Admin","password":"password123"}`,
		map[string]string{
			"X-CSRF-Token": csrf,
			"Cookie":       csrfCookie,
		},
	)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, false, body["setup_required"])
	assert.Equal(t, true, body["authenticated"])
	assert.NotNil(t, body["viewer"])

	var sessionCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "dinchy_session" {
			sessionCookie = c
			break
		}
	}
	require.NotNil(t, sessionCookie, "session cookie should be set")
	assert.True(t, sessionCookie.HttpOnly, "session cookie must be HttpOnly")
}

func TestSetupFirstUser_AlreadyDone(t *testing.T) {
	t.Parallel()
	handler := setupTestServer(t)

	csrf, csrfCookie := csrfToken(t, handler)
	headers := map[string]string{"X-CSRF-Token": csrf, "Cookie": csrfCookie}
	body := `{"email":"admin@example.com","display_name":"Admin","password":"password123"}`

	resp1 := doRequest(t, handler, http.MethodPost, "/api/setup/first-user", body, headers)
	_ = resp1.Body.Close()
	require.Equal(t, http.StatusOK, resp1.StatusCode)

	resp2 := doRequest(t, handler, http.MethodPost, "/api/setup/first-user", body, headers)
	defer func() { _ = resp2.Body.Close() }()
	assert.Equal(t, http.StatusConflict, resp2.StatusCode)

	var errBody map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&errBody))
	errPayload := errBody["error"].(map[string]any)
	assert.Equal(t, "auth.setup_completed", errPayload["code"])
}

func TestLogin_GoldenPath(t *testing.T) {
	t.Parallel()
	handler := setupTestServer(t)

	// Setup user first.
	csrf, csrfCookie := csrfToken(t, handler)
	headers := map[string]string{"X-CSRF-Token": csrf, "Cookie": csrfCookie}
	setupResp := doRequest(t, handler, http.MethodPost, "/api/setup/first-user",
		`{"email":"admin@example.com","display_name":"Admin","password":"secret123"}`, headers)
	_ = setupResp.Body.Close()
	require.Equal(t, http.StatusOK, setupResp.StatusCode)

	// Now login.
	loginResp := doRequest(t, handler, http.MethodPost, "/api/auth/login",
		`{"email":"admin@example.com","password":"secret123"}`, headers)
	defer func() { _ = loginResp.Body.Close() }()

	assert.Equal(t, http.StatusOK, loginResp.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(loginResp.Body).Decode(&body))
	assert.Equal(t, true, body["authenticated"])
}

func TestLogin_WrongPassword(t *testing.T) {
	t.Parallel()
	handler := setupTestServer(t)

	csrf, csrfCookie := csrfToken(t, handler)
	headers := map[string]string{"X-CSRF-Token": csrf, "Cookie": csrfCookie}

	// Setup user.
	setupResp := doRequest(t, handler, http.MethodPost, "/api/setup/first-user",
		`{"email":"admin@example.com","display_name":"Admin","password":"correct"}`, headers)
	_ = setupResp.Body.Close()

	loginResp := doRequest(t, handler, http.MethodPost, "/api/auth/login",
		`{"email":"admin@example.com","password":"wrong"}`, headers)
	defer func() { _ = loginResp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, loginResp.StatusCode)
	var errBody map[string]any
	require.NoError(t, json.NewDecoder(loginResp.Body).Decode(&errBody))
	errPayload := errBody["error"].(map[string]any)
	assert.Equal(t, "auth.invalid_credentials", errPayload["code"])
	assert.NotEmpty(t, errPayload["message"])
}

func TestCSRF_MissingToken_Returns400(t *testing.T) {
	t.Parallel()
	handler := setupTestServer(t)

	// POST without CSRF token should fail.
	resp := doRequest(t, handler, http.MethodPost, "/api/auth/login",
		`{"email":"a@b.com","password":"p"}`, nil)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var errBody map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errBody))
	errPayload := errBody["error"].(map[string]any)
	assert.Equal(t, "security.csrf_failed", errPayload["code"])
}

func TestHealthz_NotOnPublicPort(t *testing.T) {
	t.Parallel()
	handler := setupTestServer(t)

	resp := doRequest(t, handler, http.MethodGet, "/healthz", "", nil)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

var fixedTime = clock.RealClock{}.Now()
