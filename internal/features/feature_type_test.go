package features

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/logging"
)

func TestBaseFeatureLoggerPrefersContextLoggerAndAddsFeature(t *testing.T) {
	var fallbackOutput bytes.Buffer
	fallback := slog.New(slog.NewJSONHandler(&fallbackOutput, nil))
	feature := NewBaseFeature("auth", FeatureDependencies{Logger: fallback})

	var requestOutput bytes.Buffer
	requestLogger := slog.New(slog.NewJSONHandler(&requestOutput, nil)).With(slog.String("request_id", "request-123"))
	ctx := logging.WithLogger(context.Background(), requestLogger)
	feature.Logger(ctx).Info("Feature operation")

	assert.Empty(t, fallbackOutput.String())
	assert.Contains(t, requestOutput.String(), `"feature":"auth"`)
	assert.Contains(t, requestOutput.String(), `"request_id":"request-123"`)
}

func TestNewBaseServiceRequiresClock(t *testing.T) {
	_, err := NewBaseService("auth", ServiceDependencies{})
	require.Error(t, err)
}

func TestNewBaseServiceDefaultsIDGenerator(t *testing.T) {
	base, err := NewBaseService("auth", ServiceDependencies{Clock: clock.Fixed(time.Unix(0, 0))})
	require.NoError(t, err)
	assert.Equal(t, "auth", base.Name())
	assert.NotNil(t, base.IDGenerator())
}
