// Package session owns authenticated request principals and session lifecycle contracts.
package session

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sidarth-23/dinchy/internal/access/permission"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
	"github.com/sidarth-23/dinchy/internal/transport/support"
)

type contextKey int

const (
	principalContextKey contextKey = iota
	resolutionErrorContextKey
)

// Principal is the authenticated user and active organization for a request.
type Principal struct {
	SessionID        string
	UserID           string
	Email            string
	DisplayName      string
	OrganisationID   string
	OrganisationName string
	OrganisationSlug string
	Role             permission.Role
	Permissions      []permission.Permission
	IdleExpiresAt    time.Time
	ExpiresAt        time.Time
	RevokedAt        pgtype.Timestamptz
}

// FromGetSessionRow builds a principal from a session query row.
func FromGetSessionRow(row sqlcgen.GetSessionByTokenHashRow) *Principal {
	permissions := make([]permission.Permission, 0, len(row.Permissions))
	for _, granted := range row.Permissions {
		permissions = append(permissions, permission.Permission(granted))
	}
	principal := Principal{
		SessionID:        row.ID.String(),
		UserID:           row.UserID.String(),
		Email:            row.Email,
		DisplayName:      row.DisplayName,
		OrganisationID:   row.ActiveOrganisationID.String(),
		OrganisationName: row.OrganisationName,
		OrganisationSlug: row.OrganisationSlug,
		Role:             permission.Role(row.Role),
		Permissions:      permissions,
		IdleExpiresAt:    sqltype.TimeValue(row.IdleExpiresAt),
		ExpiresAt:        sqltype.TimeValue(row.ExpiresAt),
		RevokedAt:        row.RevokedAt,
	}
	return &principal
}

// HasPermission reports whether the principal has a granted permission.
func (p *Principal) HasPermission(value permission.Permission) bool {
	for _, granted := range p.Permissions {
		if granted == value {
			return true
		}
	}
	return false
}

// WithPrincipal adds a principal to a request context.
func WithPrincipal(ctx context.Context, principal *Principal) context.Context {
	return context.WithValue(ctx, principalContextKey, principal)
}

// PrincipalFrom returns the principal stored in a request context.
func PrincipalFrom(ctx context.Context) *Principal {
	principal, _ := ctx.Value(principalContextKey).(*Principal)
	return principal
}

// WithResolutionError adds a session-resolution failure to a request context.
func WithResolutionError(ctx context.Context, err error) context.Context {
	return context.WithValue(ctx, resolutionErrorContextKey, err)
}

// ResolutionErrorFrom returns a session-resolution failure stored in a request context.
func ResolutionErrorFrom(ctx context.Context) error {
	err, _ := ctx.Value(resolutionErrorContextKey).(error)
	return err
}

// SessionCookie builds a session cookie carrying the given token, marking it secure when requested.
func SessionCookie(name, token string, secure bool) *http.Cookie {
	return support.ValueCookie(name, token, secure)
}

// ClearSessionCookie builds a cookie that clears a session cookie on the client.
func ClearSessionCookie(name string, secure bool) *http.Cookie {
	return support.ClearCookie(name, secure)
}
