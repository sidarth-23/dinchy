package bootstrap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/features/session"
	"github.com/sidarth-23/dinchy/internal/transport/support"
)

type testSettingsReader struct {
	state BootstrapState
}

func (r testSettingsReader) Bootstrap(context.Context) (BootstrapState, error) {
	return r.state, nil
}

func TestAPIBootstrap_Anonymous(t *testing.T) {
	t.Parallel()
	api := &API{
		settings:     testSettingsReader{state: BootstrapState{SetupRequired: true, InstanceName: "dinchy"}},
		requireHTTPS: false,
	}

	out, err := api.bootstrap(context.Background(), &struct{}{})
	require.NoError(t, err)
	assert.Equal(t, true, out.Body.SetupRequired)
	assert.Equal(t, "dinchy", out.Body.App.InstanceName)
	assert.False(t, out.Body.Authenticated)
}

func TestAPIBootstrap_WithSession(t *testing.T) {
	t.Parallel()
	api := &API{
		settings:     testSettingsReader{state: BootstrapState{SetupRequired: false, InstanceName: "dinchy"}},
		requireHTTPS: false,
	}
	ctx := support.WithSession(context.Background(), &session.SessionWithUser{
		SessionID:   "s1",
		UserID:      "u1",
		Email:       "viewer@example.com",
		DisplayName: "Viewer",
		Role:        session.RoleAdmin,
	})

	out, err := api.bootstrap(ctx, &struct{}{})
	require.NoError(t, err)
	assert.False(t, out.Body.SetupRequired)
	assert.True(t, out.Body.Authenticated)
	require.NotNil(t, out.Body.Viewer)
	assert.Equal(t, "viewer@example.com", out.Body.Viewer.Email)
	assert.Equal(t, "Viewer", out.Body.Viewer.DisplayName)
}
