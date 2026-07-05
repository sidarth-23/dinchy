package auth

import (
	"context"
	"database/sql"
	"strings"

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/events"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/eventbus"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
)

const (
	ssoSettingSourceEnv = "env"
	ssoSettingSourceDB  = "db"
)

func (s *Service) listSSOProviderSettings(ctx context.Context) ([]SSOProviderSettingOut, error) {
	settings, err := s.mergedSSOProviderSettings(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SSOProviderSettingOut, 0, len(settings))
	for _, setting := range settings {
		out = append(out, ssoProviderSettingOut(setting))
	}
	return out, nil
}

func (s *Service) updateSSOProviderSetting(ctx context.Context, providerID string, body SSOProviderSettingUpdateBody) (SSOProviderSettingOut, error) {
	if !config.IsSupportedSSOProvider(providerID) {
		return SSOProviderSettingOut{}, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOProviderNotFound, i18n.P("provider", providerID)))
	}
	current, err := s.mergedSSOProviderSetting(ctx, providerID)
	if err != nil {
		return SSOProviderSettingOut{}, err
	}
	if body.ClientID != nil && current.ClientIDSource == ssoSettingSourceEnv {
		return SSOProviderSettingOut{}, apperrors.Forbidden(i18n.Msg(i18n.CodeAuthSSOFieldManagedByEnv))
	}
	if body.ClientSecret != nil && current.SecretSource == ssoSettingSourceEnv {
		return SSOProviderSettingOut{}, apperrors.Forbidden(i18n.Msg(i18n.CodeAuthSSOFieldManagedByEnv))
	}
	if body.CallbackURL != nil && current.CallbackURLSource == ssoSettingSourceEnv {
		return SSOProviderSettingOut{}, apperrors.Forbidden(i18n.Msg(i18n.CodeAuthSSOFieldManagedByEnv))
	}
	existingDB, err := s.dbSSOProviderSettings(ctx)
	if err != nil {
		return SSOProviderSettingOut{}, err
	}
	dbSetting := existingDB[providerID]
	clientID, clientIDValid := dbSetting.ClientID, dbSetting.ClientIDValid
	secret, secretValid := dbSetting.Secret, dbSetting.SecretValid
	callbackURL, callbackURLValid := dbSetting.CallbackURL, dbSetting.CallbackValid
	enabled := dbSetting.Enabled
	if body.ClientID != nil {
		clientID = strings.TrimSpace(*body.ClientID)
		clientIDValid = clientID != ""
	}
	if body.ClientSecret != nil {
		secret = strings.TrimSpace(*body.ClientSecret)
		secretValid = secret != ""
	}
	if body.CallbackURL != nil {
		callbackURL = strings.TrimSpace(*body.CallbackURL)
		callbackURLValid = callbackURL != ""
	}
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	if enabled && s.cache == nil {
		return SSOProviderSettingOut{}, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOCacheRequired))
	}
	if err := s.store.UpsertSSOProviderSetting(ctx, sqlcgen.UpsertSSOProviderSettingParams{
		ProviderID:   providerID,
		ClientID:     sql.NullString{String: clientID, Valid: clientIDValid},
		ClientSecret: sql.NullString{String: secret, Valid: secretValid},
		CallbackUrl:  sql.NullString{String: callbackURL, Valid: callbackURLValid},
		Enabled:      enabled,
		CreatedAt:    s.clock.Now().UTC(),
		UpdatedAt:    s.clock.Now().UTC(),
	}); err != nil {
		return SSOProviderSettingOut{}, err
	}
	updated, err := s.mergedSSOProviderSetting(ctx, providerID)
	if err != nil {
		return SSOProviderSettingOut{}, err
	}
	if err := s.publishEvent(ctx, eventbus.Event{
		Category:      "security",
		Subcategory:   "sso_settings",
		EventType:     string(events.AuthSecuritySSOSettingsUpdated),
		Action:        "update_sso_settings",
		Outcome:       "succeeded",
		TargetType:    "sso_provider",
		TargetID:      providerID,
		TargetDisplay: providerID,
		Changes: events.AuthSecuritySSOSettingsUpdatedChanges{
			ClientID:     body.ClientID != nil,
			ClientSecret: body.ClientSecret != nil,
			CallbackURL:  body.CallbackURL != nil,
			Enabled:      body.Enabled != nil,
		}.Map(),
	}); err != nil {
		return SSOProviderSettingOut{}, err
	}
	return ssoProviderSettingOut(updated), nil
}

func (s *Service) effectiveSSOProviderConfig(ctx context.Context, providerID string) (config.SSOProviderConfig, bool, error) {
	settings, err := s.mergedSSOProviderSettings(ctx)
	if err != nil {
		return config.SSOProviderConfig{}, false, err
	}
	for _, setting := range settings {
		if setting.ID != providerID {
			continue
		}
		cfg := config.SSOProviderConfig{
			ID:          config.SSOProviderID(setting.ID),
			Name:        setting.Name,
			ClientID:    setting.ClientID,
			Secret:      setting.Secret,
			CallbackURL: setting.CallbackURL,
			Enabled:     setting.Enabled && setting.ClientID != "" && setting.Secret != "" && setting.CallbackURL != "",
		}
		return cfg, true, nil
	}
	return config.SSOProviderConfig{}, false, nil
}

func (s *Service) effectiveSSOProviderConfigs(ctx context.Context) ([]config.SSOProviderConfig, error) {
	settings, err := s.mergedSSOProviderSettings(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]config.SSOProviderConfig, 0, len(settings))
	for _, setting := range settings {
		out = append(out, config.SSOProviderConfig{
			ID:          config.SSOProviderID(setting.ID),
			Name:        setting.Name,
			ClientID:    setting.ClientID,
			Secret:      setting.Secret,
			CallbackURL: setting.CallbackURL,
			Enabled:     setting.Enabled && setting.ClientID != "" && setting.Secret != "" && setting.CallbackURL != "",
		})
	}
	return out, nil
}
