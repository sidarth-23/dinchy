package middleware

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/sidarth-23/dinchy/internal/access/permission"
	"github.com/sidarth-23/dinchy/internal/access/session"
	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
)

// SessionResolutionGuard returns Huma middleware that surfaces session lookup failures.
func SessionResolutionGuard(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if err := session.ResolutionErrorFrom(ctx.Context()); err != nil {
			_ = huma.WriteErr(api, ctx, http.StatusInternalServerError, "session resolution failed", err)
			return
		}
		next(ctx)
	}
}

// RequirePermissions returns Huma middleware that requires every listed permission.
func RequirePermissions(api huma.API, permissions ...permission.Permission) func(huma.Context, func(huma.Context)) {
	return requirePermissions(api, permissions...)
}

func requirePermissions(api huma.API, permissions ...permission.Permission) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		principal := session.PrincipalFrom(ctx.Context())
		if principal == nil {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "authentication required", apperrors.Unauthorized(i18n.Msg(i18n.CodeAccountAuthUnauthenticated)))
			return
		}
		for _, required := range permissions {
			if !principal.HasPermission(required) {
				_ = huma.WriteErr(api, ctx, http.StatusForbidden, "permission required", apperrors.Forbidden(i18n.Msg(i18n.CodeAccountAuthForbidden)))
				return
			}
		}
		next(ctx)
	}
}
