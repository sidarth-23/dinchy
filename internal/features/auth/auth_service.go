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
}

func NewService(s Store, idg *id.Generator, clk clock.Clock, authConfig config.AuthConfig, providers []config.SSOProviderConfig, cacheStore cachecore.Store, cacheKeyer cachecore.Keyer, sender email.Sender) (*Service, error) {
	registry, err := newSSORegistry(authConfig, providers, cacheKeyer)
	if err != nil {
		return nil, err
	}
	if sender == nil {
		sender = email.NoopSender{}
	}
	return &Service{store: s, idg: idg, clock: clk, authConfig: authConfig, sso: registry, cache: cacheStore, email: sender}, nil
}

func (s *Service) OrganisationsForUser(ctx context.Context, userID string) ([]Organisation, error) {
	return s.store.ListOrganisationsForUser(ctx, userID)
}
