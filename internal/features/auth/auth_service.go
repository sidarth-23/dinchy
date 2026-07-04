// Package auth handles password hashing, session issuance, and session validation.
package auth

import (
	"context"
	"database/sql"

	"github.com/sidarth-23/dinchy/internal/config"
	cachecore "github.com/sidarth-23/dinchy/internal/platform/cache/core"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/email"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
)

type Service struct {
	db         *sql.DB
	beginTx    func(context.Context) (*setupTransaction, error)
	store      Store
	idg        *id.Generator
	clock      clock.Clock
	authConfig config.AuthConfig
	sso        *ssoRegistry
	cache      cachecore.Store
	email      email.Sender
	audit      AuditRecorder
}

func NewService(db *sql.DB, s Store, idg *id.Generator, clk clock.Clock, authConfig config.AuthConfig, providers []config.SSOProviderConfig, cacheStore cachecore.Store, cacheKeyer cachecore.Keyer, sender email.Sender, auditors ...AuditRecorder) (*Service, error) {
	registry, err := newSSORegistry(authConfig, providers, cacheKeyer)
	if err != nil {
		return nil, err
	}
	if sender == nil {
		sender = email.NoopSender{}
	}
	var audit AuditRecorder
	if len(auditors) > 0 {
		audit = auditors[0]
	}
	service := &Service{db: db, store: s, idg: idg, clock: clk, authConfig: authConfig, sso: registry, cache: cacheStore, email: sender, audit: audit}
	if db != nil {
		service.beginTx = func(ctx context.Context) (*setupTransaction, error) {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				return nil, err
			}
			return &setupTransaction{
				queries:  sqlcgen.New(tx),
				commit:   tx.Commit,
				rollback: tx.Rollback,
			}, nil
		}
	}
	return service, nil
}

func (s *Service) OrganisationsForUser(ctx context.Context, userID string) ([]Organisation, error) {
	rows, err := s.store.ListOrganisationsForUser(ctx, mustParseUUID(userID))
	if err != nil {
		return nil, err
	}
	out := make([]Organisation, 0, len(rows))
	for _, row := range rows {
		out = append(out, organisationFromListOrganisationRow(row))
	}
	return out, nil
}

func (s *Service) Bootstrap(ctx context.Context) (BootstrapState, error) {
	count, err := s.store.CountUsers(ctx)
	if err != nil {
		return BootstrapState{}, err
	}
	name, err := s.store.GetInstanceName(ctx)
	if err != nil {
		return BootstrapState{}, err
	}
	return BootstrapState{SetupRequired: count == 0, InstanceName: name}, nil
}

func (s *Service) recordAudit(ctx context.Context, event AuditEvent) error {
	if s.audit == nil {
		return nil
	}
	return s.audit.RecordAuthEvent(ctx, event)
}
