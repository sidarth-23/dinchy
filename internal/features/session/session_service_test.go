package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/features"
	"github.com/sidarth-23/dinchy/internal/features/session"
	"github.com/sidarth-23/dinchy/internal/foundation/clock"
	"github.com/sidarth-23/dinchy/internal/foundation/id"
	"github.com/sidarth-23/dinchy/internal/foundation/permission"
	"github.com/sidarth-23/dinchy/internal/foundation/security"
	"github.com/sidarth-23/dinchy/internal/platform/cache"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
)

var (
	fixedTime    = time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	testUserID   = "00000000-0000-0000-0000-000000000001"
	testOrgID    = "00000000-0000-0000-0000-000000000002"
	testSessID   = "00000000-0000-0000-0000-000000000003"
	testRawToken = "raw-token"
)

type fakeStore struct {
	row            sqlcgen.GetSessionByTokenHashRow
	getCalls       int
	userHashes     []string
	orgHashes      []string
	revokedForUser int
}

func (f *fakeStore) InsertSession(context.Context, sqlcgen.InsertSessionParams) error { return nil }

func (f *fakeStore) GetSessionByTokenHash(context.Context, string) (sqlcgen.GetSessionByTokenHashRow, error) {
	f.getCalls++
	return f.row, nil
}

func (f *fakeStore) GetActiveSessionTokenHashesForUser(context.Context, uuid.UUID) ([]string, error) {
	return f.userHashes, nil
}

func (f *fakeStore) GetActiveSessionTokenHashesForOrganization(context.Context, uuid.UUID) ([]string, error) {
	return f.orgHashes, nil
}

func (f *fakeStore) RevokeSessionByTokenHash(context.Context, sqlcgen.RevokeSessionByTokenHashParams) error {
	return nil
}

func (f *fakeStore) RevokeSessionsForUser(context.Context, sqlcgen.RevokeSessionsForUserParams) error {
	f.revokedForUser++
	return nil
}

func validRow(revoked pgtype.Timestamptz) sqlcgen.GetSessionByTokenHashRow {
	return sqlcgen.GetSessionByTokenHashRow{
		ID:                   id.MustParse(testSessID),
		UserID:               id.MustParse(testUserID),
		Email:                "user@example.com",
		DisplayName:          "User",
		ActiveOrganizationID: id.MustParse(testOrgID),
		OrganizationName:     "Default",
		OrganizationSlug:     "default",
		Role:                 string(permission.RoleAdmin),
		Permissions:          []string{"audit.logs.read"},
		IdleExpiresAt:        sqltype.Timestamptz(fixedTime.Add(time.Hour)),
		ExpiresAt:            sqltype.Timestamptz(fixedTime.Add(24 * time.Hour)),
		RevokedAt:            revoked,
	}
}

func newRedis(t *testing.T) (*goredis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return client, mr
}

func newService(t *testing.T, store session.Store, client *goredis.Client) *session.Service {
	t.Helper()
	base := (&features.Service{Clock: clock.Fixed(fixedTime), IDGenerator: id.NewGenerator(), RedisClient: client, CacheKeyer: cache.NewKeyer("dinchy")}).Named("session")
	service, err := session.NewService(base, store, config.DefaultSession(), config.DefaultCache())
	require.NoError(t, err)
	return service
}

func TestServiceName(t *testing.T) {
	assert.Equal(t, "session", newService(t, &fakeStore{}, nil).Name())
}

func TestSession_CachesAfterFirstLookup(t *testing.T) {
	t.Parallel()
	store := &fakeStore{row: validRow(pgtype.Timestamptz{})}
	client, _ := newRedis(t)
	svc := newService(t, store, client)
	ctx := context.Background()

	first, err := svc.Session(ctx, testRawToken)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 1, store.getCalls)

	second, err := svc.Session(ctx, testRawToken)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 1, store.getCalls, "second lookup should be served from cache")
	assert.Equal(t, first.UserID, second.UserID)
	assert.Equal(t, first.OrganizationSlug, second.OrganizationSlug)
	assert.Equal(t, []permission.Permission{permission.Permission("audit.logs.read")}, second.Permissions)
}

func TestSession_DisabledCacheAlwaysHitsStore(t *testing.T) {
	t.Parallel()
	store := &fakeStore{row: validRow(pgtype.Timestamptz{})}
	svc := newService(t, store, nil)
	ctx := context.Background()

	_, err := svc.Session(ctx, testRawToken)
	require.NoError(t, err)
	_, err = svc.Session(ctx, testRawToken)
	require.NoError(t, err)
	assert.Equal(t, 2, store.getCalls)
}

func TestSession_DoesNotCacheRevoked(t *testing.T) {
	t.Parallel()
	store := &fakeStore{row: validRow(sqltype.Timestamptz(fixedTime))}
	client, _ := newRedis(t)
	svc := newService(t, store, client)
	ctx := context.Background()

	p, err := svc.Session(ctx, testRawToken)
	require.NoError(t, err)
	assert.Nil(t, p)
	_, err = svc.Session(ctx, testRawToken)
	require.NoError(t, err)
	assert.Equal(t, 2, store.getCalls, "revoked sessions must not be cached")
}

func TestSession_DoesNotCacheExpired(t *testing.T) {
	t.Parallel()
	row := validRow(pgtype.Timestamptz{})
	row.IdleExpiresAt = sqltype.Timestamptz(fixedTime.Add(-time.Minute))
	store := &fakeStore{row: row}
	client, _ := newRedis(t)
	svc := newService(t, store, client)
	ctx := context.Background()

	p, err := svc.Session(ctx, testRawToken)
	require.NoError(t, err)
	assert.Nil(t, p)
	_, err = svc.Session(ctx, testRawToken)
	require.NoError(t, err)
	assert.Equal(t, 2, store.getCalls, "expired sessions must not be cached")
}

func TestSession_TTLCapExpiresEntry(t *testing.T) {
	t.Parallel()
	store := &fakeStore{row: validRow(pgtype.Timestamptz{})}
	client, mr := newRedis(t)
	svc := newService(t, store, client)
	ctx := context.Background()

	_, err := svc.Session(ctx, testRawToken)
	require.NoError(t, err)
	assert.Equal(t, 1, store.getCalls)

	mr.FastForward(config.DefaultCache().SessionTTLCap + time.Second)

	_, err = svc.Session(ctx, testRawToken)
	require.NoError(t, err)
	assert.Equal(t, 2, store.getCalls, "entry should expire at the TTL cap")
}

func TestLogout_InvalidatesCache(t *testing.T) {
	t.Parallel()
	store := &fakeStore{row: validRow(pgtype.Timestamptz{})}
	client, _ := newRedis(t)
	svc := newService(t, store, client)
	ctx := context.Background()

	_, err := svc.Session(ctx, testRawToken)
	require.NoError(t, err)
	assert.Equal(t, 1, store.getCalls)

	_, err = svc.Logout(ctx, testRawToken)
	require.NoError(t, err)

	_, err = svc.Session(ctx, testRawToken)
	require.NoError(t, err)
	assert.Equal(t, 2, store.getCalls, "logout must purge the cached principal")
}

func TestRevokeForUser_InvalidatesCache(t *testing.T) {
	t.Parallel()
	store := &fakeStore{row: validRow(pgtype.Timestamptz{}), userHashes: []string{security.HashToken(testRawToken)}}
	client, _ := newRedis(t)
	svc := newService(t, store, client)
	ctx := context.Background()

	_, err := svc.Session(ctx, testRawToken)
	require.NoError(t, err)
	assert.Equal(t, 1, store.getCalls)

	require.NoError(t, svc.RevokeForUser(ctx, testUserID))
	assert.Equal(t, 1, store.revokedForUser)

	_, err = svc.Session(ctx, testRawToken)
	require.NoError(t, err)
	assert.Equal(t, 2, store.getCalls, "revoking a user's sessions must purge their cached principals")
}
