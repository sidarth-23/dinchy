package features

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/foundation/clock"
	"github.com/sidarth-23/dinchy/internal/foundation/id"
	"github.com/sidarth-23/dinchy/internal/platform/logging"
)

func TestNamedCreatesIsolatedModuleServices(t *testing.T) {
	base := Service{Clock: clock.Fixed(time.Unix(0, 0)), IDGenerator: id.NewGenerator()}
	authService := base.Named("auth")
	auditService := base.Named("audit")
	require.NoError(t, authService.Initialize())
	require.NoError(t, auditService.Initialize())

	assert.Equal(t, "auth", authService.Name())
	assert.Equal(t, "audit", auditService.Name())
	assert.Equal(t, authService.Clock, auditService.Clock)
	assert.Equal(t, authService.IDGenerator, auditService.IDGenerator)
}

func TestServiceLoggerPrefersContextLoggerAndAddsModule(t *testing.T) {
	var fallbackOutput bytes.Buffer
	fallback := slog.New(slog.NewJSONHandler(&fallbackOutput, nil))
	service := Service{ModuleName: "auth", BaseLogger: fallback, Clock: clock.Fixed(time.Unix(0, 0))}
	require.NoError(t, service.Initialize())

	var requestOutput bytes.Buffer
	requestLogger := slog.New(slog.NewJSONHandler(&requestOutput, nil)).With(slog.String("request_id", "request-123"))
	service.Logger(logging.WithLogger(context.Background(), requestLogger)).Info("Module operation")

	assert.Empty(t, fallbackOutput.String())
	assert.Contains(t, requestOutput.String(), `"module":"auth"`)
	assert.Contains(t, requestOutput.String(), `"request_id":"request-123"`)
}

func TestInitializeRequiresClock(t *testing.T) {
	err := (&Service{ModuleName: "auth"}).Initialize()
	require.Error(t, err)
}
