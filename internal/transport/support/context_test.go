package support_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/text/language"

	"github.com/sidarth-23/dinchy/internal/transport/support"
)

func TestSecureCookies_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx := support.WithSecureCookies(context.Background(), true)
	assert.True(t, support.SecureCookies(ctx))

	ctx = support.WithSecureCookies(context.Background(), false)
	assert.False(t, support.SecureCookies(ctx))
}

func TestSecureCookies_Default(t *testing.T) {
	t.Parallel()
	assert.False(t, support.SecureCookies(context.Background()))
}

func TestRequestCookies_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx := support.WithRequestCookies(context.Background(), []*http.Cookie{
		{Name: "session", Value: "rawtoken"},
	})
	assert.Equal(t, "rawtoken", support.CookieValueFrom(ctx, "session"))
	assert.Equal(t, "", support.CookieValueFrom(ctx, "missing"))
}

func TestLang_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx := support.WithLang(context.Background(), language.German)
	assert.Equal(t, language.German, support.LangFrom(ctx))
}

func TestLangFrom_Default(t *testing.T) {
	t.Parallel()
	assert.Equal(t, language.English, support.LangFrom(context.Background()))
}
