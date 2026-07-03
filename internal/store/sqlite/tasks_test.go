package sqlite_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/features/auth"
)

const testTaskName = "session_cleanup"

func TestStore_ClaimTask_LeaseContention(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.EnsureTask(testCtx, testTaskName, 300, fixedTime))

	lease := 15 * time.Second
	claimed, err := store.ClaimTask(testCtx, testTaskName, "owner-a", fixedTime.Add(lease), fixedTime)
	require.NoError(t, err)
	assert.True(t, claimed, "first claim should succeed on a due, unleased task")

	// A competing owner cannot claim while the lease is still active.
	contended, err := store.ClaimTask(testCtx, testTaskName, "owner-b", fixedTime.Add(2*lease), fixedTime.Add(5*time.Second))
	require.NoError(t, err)
	assert.False(t, contended, "claim should fail while the lease held by owner-a is active")

	// Once the lease has expired, the task can be reclaimed.
	reclaimed, err := store.ClaimTask(testCtx, testTaskName, "owner-b", fixedTime.Add(3*lease), fixedTime.Add(20*time.Second))
	require.NoError(t, err)
	assert.True(t, reclaimed, "claim should succeed after the previous lease expired")
}

func TestStore_ClaimTask_NotYetDue(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	// next_run_at is one hour in the future.
	require.NoError(t, store.EnsureTask(testCtx, testTaskName, 300, fixedTime.Add(time.Hour)))

	claimed, err := store.ClaimTask(testCtx, testTaskName, "owner", fixedTime.Add(15*time.Second), fixedTime)
	require.NoError(t, err)
	assert.False(t, claimed, "claim should fail before next_run_at is reached")
}

func TestStore_ClaimTask_UnknownTask(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	claimed, err := store.ClaimTask(testCtx, "does-not-exist", "owner", fixedTime.Add(15*time.Second), fixedTime)
	require.NoError(t, err)
	assert.False(t, claimed, "claiming an unregistered task should report no rows affected, not an error")
}

func TestStore_EnsureTask_Idempotent(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.EnsureTask(testCtx, testTaskName, 300, fixedTime))
	// A second EnsureTask must not reset the existing schedule (ON CONFLICT DO NOTHING).
	require.NoError(t, store.EnsureTask(testCtx, testTaskName, 999, fixedTime.Add(time.Hour)))

	claimed, err := store.ClaimTask(testCtx, testTaskName, "owner", fixedTime.Add(15*time.Second), fixedTime)
	require.NoError(t, err)
	assert.True(t, claimed, "task should remain due at its original next_run_at after a repeat EnsureTask")
}

func TestStore_FinishTask_ReschedulesAndClearsLease(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.EnsureTask(testCtx, testTaskName, 300, fixedTime))

	claimed, err := store.ClaimTask(testCtx, testTaskName, "owner", fixedTime.Add(15*time.Second), fixedTime)
	require.NoError(t, err)
	require.True(t, claimed)

	nextRun := fixedTime.Add(5 * time.Minute)
	require.NoError(t, store.FinishTask(testCtx, testTaskName, fixedTime, true, "", "", nextRun))

	// Before next_run_at the task is not due even though the lease was cleared.
	early, err := store.ClaimTask(testCtx, testTaskName, "owner", fixedTime.Add(2*time.Minute), fixedTime.Add(time.Minute))
	require.NoError(t, err)
	assert.False(t, early, "task should not be claimable before its rescheduled next_run_at")

	// After next_run_at it becomes claimable again.
	due, err := store.ClaimTask(testCtx, testTaskName, "owner", nextRun.Add(15*time.Second), nextRun.Add(time.Second))
	require.NoError(t, err)
	assert.True(t, due, "task should be claimable once the rescheduled next_run_at has passed")
}

func TestStore_DeleteEndedSessionsOlderThan(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	in := seedFirstUser(t, store)

	cutoff := fixedTime.Add(24 * time.Hour)

	// Ended and stale: expired before the cutoff and last updated before the cutoff.
	_, err := store.CreateSession(testCtx, auth.CreateSessionInput{
		ID:             "session-stale",
		UserID:         in.ID,
		OrganisationID: in.OrganisationID,
		TokenHash:      "stale",
		IP:             "127.0.0.1",
		UserAgent:      "test-agent",
		Now:            fixedTime,
		IdleExpiresAt:  fixedTime.Add(time.Minute),
		ExpiresAt:      fixedTime.Add(time.Hour),
	})
	require.NoError(t, err)

	// Active and recent: updated after the cutoff, so it must be retained.
	_, err = store.CreateSession(testCtx, auth.CreateSessionInput{
		ID:             "session-active",
		UserID:         in.ID,
		OrganisationID: in.OrganisationID,
		TokenHash:      "active",
		IP:             "127.0.0.1",
		UserAgent:      "test-agent",
		Now:            cutoff.Add(time.Hour),
		IdleExpiresAt:  cutoff.Add(2 * time.Hour),
		ExpiresAt:      cutoff.Add(30 * 24 * time.Hour),
	})
	require.NoError(t, err)

	deleted, err := store.DeleteEndedSessionsOlderThan(testCtx, cutoff)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	stale, err := store.GetSessionByTokenHash(testCtx, "stale")
	require.NoError(t, err)
	assert.Nil(t, stale, "stale ended session should have been deleted")

	active, err := store.GetSessionByTokenHash(testCtx, "active")
	require.NoError(t, err)
	assert.NotNil(t, active, "recent session should be retained")
}
