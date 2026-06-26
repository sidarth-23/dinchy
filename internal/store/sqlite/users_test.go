package sqlite_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/id"
)

func TestCreateFirstUser_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := mustOpenTestDB(t)

	u, err := createTestUser(ctx, t, s)
	require.NoError(t, err)
	assert.Equal(t, "admin@example.com", u.Email)
	assert.Equal(t, "Admin", u.DisplayName)
	assert.Equal(t, auth.RoleAdmin, u.Role)
	assert.NotEmpty(t, u.ID)
}

func TestCreateFirstUser_SetupAlreadyCompleted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := mustOpenTestDB(t)

	_, err := createTestUser(ctx, t, s)
	require.NoError(t, err)

	_, err = createTestUser(ctx, t, s)
	require.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.Conflict(i18n.Msg(i18n.CodeAuthSetupCompleted, i18n.P("resource", "users"), i18n.P("count", 1)))))
}

func TestCreateFirstUser_Concurrent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := mustOpenTestDB(t)

	const n = 10
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := s.CreateFirstUser(ctx, auth.CreateUserInput{
				ID:           id.NewGenerator().New(),
				Email:        "admin@example.com",
				PasswordHash: "hash",
				DisplayName:  "Admin",
				Now:          time.Now().UTC(),
			})
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	successCount := 0
	for _, err := range errs {
		if err == nil {
			successCount++
		}
	}
	assert.Equal(t, 1, successCount, "exactly one goroutine should succeed")
}

func TestFindUserByEmail_Found(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := mustOpenTestDB(t)

	created, err := createTestUser(ctx, t, s)
	require.NoError(t, err)

	found, err := s.FindUserByEmail(ctx, "admin@example.com")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, created.ID, found.ID)
	assert.Equal(t, created.Email, found.Email)
}

func TestFindUserByEmail_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := mustOpenTestDB(t)

	found, err := s.FindUserByEmail(ctx, "nobody@example.com")
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestFindUserByEmail_CaseSensitive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := mustOpenTestDB(t)

	_, err := createTestUser(ctx, t, s)
	require.NoError(t, err)

	// The store stores whatever email is given; normalization is the service's job.
	found, err := s.FindUserByEmail(ctx, "ADMIN@EXAMPLE.COM")
	require.NoError(t, err)
	assert.Nil(t, found, "store lookup is case-sensitive; service must normalize before calling")
}
