package auth

import (
	"context"
	"strings"

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
)

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
	supportedProviders := config.SupportedSSOProviders()
	out := make([]mergedSSOProviderSetting, 0, len(supportedProviders))
	for _, provider := range supportedProviders {
		providerID := string(provider.ID)
		envSetting := s.sso.envProviders[providerID]
		dbSetting := dbSettings[providerID]
		setting := mergedSSOProviderSetting{ID: providerID, Name: provider.Name}
		setting.ClientID, setting.ClientIDSource = mergeSSOField(envSetting.ClientID, dbSetting.ClientID, dbSetting.ClientIDValid)
		setting.Secret, setting.SecretSource = mergeSSOField(envSetting.Secret, dbSetting.Secret, dbSetting.SecretValid)
		setting.CallbackURL, setting.CallbackURLSource = mergeSSOField(envSetting.CallbackURL, dbSetting.CallbackURL, dbSetting.CallbackValid)
		setting.Enabled = dbSetting.Enabled
		if envSetting.Enabled {
			setting.Enabled = true
			setting.EnabledSource = ssoSettingSourceEnv
		} else if _, ok := dbSettings[providerID]; ok {
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
