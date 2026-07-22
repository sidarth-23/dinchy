package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunLogReturnFlagsLoggingAndReturningFunction(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeModuleFixture(t, dir)
	writeFileFixture(t, dir, `package main

import (
	"context"
	"log/slog"

	logging "github.com/sidarth-23/dinchy/internal/platform/logging"
)

func flagged(ctx context.Context, logger *slog.Logger, err error) error {
	logging.Error(ctx, logger, "boom", err)
	return err
}
`)

	err := runLogReturn([]string{filepath.Join(dir, "fixture.go")})
	require.Error(t, err)
	require.Contains(t, err.Error(), "function \"flagged\" logs an error and also returns one")
}

func TestRunLogReturnFlagsSlogErrorContext(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeModuleFixture(t, dir)
	writeFileFixture(t, dir, `package main

import (
	"context"
	"log/slog"
)

func flagged(ctx context.Context, logger *slog.Logger, err error) error {
	logger.ErrorContext(ctx, "boom", "error", err)
	return err
}
`)

	err := runLogReturn([]string{filepath.Join(dir, "fixture.go")})
	require.Error(t, err)
	require.Contains(t, err.Error(), "function \"flagged\" logs an error and also returns one")
}

func TestRunLogReturnAcceptsBoundaryAndReturnOnlyFunctions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeModuleFixture(t, dir)
	writeFileFixture(t, dir, `package main

import (
	"context"
	"log/slog"
)

func boundary(ctx context.Context, logger *slog.Logger) {
	logger.ErrorContext(ctx, "boom")
}

func service(err error) error {
	return err
}
`)

	require.NoError(t, runLogReturn([]string{filepath.Join(dir, "fixture.go")}))
}

func TestRunLogReturnAllowsAnnotatedCleanupFunction(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeModuleFixture(t, dir)
	writeFileFixture(t, dir, `package main

import (
	"context"
	"log/slog"
)

//dinchy:allow-logreturn cleanup failure
func allowed(ctx context.Context, logger *slog.Logger, err error) error {
	logger.ErrorContext(ctx, "boom", "error", err)
	return err
}
`)

	require.NoError(t, runLogReturn([]string{filepath.Join(dir, "fixture.go")}))
}

func TestRunLogReturnRejectsDirectiveWithoutReason(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeModuleFixture(t, dir)
	writeFileFixture(t, dir, `package main

import (
	"context"
	"log/slog"
)

//dinchy:allow-logreturn
func flagged(ctx context.Context, logger *slog.Logger, err error) error {
	logger.ErrorContext(ctx, "boom", "error", err)
	return err
}
`)

	err := runLogReturn([]string{filepath.Join(dir, "fixture.go")})
	require.Error(t, err)
	require.Contains(t, err.Error(), "allow-logreturn directive requires a reason")
}

func TestRunLogReturnAcceptsRealTree(t *testing.T) {
	t.Parallel()

	require.NoError(t, runLogReturn([]string{"./..."}))
}

func writeModuleFixture(t *testing.T, dir string) {
	t.Helper()

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fixture\n\ngo 1.26\n\nrequire github.com/sidarth-23/dinchy v0.0.0\nreplace github.com/sidarth-23/dinchy => "+repoRoot+"\n"), 0o644))
}

func writeFileFixture(t *testing.T, dir, source string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "fixture.go"), []byte(source), 0o644))
}
