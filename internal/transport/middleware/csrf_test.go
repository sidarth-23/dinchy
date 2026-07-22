package middleware_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/transport/middleware"
	"github.com/sidarth-23/dinchy/internal/transport/support"
)

func csrfHandler(t *testing.T) (http.Handler, *bool) {
	t.Helper()
	called := new(bool)
	h := middleware.CSRF(apperrors.NewRenderer(i18n.Default, false))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*called = true
		w.WriteHeader(http.StatusOK)
	}))
	return h, called
}

func decodeErrorCode(t *testing.T, body []byte) string {
	t.Helper()
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(body, &payload))
	return payload.Error.Code
}

func csrfCookieValue(resp *http.Response) string {
	for _, c := range resp.Cookies() {
		if c.Name == support.CSRFCookieName {
			return c.Value
		}
	}
	return ""
}

func TestCSRF_SafeRequestIssuesCookie(t *testing.T) {
	t.Parallel()
	handler, called := csrfHandler(t)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "http://example.test/", http.NoBody))

	assert.True(t, *called, "safe request should pass through")
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.NotEmpty(t, csrfCookieValue(rr.Result()), "a CSRF cookie should be issued when none is present")
}

func TestCSRF_MutatingRequestWithMatchingTokenSucceeds(t *testing.T) {
	t.Parallel()
	handler, called := csrfHandler(t)

	req := httptest.NewRequest(http.MethodPost, "http://example.test/", http.NoBody)
	req.AddCookie(&http.Cookie{Name: support.CSRFCookieName, Value: "matching-token"})
	req.Header.Set("X-CSRF-Token", "matching-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.True(t, *called)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestCSRF_MutatingRequestWithMismatchedTokenRejected(t *testing.T) {
	var buffer bytes.Buffer
	original := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(original)
	})
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buffer, nil)))

	handler, called := csrfHandler(t)

	req := httptest.NewRequest(http.MethodPost, "http://example.test/", http.NoBody)
	req.AddCookie(&http.Cookie{Name: support.CSRFCookieName, Value: "cookie-token"})
	req.Header.Set("X-CSRF-Token", "different-token")

	rr := httptest.NewRecorder()
	require.NotPanics(t, func() {
		handler.ServeHTTP(rr, req)
	})

	assert.False(t, *called, "next handler must not run when the CSRF check fails")
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	assert.Equal(t, string(i18n.CodeSecurityCSRFFailed), decodeErrorCode(t, rr.Body.Bytes()))
	require.Empty(t, strings.TrimSpace(buffer.String()))
}

func TestCSRF_MutatingRequestWithoutTokenRejected(t *testing.T) {
	t.Parallel()
	handler, called := csrfHandler(t)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "http://example.test/", http.NoBody))

	assert.False(t, *called)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Equal(t, string(i18n.CodeSecurityCSRFFailed), decodeErrorCode(t, rr.Body.Bytes()))
}
