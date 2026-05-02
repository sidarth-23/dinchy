package domain

import "context"

// BootstrapState describes whether initial setup is required and the configured instance name.
type BootstrapState struct {
	SetupRequired bool
	InstanceName  string
}

// SettingsReader provides read access to bootstrap and application settings state.
type SettingsReader interface {
	Bootstrap(ctx context.Context) (BootstrapState, error)
}
