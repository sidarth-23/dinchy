package support_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sidarth-23/dinchy/internal/transport/support"
)

func TestSessionCookie_Secure(t *testing.T) {
	t.Parallel()
	c := support.SessionCookie("mytoken", true)
	assert.Equal(t, support.SessionCookieName, c.Name)
	assert.Equal(t, "mytoken", c.Value)
	assert.True(t, c.HttpOnly)
	assert.True(t, c.Secure)
	assert.Equal(t, http.SameSiteLaxMode, c.SameSite)
	assert.Equal(t, "/", c.Path)
}

func TestSessionCookie_Insecure(t *testing.T) {
	t.Parallel()
	c := support.SessionCookie("mytoken", false)
	assert.False(t, c.Secure)
}

func TestClearSessionCookie(t *testing.T) {
	t.Parallel()
	c := support.ClearSessionCookie(false)
	assert.Equal(t, support.SessionCookieName, c.Name)
	assert.Equal(t, "", c.Value)
	assert.Equal(t, -1, c.MaxAge)
	assert.True(t, c.HttpOnly)
}

func TestCSRFCookie_NotHttpOnly(t *testing.T) {
	t.Parallel()
	c := support.CSRFCookie("csrftoken", false)
	assert.Equal(t, support.CSRFCookieName, c.Name)
	assert.False(t, c.HttpOnly, "CSRF cookie must NOT be HttpOnly — JS needs to read it")
	assert.Equal(t, "csrftoken", c.Value)
}
