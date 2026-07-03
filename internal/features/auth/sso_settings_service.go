package auth

import (
	"context"
	"strings"

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
)

const (
	ssoSettingSourceEnv = "env"
	ssoSettingSourceDB  = "db"
)

type mergedSSOProviderSetting struct {
	ID                string
	Name              string
	ClientID          string
	ClientIDSource    string
	Secret            string
	SecretSource      string
	CallbackURL       string
	CallbackURLSource string
	Enabled           bool
	EnabledSource     string
}

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
	if !supportedSSOProvider(providerID) {
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
	if err := s.store.UpsertSSOProviderSetting(ctx, UpsertSSOProviderSettingInput{
		ProviderID:    providerID,
		ClientID:      clientID,
		ClientIDValid: clientIDValid,
		Secret:        secret,
		SecretValid:   secretValid,
		CallbackURL:   callbackURL,
		CallbackValid: callbackURLValid,
		Enabled:       enabled,
		Now:           s.clock.Now(),
	}); err != nil {
		return SSOProviderSettingOut{}, err
	}
	updated, err := s.mergedSSOProviderSetting(ctx, providerID)
	if err != nil {
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

func (s *Service) mergedSSOProviderSetting(ctx context.Context, providerID string) (mergedSSOProviderSetting, error) {
	settings, err := s.mergedSSOProviderSettings(ctx)
	if err != nil {
		return mergedSSOProviderSetting{}, err
	}
	for _, setting := range settings {
		if setting.ID == providerID {
			return setting, nil
		}
	}
	return mergedSSOProviderSetting{}, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOProviderNotFound, i18n.P("provider", providerID)))
}

func (s *Service) mergedSSOProviderSettings(ctx context.Context) ([]mergedSSOProviderSetting, error) {
	dbSettings, err := s.dbSSOProviderSettings(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]mergedSSOProviderSetting, 0, len(supportedSSOProviders))
	for _, provider := range supportedSSOProviders {
		envSetting := s.sso.envProviders[provider.ID]
		dbSetting := dbSettings[provider.ID]
		setting := mergedSSOProviderSetting{ID: provider.ID, Name: provider.Name}
		setting.ClientID, setting.ClientIDSource = mergeSSOField(envSetting.ClientID, dbSetting.ClientID, dbSetting.ClientIDValid)
		setting.Secret, setting.SecretSource = mergeSSOField(envSetting.Secret, dbSetting.Secret, dbSetting.SecretValid)
		setting.CallbackURL, setting.CallbackURLSource = mergeSSOField(envSetting.CallbackURL, dbSetting.CallbackURL, dbSetting.CallbackValid)
		setting.Enabled = dbSetting.Enabled
		if envSetting.Enabled {
			setting.Enabled = true
			setting.EnabledSource = ssoSettingSourceEnv
		} else if _, ok := dbSettings[provider.ID]; ok {
			setting.EnabledSource = ssoSettingSourceDB
		}
		out = append(out, setting)
	}
	return out, nil
}

func (s *Service) dbSSOProviderSettings(ctx context.Context) (map[string]SSOProviderSetting, error) {
	settings, err := s.store.ListSSOProviderSettings(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]SSOProviderSetting, len(settings))
	for _, setting := range settings {
		out[setting.ProviderID] = setting
	}
	return out, nil
}

func mergeSSOField(envValue, dbValue string, dbValid bool) (string, string) {
	envValue = strings.TrimSpace(envValue)
	if envValue != "" {
		return envValue, ssoSettingSourceEnv
	}
	if dbValid {
		return strings.TrimSpace(dbValue), ssoSettingSourceDB
	}
	return "", ""
}

func ssoProviderSettingOut(setting mergedSSOProviderSetting) SSOProviderSettingOut {
	return SSOProviderSettingOut{
		ID:            setting.ID,
		Name:          setting.Name,
		ClientID:      ssoProviderFieldOut(setting.ClientID, setting.ClientIDSource),
		ClientSecret:  ssoProviderFieldOut(setting.Secret, setting.SecretSource),
		CallbackURL:   ssoProviderFieldOut(setting.CallbackURL, setting.CallbackURLSource),
		Enabled:       setting.Enabled,
		EnabledSource: setting.EnabledSource,
	}
}

func ssoProviderFieldOut(value, source string) SSOProviderFieldOut {
	return SSOProviderFieldOut{Set: strings.TrimSpace(value) != "", Source: source}
}
