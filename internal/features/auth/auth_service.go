// Package auth handles password hashing, session issuance, and session validation.
package auth

import (
	"context"

	cachecore "github.com/sidarth-23/dinchy/internal/cache/core"
	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/email"
	"github.com/sidarth-23/dinchy/internal/platform/id"
)

type Service struct {
	store      Store
	idg        *id.Generator
	clock      clock.Clock
	authConfig config.AuthConfig
	sso        *ssoRegistry
	cache      cachecore.Store
	email      email.Sender
	audit      AuditRecorder
}

func NewService(s Store, idg *id.Generator, clk clock.Clock, authConfig config.AuthConfig, providers []config.SSOProviderConfig, cacheStore cachecore.Store, cacheKeyer cachecore.Keyer, sender email.Sender, auditors ...AuditRecorder) (*Service, error) {
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
	return &Service{store: s, idg: idg, clock: clk, authConfig: authConfig, sso: registry, cache: cacheStore, email: sender, audit: audit}, nil
}

func (s *Service) OrganisationsForUser(ctx context.Context, userID string) ([]Organisation, error) {
	return s.store.ListOrganisationsForUser(ctx, userID)
}

func (s *Service) recordAudit(ctx context.Context, event AuditEvent) error {
	if s.audit == nil {
		return nil
	}
	return s.audit.RecordAuthEvent(ctx, event)
}
