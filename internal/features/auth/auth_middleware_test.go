package auth

import (
	"context"
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
