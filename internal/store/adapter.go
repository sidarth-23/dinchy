package store

import (
	"database/sql"

	"github.com/google/uuid"

	"github.com/sidarth-23/dinchy/internal/store/core"
	"github.com/sidarth-23/dinchy/internal/store/postgres/sqlcgen"
)

type queries struct {
	q *sqlcgen.Queries
}

func newQueries(q *sqlcgen.Queries) core.Queries {
	return &queries{q: q}
}

func nullString(v string) sql.NullString {
	if v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}
