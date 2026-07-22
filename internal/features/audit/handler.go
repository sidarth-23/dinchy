package audit

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/sidarth-23/dinchy/internal/access/permission"
	"github.com/sidarth-23/dinchy/internal/access/session"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/transport/middleware"
)

// API groups the audit handlers and their shared dependencies.
type API struct {
	service *Service
}

// Register mounts the audit operations on the given huma.API instance.
func Register(h huma.API, service *Service) {
	api := &API{service: service}
	huma.Register(h, huma.Operation{
		OperationID: "audit-list-logs",
		Method:      http.MethodGet,
		Path:        "/audit/logs",
		Summary:     "List audit logs",
		Description: "Returns audit log entries filtered by the query parameters. Requires an owner or admin session.",
		Tags:        []string{"Audit"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity},
		Middlewares: huma.Middlewares{middleware.RequirePermissions(h, permission.AuditLogsRead)},
	}, api.list)
}

func (a *API) list(ctx context.Context, in *ListIn) (*ListOut, error) {
	principal := session.PrincipalFrom(ctx)
	if principal == nil {
		return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAccountAuthUnauthenticated))
	}
	if !principal.HasPermission(permission.AuditLogsRead) {
		return nil, apperrors.Forbidden(i18n.Msg(i18n.CodeAccountAuthUnauthenticated))
	}
	logs, err := a.service.List(ctx, ListInput{
		Category: in.Category, Subcategory: in.Subcategory, EventType: in.EventType,
		ActorUserID: in.ActorUserID, TargetType: in.TargetType, TargetID: in.TargetID,
		Outcome: in.Outcome, Before: in.Before.UTC(), BeforeValid: !in.Before.IsZero(), Limit: in.Limit,
	})
	if err != nil {
		return nil, err
	}
	out := &ListOut{}
	for _, log := range logs {
		out.Body.Logs = append(out.Body.Logs, LogOut{
			ID:                  log.ID,
			Category:            log.Category,
			Subcategory:         log.Subcategory,
			EventType:           log.EventType,
			Action:              log.Action,
			Outcome:             log.Outcome,
			ActorUserID:         log.ActorUserID,
			ActorOrganisationID: log.ActorOrganisationID,
			TargetType:          log.TargetType,
			TargetID:            log.TargetID,
			TargetDisplay:       log.TargetDisplay,
			RequestID:           log.RequestID,
			TraceID:             log.TraceID,
			SpanID:              log.SpanID,
			IPAddress:           log.IPAddress,
			UserAgent:           log.UserAgent,
			Metadata:            log.Metadata,
			Changes:             log.Changes,
			CreatedAt:           log.CreatedAt,
		})
	}
	return out, nil
}
