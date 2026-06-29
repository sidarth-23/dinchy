package auth

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSession_RoundTrip(t *testing.T) {
	t.Parallel()
	sess := &SessionWithUser{SessionID: "s1", Email: "a@b.com"}
	ctx := WithSession(context.Background(), sess)
	assert.Equal(t, sess, SessionFrom(ctx))
}

func TestSessionFrom_Empty(t *testing.T) {
	t.Parallel()
	assert.Nil(t, SessionFrom(context.Background()))
}

func TestSessionCookie_Secure(t *testing.T) {
	t.Parallel()
	c := SessionCookie("mytoken", true)
	assert.Equal(t, SessionCookieName, c.Name)
	assert.Equal(t, "mytoken", c.Value)
	assert.True(t, c.HttpOnly)
	assert.True(t, c.Secure)
	assert.Equal(t, http.SameSiteLaxMode, c.SameSite)
	assert.Equal(t, "/", c.Path)
}

func TestSessionCookie_Insecure(t *testing.T) {
	t.Parallel()
	c := SessionCookie("mytoken", false)
	assert.False(t, c.Secure)
}

func TestClearSessionCookie(t *testing.T) {
	t.Parallel()
	c := ClearSessionCookie(false)
	assert.Equal(t, SessionCookieName, c.Name)
	assert.Equal(t, "", c.Value)
	assert.Equal(t, -1, c.MaxAge)
	assert.True(t, c.HttpOnly)
}
