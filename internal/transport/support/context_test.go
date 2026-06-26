package support_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/text/language"

	"github.com/sidarth-23/dinchy/internal/domain"
	"github.com/sidarth-23/dinchy/internal/transport/support"
)

func TestSession_RoundTrip(t *testing.T) {
	t.Parallel()
	sess := &domain.SessionWithUser{SessionID: "s1", Email: "a@b.com"}
	ctx := support.WithSession(context.Background(), sess)
	assert.Equal(t, sess, support.SessionFrom(ctx))
}

func TestSessionFrom_Empty(t *testing.T) {
	t.Parallel()
	assert.Nil(t, support.SessionFrom(context.Background()))
}

func TestSecure_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx := support.WithSecure(context.Background(), true)
	assert.True(t, support.IsSecure(ctx))

	ctx = support.WithSecure(context.Background(), false)
	assert.False(t, support.IsSecure(ctx))
}

func TestIsSecure_Default(t *testing.T) {
	t.Parallel()
	assert.False(t, support.IsSecure(context.Background()))
}

func TestRequestInfo_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx := support.WithRequestInfo(context.Background(), "1.2.3.4", "Mozilla/5.0")
	assert.Equal(t, "1.2.3.4", support.RemoteIPFrom(ctx))
	assert.Equal(t, "Mozilla/5.0", support.UserAgentFrom(ctx))
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
