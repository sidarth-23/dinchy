package audit

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/i18n"
)

type API struct {
	service *Service
}

func Register(h huma.API, service *Service) {
	api := &API{service: service}
	huma.Register(h, huma.Operation{
		OperationID: "audit-list-logs",
		Method:      "GET",
		Path:        "/audit/logs",
		Summary:     "List audit logs",
	}, api.list)
}

func (a *API) list(ctx context.Context, in *ListIn) (*ListOut, error) {
	session := auth.SessionFrom(ctx)
	if session == nil {
		return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthUnauthenticated))
	}
	if session.Role != auth.RoleOwner && session.Role != auth.RoleAdmin {
		return nil, apperrors.Forbidden(i18n.Msg(i18n.CodeAuthUnauthenticated))
	}
	var before time.Time
	beforeValid := false
	if in.Before != "" {
		parsed, err := time.Parse(time.RFC3339Nano, in.Before)
		if err != nil {
			return nil, apperrors.BadRequest(i18n.Msg(i18n.CodeRequestValidationFailed))
		}
		before = parsed.UTC()
		beforeValid = true
	}
	logs, err := a.service.List(ctx, ListInput{
		Category: in.Category, Subcategory: in.Subcategory, EventType: in.EventType,
		ActorUserID: in.ActorUserID, TargetType: in.TargetType, TargetID: in.TargetID,
		Outcome: in.Outcome, Before: before, BeforeValid: beforeValid, Limit: in.Limit,
	})
	if err != nil {
		return nil, err
	}
	out := &ListOut{}
	for _, log := range logs {
		out.Body.Logs = append(out.Body.Logs, LogOut(log))
	}
	return out, nil
}
