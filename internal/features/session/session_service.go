package session

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	goredis "github.com/redis/go-redis/v9"

	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/features"
	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
	"github.com/sidarth-23/dinchy/internal/foundation/id"
	"github.com/sidarth-23/dinchy/internal/foundation/security"
	"github.com/sidarth-23/dinchy/internal/platform/cache"
	"github.com/sidarth-23/dinchy/internal/platform/logging"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
)

// sessionCacheNamespace scopes cached session principals. It is the only place
// the cache namespace literal appears.
const sessionCacheNamespace = "session"

//go:generate go tool mockgen -self_package=github.com/sidarth-23/dinchy/internal/features/session -destination=store_mockdata_test.go -package=session . Store

// Store is the persistence surface the session service depends on.
type Store interface {
	InsertSession(ctx context.Context, arg sqlcgen.InsertSessionParams) error
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (sqlcgen.GetSessionByTokenHashRow, error)
	GetActiveSessionTokenHashesForUser(ctx context.Context, userID uuid.UUID) ([]string, error)
	GetActiveSessionTokenHashesForOrganization(ctx context.Context, activeOrganizationID uuid.UUID) ([]string, error)
	RevokeSessionByTokenHash(ctx context.Context, arg sqlcgen.RevokeSessionByTokenHashParams) error
	RevokeSessionsForUser(ctx context.Context, arg sqlcgen.RevokeSessionsForUserParams) error
}

// Service owns session token creation, resolution, revocation, and cookie naming.
type Service struct {
	*features.Service
	store         Store
	config        config.SessionConfig
	principals    cache.Entry[cachedPrincipal]
	sessionTTLCap time.Duration
}

// NewService builds a session Service. A nil or disabled cache makes session
// resolution read straight through to the store.
func NewService(base *features.Service, store Store, sessionConfig config.SessionConfig, cacheConfig config.CacheConfig) (*Service, error) {
	if base == nil {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodePlatformServerInternalError), apperrors.WithCause(errors.New("session module service is required")))
	}
	if err := base.Initialize(); err != nil {
		return nil, apperrors.Annotate(err)
	}
	var cacheClient *goredis.Client
	if cacheConfig.Enabled {
		cacheClient = base.RedisClient
	}
	return &Service{
		Service:       base,
		store:         store,
		config:        sessionConfig,
		principals:    cache.NewEntry[cachedPrincipal](cacheClient, base.CacheKeyer, sessionCacheNamespace, cacheConfig.SessionTTLCap),
		sessionTTLCap: cacheConfig.SessionTTLCap,
	}, nil
}

var _ features.Module = (*Service)(nil)

// SessionCookieName returns the configured name of the session cookie.
func (s *Service) SessionCookieName() string {
	return s.config.SessionCookieName
}

// Session resolves a raw token to its active session, returning nil when the token is empty, unknown, revoked, or expired.
func (s *Service) Session(ctx context.Context, rawToken string) (*Principal, error) {
	if rawToken == "" {
		return nil, nil
	}
	hash := security.HashToken(rawToken)
	now := s.Clock.Now()

	if cached, hit, err := s.principals.Get(ctx, hash); err != nil {
		logging.Warn(ctx, s.Logger(ctx), "Session cache read failed, falling back to database", slog.Any("error", err))
	} else if hit {
		principal := cached.toPrincipal()
		if s.expired(principal, now) {
			return nil, nil
		}
		return principal, nil
	}

	row, err := s.store.GetSessionByTokenHash(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsSessionGetSession), apperrors.WithCause(err))
	}
	principal := FromGetSessionRow(row)
	if principal.RevokedAt.Valid || s.expired(principal, now) {
		return nil, nil
	}
	if ttl := s.cacheTTL(principal, now); ttl > 0 {
		if err := s.principals.SetWithTTL(ctx, hash, principal.toCache(), ttl); err != nil {
			logging.Warn(ctx, s.Logger(ctx), "Session cache write failed", slog.Any("error", err))
		}
	}
	return principal, nil
}

func (s *Service) expired(p *Principal, now time.Time) bool {
	return now.After(p.IdleExpiresAt) || now.After(p.ExpiresAt)
}

func (s *Service) cacheTTL(p *Principal, now time.Time) time.Duration {
	return min(p.IdleExpiresAt.Sub(now), s.sessionTTLCap)
}

// Create issues a new session token for the given user and organization.
func (s *Service) Create(ctx context.Context, userID, organizationID, ip, userAgent string) (string, error) {
	token := s.IDGenerator.New()
	now := s.Clock.Now().UTC()
	if err := s.store.InsertSession(ctx, sqlcgen.InsertSessionParams{
		ID:                   id.MustParse(token),
		UserID:               id.MustParse(userID),
		ActiveOrganizationID: id.MustParse(organizationID),
		TokenHash:            security.HashToken(token),
		IpAddress:            ip,
		UserAgent:            userAgent,
		LastSeenAt:           sqltype.Timestamptz(now),
		IdleExpiresAt:        sqltype.Timestamptz(now.Add(s.config.SessionIdleTimeout)),
		ExpiresAt:            sqltype.Timestamptz(now.Add(s.config.SessionMaxLifetime)),
		CreatedAt:            sqltype.Timestamptz(now),
		UpdatedAt:            sqltype.Timestamptz(now),
	}); err != nil {
		return "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsSessionCreateSession), apperrors.WithCause(err))
	}
	return token, nil
}

// Logout revokes the session identified by the raw token and returns the resolved principal.
func (s *Service) Logout(ctx context.Context, rawToken string) (*Principal, error) {
	if rawToken == "" {
		return nil, nil
	}
	principal, err := s.Session(ctx, rawToken)
	if err != nil {
		return nil, err
	}
	now := s.Clock.Now()
	hash := security.HashToken(rawToken)
	if err := s.store.RevokeSessionByTokenHash(ctx, sqlcgen.RevokeSessionByTokenHashParams{RevokedAt: sqltype.Timestamptz(now), UpdatedAt: sqltype.Timestamptz(now), TokenHash: hash}); err != nil {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsSessionRevokeSession), apperrors.WithCause(err))
	}
	if err := s.principals.Delete(ctx, hash); err != nil {
		logging.Warn(ctx, s.Logger(ctx), "Session cache invalidation failed after logout", slog.Any("error", err))
	}
	return principal, nil
}

// RevokeForUser revokes every active session for the given user.
func (s *Service) RevokeForUser(ctx context.Context, userID string) error {
	uid := id.MustParse(userID)
	hashes := s.activeHashesForInvalidation(ctx, func(ctx context.Context) ([]string, error) {
		return s.store.GetActiveSessionTokenHashesForUser(ctx, uid)
	})
	now := s.Clock.Now()
	if err := s.store.RevokeSessionsForUser(ctx, sqlcgen.RevokeSessionsForUserParams{UserID: uid, RevokedAt: sqltype.Timestamptz(now), UpdatedAt: sqltype.Timestamptz(now)}); err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsSessionRevokeSessionsForUser), apperrors.WithCause(err))
	}
	if err := s.principals.Delete(ctx, hashes...); err != nil {
		logging.Warn(ctx, s.Logger(ctx), "Session cache invalidation failed after user revocation", slog.Any("error", err))
	}
	return nil
}

// InvalidateForUser drops cached principals for every active session of the user.
// Call this whenever a change to the user (profile, membership, role) alters the
// resolved principal without revoking the underlying sessions.
func (s *Service) InvalidateForUser(ctx context.Context, userID string) error {
	if !s.principals.Enabled() {
		return nil
	}
	hashes, err := s.store.GetActiveSessionTokenHashesForUser(ctx, id.MustParse(userID))
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsSessionGetSession), apperrors.WithCause(err))
	}
	return s.principals.Delete(ctx, hashes...)
}

// InvalidateForOrganization drops cached principals for every active session in
// the organization. Call this whenever an organization-wide change (role
// permissions, organization name or slug) alters the resolved principal.
func (s *Service) InvalidateForOrganization(ctx context.Context, organizationID string) error {
	if !s.principals.Enabled() {
		return nil
	}
	hashes, err := s.store.GetActiveSessionTokenHashesForOrganization(ctx, id.MustParse(organizationID))
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsSessionGetSession), apperrors.WithCause(err))
	}
	return s.principals.Delete(ctx, hashes...)
}

// activeHashesForInvalidation loads token hashes for best-effort cache purging.
// A load failure is non-fatal: it is logged and the caller proceeds, since the
// cached principals expire within the TTL cap.
func (s *Service) activeHashesForInvalidation(ctx context.Context, load func(context.Context) ([]string, error)) []string {
	if !s.principals.Enabled() {
		return nil
	}
	hashes, err := load(ctx)
	if err != nil {
		logging.Warn(ctx, s.Logger(ctx), "Loading session token hashes for cache invalidation failed", slog.Any("error", err))
		return nil
	}
	return hashes
}
