package store

import (
	"context"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/store/types"
)

func (s *Store) InsertAuditLog(ctx context.Context, in types.InsertAuditLogParams) error {
	if err := s.Query().InsertAuditLog(ctx, in); err != nil {
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.Operation("InsertAuditLog")))
	}
	return nil
}

func (s *Store) ListAuditLogs(ctx context.Context, in types.ListAuditLogsParams) ([]types.AuditLogRow, error) {
	rows, err := s.Query().ListAuditLogs(ctx, in)
	if err != nil {
		return nil, apperrors.Annotate(err, apperrors.WithOperation(apperrors.Operation("ListAuditLogs")))
	}
	return rows, nil
}
