package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/store/core"
	"github.com/sidarth-23/dinchy/internal/store/sqlite/sqlcgen"
)

type queries struct {
	q *sqlcgen.Queries
}

func newQueries(q *sqlcgen.Queries) core.Queries {
	return &queries{q: q}
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(v string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func nullString(v string) sql.NullString {
	if v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}

func nullStringValid(v string, valid bool) sql.NullString {
	if !valid {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}

func nullStringTime(t time.Time, valid bool) sql.NullString {
	if !valid {
		return sql.NullString{}
	}
	return sql.NullString{String: formatTime(t), Valid: true}
}

func parseNullTime(v sql.NullString, field string) (time.Time, bool, error) {
	if !v.Valid {
		return time.Time{}, false, nil
	}
	t, err := parseTime(v.String)
	if err != nil {
		return time.Time{}, false, wrapParseErr(field, err)
	}
	return t, true, nil
}

func wrapParseErr(field string, err error) error {
	return apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("invalid sqlite timestamp for %s: %w", field, err)))
}
