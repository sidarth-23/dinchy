package audit

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/events"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/module"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
)

const testAuditTime = 1_735_689_600

func newTestService(t *testing.T) *Service {
	t.Helper()
	base := (&module.Service{Clock: clock.Fixed(time.Unix(testAuditTime, 0).UTC())}).Named("audit")
	svc, err := NewService(base, &failingStore{t: t})
	require.NoError(t, err)
	return svc
}

type failingStore struct {
	t *testing.T
}

func (s *failingStore) InsertAuditLog(context.Context, sqlcgen.InsertAuditLogParams) error {
	s.t.Helper()
	s.t.Fatalf("unexpected InsertAuditLog call")
	return nil
}

func (s *failingStore) ListAuditLogs(context.Context, sqlcgen.ListAuditLogsParams) ([]sqlcgen.AppAuditLog, error) {
	s.t.Helper()
	s.t.Fatalf("unexpected ListAuditLogs call")
	return nil, nil
}

func TestInsertParams_MalformedEventIDReturnsError(t *testing.T) {
	const badID = "not-a-uuid"

	_, err := insertParams(events.Record{
		ID:        badID,
		EventType: "audit.event.created",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), badID)
}

func TestServiceName(t *testing.T) {
	assert.Equal(t, "audit", newTestService(t).Name())
}

func TestHandle_MalformedEventIDReturnsErrorWithoutPanicking(t *testing.T) {
	svc := newTestService(t)
	const badID = "not-a-uuid"

	var gotErr error
	require.NotPanics(t, func() {
		gotErr = svc.Handle(context.Background(), events.Record{
			ID:        badID,
			EventType: "audit.event.created",
		})
	})

	require.Error(t, gotErr)
	assert.Contains(t, gotErr.Error(), badID)
}

func TestList_MalformedActorUserIDReturnsValidationAppError(t *testing.T) {
	svc := newTestService(t)
	const badActorUserID = "not-a-uuid"

	var (
		logs   []events.Record
		gotErr error
	)
	require.NotPanics(t, func() {
		logs, gotErr = svc.List(context.Background(), ListInput{ActorUserID: badActorUserID})
	})

	require.Error(t, gotErr)
	assert.Nil(t, logs)

	var appErr *apperrors.AppError
	require.ErrorAs(t, gotErr, &appErr)
	assert.Equal(t, http.StatusUnprocessableEntity, appErr.Status())
	assert.Equal(t, i18n.CodeRequestValidationFailed, appErr.Code())
	assert.Equal(t, map[string]any{string(apperrors.MetaKeyFieldName): "actor_user_id"}, appErr.Meta())
}
