package support_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sidarth-23/dinchy/internal/transport/support"
)

func TestCSRFCookie_NotHttpOnly(t *testing.T) {
	t.Parallel()
	c := support.CSRFCookie("csrftoken", false)
	assert.Equal(t, support.CSRFCookieName, c.Name)
	assert.False(t, c.HttpOnly, "CSRF cookie must NOT be HttpOnly — JS needs to read it")
	assert.Equal(t, "csrftoken", c.Value)
}

func TestCSRFCookie_Secure(t *testing.T) {
	t.Parallel()
	c := support.CSRFCookie("mytoken", true)
	assert.Equal(t, support.CSRFCookieName, c.Name)
	assert.Equal(t, "mytoken", c.Value)
	assert.False(t, c.HttpOnly)
	assert.True(t, c.Secure)
	assert.Equal(t, http.SameSiteLaxMode, c.SameSite)
	assert.Equal(t, "/", c.Path)
}
