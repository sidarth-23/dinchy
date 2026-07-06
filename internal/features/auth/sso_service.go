package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/events"
	"github.com/sidarth-23/dinchy/internal/i18n"
	cachecore "github.com/sidarth-23/dinchy/internal/platform/cache/core"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/security"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/transform"
)

func (s *Service) listSSOProviders(ctx context.Context) ([]SSOProviderOut, error) {
	configs, err := s.effectiveSSOProviderConfigs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SSOProviderOut, 0, len(configs))
	for _, providerConfig := range configs {
		if !providerConfig.Enabled {
			continue
		}
		out = append(out, SSOProviderOut{ID: string(providerConfig.ID), Name: providerConfig.Name})
	}
	return out, nil
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
	if err := s.publishEvent(ctx, events.AuthSecuritySSOSettingsUpdatedEvent{
		EventType: events.AuthSecuritySSOSettingsUpdated,
		Envelope: events.Envelope{
			TargetType:    "sso_provider",
			TargetID:      providerID,
			TargetDisplay: providerID,
		},
		Changes: events.NewAuthSecuritySSOSettingsUpdatedChanges(
			body.ClientID != nil,
			body.ClientSecret != nil,
			body.CallbackURL != nil,
			body.Enabled != nil,
		),
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

func (s *Service) startSSO(ctx context.Context, providerID, returnTo, organisationSlug string) (string, []http.Cookie, error) {
	providerConfig, ok, err := s.effectiveSSOProviderConfig(ctx, providerID)
	if err != nil {
		return "", nil, err
	}
	if !ok || !providerConfig.Enabled {
		return "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOProviderNotFound, i18n.P("provider", providerID)))
	}
	if s.cache == nil {
		return "", nil, apperrors.Internal(i18n.Msg(i18n.CodeAuthSSOCacheRequired))
	}
	provider, err := newGothProviderForSSO(providerConfig)
	if err != nil {
		return "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOStart))
	}
	stateToken, err := security.RandomToken(32)
	if err != nil {
		return "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageGenerateToken))
	}
	session, err := provider.BeginAuth(stateToken)
	if err != nil {
		return "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOStart))
	}
	authURL, err := session.GetAuthURL()
	if err != nil {
		return "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOStart))
	}
	transactionID, err := security.RandomToken(32)
	if err != nil {
		return "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageGenerateToken))
	}
	cacheState := ssoCacheState{
		ProviderID:       providerID,
		ReturnTo:         internalReturnPath(returnTo),
		OrganisationSlug: strings.TrimSpace(organisationSlug),
		State:            stateToken,
		Session:          session.Marshal(),
	}
	if err := cachecore.SetJSON(ctx, s.cache, s.sso.cacheKey(transactionID), cacheState, s.sso.stateLifetime); err != nil {
		return "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOStart))
	}
	return authURL, []http.Cookie{{
		Name:     s.sso.stateCookieName,
		Value:    transactionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.sso.stateLifetime.Seconds()),
		Expires:  s.clock.Now().Add(s.sso.stateLifetime),
	}}, nil
}

func (s *Service) completeSSO(ctx context.Context, providerID, queryState, code, transactionID, ip, userAgent string) (string, string, []http.Cookie, error) {
	providerConfig, ok, err := s.effectiveSSOProviderConfig(ctx, providerID)
	if err != nil {
		return "", "", nil, err
	}
	if !ok || !providerConfig.Enabled {
		return "", "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOProviderNotFound, i18n.P("provider", providerID)))
	}
	if s.cache == nil || transactionID == "" {
		return "", "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOInvalidState))
	}
	var cached ssoCacheState
	if err := cachecore.GetJSON(ctx, s.cache, s.sso.cacheKey(transactionID), &cached); err != nil {
		return "", "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOInvalidState))
	}
	if cached.ProviderID != providerID || cached.State != queryState {
		return "", "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOInvalidState))
	}
	provider, err := newGothProviderForSSO(providerConfig)
	if err != nil {
		return "", "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOCallback))
	}
	session, err := provider.UnmarshalSession(cached.Session)
	if err != nil {
		return "", "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOCallback))
	}
	if err := validateSSOState(session, queryState); err != nil {
		return "", "", nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthSSOInvalidState))
	}
	if _, err := session.Authorize(provider, url.Values{"code": []string{code}, "state": []string{queryState}}); err != nil {
		return "", "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOCallback))
	}
	gothUser, err := provider.FetchUser(session)
	if err != nil {
		return "", "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOCallback))
	}
	userRow, err := s.store.FindUserByProviderAccount(ctx, sqlcgen.FindUserByProviderAccountParams{Provider: providerID, ProviderAccountID: gothUser.UserID})
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return "", "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageFindUser))
		}
	}
	user := userFromProviderAccountRow(userRow)
	if user == nil && gothUser.Email != "" {
		emailRow, emailErr := s.store.FindUserByEmail(ctx, transform.Email(gothUser.Email))
		if emailErr != nil {
			if errors.Is(emailErr, sql.ErrNoRows) {
				return "", "", nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthSSOLoginFailed))
			}
			return "", "", nil, apperrors.Annotate(emailErr, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageFindUser))
		}
		user = userFromFindUserRow(emailRow)
		if user == nil || !user.EmailVerified {
			return "", "", nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthSSOLoginFailed))
		}
		if err := s.store.InsertAccount(ctx, sqlcgen.InsertAccountParams{
			ID:                id.MustParse(s.idg.New()),
			UserID:            id.MustParse(user.ID),
			Provider:          providerID,
			ProviderAccountID: gothUser.UserID,
			CreatedAt:         s.clock.Now().UTC(),
			UpdatedAt:         s.clock.Now().UTC(),
		}); err != nil {
			return "", "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageFindAccount))
		}
	}
	if user == nil {
		return "", "", nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthSSOLoginFailed))
	}
	organisation, err := s.resolveLoginOrganisation(ctx, user.ID, cached.OrganisationSlug)
	if err != nil {
		return "", "", nil, err
	}
	token, err := s.newSession(ctx, user.ID, organisation.ID, ip, userAgent)
	if err != nil {
		return "", "", nil, err
	}
	if err := s.cache.Delete(ctx, s.sso.cacheKey(transactionID)); err != nil {
		return "", "", nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageSSOCallback))
	}
	return cached.ReturnTo, token, s.clearSSOCookies(), nil
}

func userFromProviderAccountRow(row sqlcgen.FindUserByProviderAccountRow) *User {
	if row.ID == uuid.Nil {
		return nil
	}
	return &User{ID: row.ID.String(), Email: row.Email, DisplayName: row.DisplayName, EmailVerified: row.EmailVerifiedAt.Valid}
}

const (
	ssoSettingSourceEnv = "env"
	ssoSettingSourceDB  = "db"
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
		out[setting.ProviderID] = SSOProviderSetting{
			ProviderID:    setting.ProviderID,
			ClientID:      setting.ClientID.String,
			ClientIDValid: setting.ClientID.Valid,
			Secret:        setting.ClientSecret.String,
			SecretValid:   setting.ClientSecret.Valid,
			CallbackURL:   setting.CallbackUrl.String,
			CallbackValid: setting.CallbackUrl.Valid,
			Enabled:       setting.Enabled,
		}
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
