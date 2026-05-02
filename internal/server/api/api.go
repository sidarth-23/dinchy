package api

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/sidarth-23/dinchy/internal/auth"
	"github.com/sidarth-23/dinchy/internal/domain"
)

// API groups the handler methods and their shared service dependencies.
type API struct {
	auth         *auth.Service
	settings     domain.SettingsReader
	requireHTTPS bool
}

// Register mounts all Phase 1 API operations on the given huma.API instance.
func Register(h huma.API, svc *auth.Service, sr domain.SettingsReader, requireHTTPS bool) {
	a := &API{auth: svc, settings: sr, requireHTTPS: requireHTTPS}
	a.registerBootstrap(h)
	a.registerSetup(h)
	a.registerAuth(h)
}
