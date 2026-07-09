// Package testsupport provides ephemeral Postgres containers for integration tests.
package testsupport

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sidarth-23/dinchy/internal/platform/store"
)

const postgresImage = "docker.io/library/postgres:16-alpine"

// OpenPostgresStore starts a temporary Postgres container and returns a migrated store.
func OpenPostgresStore(t testing.TB) *store.Store {
	t.Helper()

	runtime, err := containerRuntime()
	if err != nil {
		t.Skip(err.Error())
	}

	ctx := context.Background()
	containerName := fmt.Sprintf("dinchy-postgres-%d", time.Now().UnixNano())
	if _, err := runCommand(ctx, runtime, "run",
		"-d",
		"--rm",
		"--name", containerName,
		"-e", "POSTGRES_DB=dinchy",
		"-e", "POSTGRES_USER=dinchy",
		"-e", "POSTGRES_PASSWORD=dinchy",
		"-p", "127.0.0.1::5432",
		postgresImage,
	); err != nil {
		if shouldSkipContainerError(err) {
			t.Skip("container runtime is present but cannot create rootless containers in this environment")
		}
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		_, _ = runCommand(ctx, runtime, "rm", "-f", containerName)
	})

	hostPort, err := mappedPort(ctx, runtime, containerName)
	if err != nil {
		t.Fatalf("inspect postgres container port: %v", err)
	}
	dsn := fmt.Sprintf("postgres://dinchy:dinchy@127.0.0.1:%s/dinchy?sslmode=disable", hostPort)

	var db *store.Store
	for range 20 {
		db, err = store.Open(ctx, dsn)
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("close postgres store: %v", closeErr)
		}
	})
	return db
}

func containerRuntime() (string, error) {
	if runtime, err := exec.LookPath("podman"); err == nil {
		return runtime, nil
	}
	if runtime, err := exec.LookPath("docker"); err == nil {
		return runtime, nil
	}
	return "", fmt.Errorf("no container runtime found; install podman or docker")
}

func mappedPort(ctx context.Context, runtime, containerName string) (string, error) {
	output, err := runCommand(ctx, runtime, "inspect", "--format", "{{(index (index .NetworkSettings.Ports \"5432/tcp\") 0).HostPort}}", containerName)
	if err != nil {
		return "", err
	}
	port := strings.TrimSpace(output)
	if port == "" {
		return "", fmt.Errorf("container %q did not report a mapped Postgres port", containerName)
	}
	return port, nil
}

func runCommand(ctx context.Context, runtime string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, runtime, args...)
	command.Env = commandEnvironment(runtime)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", runtime, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func shouldSkipContainerError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	for _, pattern := range []string{
		"newuidmap",
		"cannot set up namespace",
		"read-only file system",
		"Failed to obtain podman configuration",
		"operation not permitted",
		"Operation not permitted",
	} {
		if strings.Contains(message, pattern) {
			return true
		}
	}
	return false
}

func commandEnvironment(runtime string) []string {
	env := os.Environ()
	if strings.Contains(filepath.Base(runtime), "podman") {
		baseDir := filepath.Join(os.TempDir(), "dinchy-podman")
		homeDir := filepath.Join(baseDir, "home")
		runtimeDir := filepath.Join(baseDir, "runtime")
		configDir := filepath.Join(baseDir, "config")
		dataDir := filepath.Join(baseDir, "data")
		for _, dir := range []string{homeDir, runtimeDir, configDir, dataDir} {
			_ = os.MkdirAll(dir, 0o700)
		}
		env = append(env,
			"HOME="+homeDir,
			"XDG_RUNTIME_DIR="+runtimeDir,
			"XDG_CONFIG_HOME="+configDir,
			"XDG_DATA_HOME="+dataDir,
		)
	}
	return env
}
