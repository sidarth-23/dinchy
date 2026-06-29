// Package auth handles password hashing, session issuance, and session validation.
package auth

import (
	"context"
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
	email      email.Sender
}

func NewService(s Store, idg *id.Generator, clk clock.Clock, authConfig config.AuthConfig, providers []config.SSOProviderConfig, sender email.Sender) (*Service, error) {
	registry, err := newSSORegistry(authConfig, providers)
	if err != nil {
		return nil, err
	}
	if sender == nil {
		sender = email.NoopSender{}
	}
	return &Service{store: s, idg: idg, clock: clk, authConfig: authConfig, sso: registry, email: sender}, nil
}

func (s *Service) OrganisationsForUser(ctx context.Context, userID string) ([]Organisation, error) {
	return s.store.ListOrganisationsForUser(ctx, userID)
}
