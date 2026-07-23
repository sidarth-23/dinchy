// Package errors provides structured, localizable application errors.
//
// AppError is the source-layer error type. Business, store, and infrastructure
// code returns it; the transport layer turns it into a localized HTTP response.
// It carries a stable i18n message (which yields the machine-readable code), an
// HTTP status, optional typed metadata, and an optional wrapped cause.
//
// # Constructing
//
// Prefer the typed constructors over New; each fixes the HTTP status so call
// sites stay declarative:
//
//	BadRequest          400
//	Unauthorized        401
//	Forbidden           403
//	Conflict            409
//	UnprocessableEntity 422
//	TooManyRequests     429
//	Internal            500
//
// Build the error at the source of the failure, with the stable code and
// localized message already attached:
//
//	return apperrors.Conflict(i18n.Msg(i18n.CodeAccountAuthInvitationExists))
//
// Leaf helpers and parsers that do not cross a layer boundary may return plain
// errors; convert them to an AppError at the first boundary that can add useful
// context, and do not let raw errors leak further up.
//
// # Annotating as it travels up
//
// Annotate adds context to an existing error without discarding it. When the
// error is already an AppError, its code, status, and metadata are preserved and
// the new options are merged on top; when it is not, it becomes a catch-all
// Internal error with the original attached as the cause. Annotate at the
// boundary where the code gains information the caller cannot infer:
//
//	rows, err := store.Delete(ctx, id)
//	if err != nil {
//		return apperrors.Annotate(err, apperrors.WithTask("purge-sessions"))
//	}
//
// # Metadata
//
// Attach metadata through the typed With* helpers rather than ad hoc maps, so
// keys stay consistent across handlers, services, and stores: WithCause,
// WithFieldName, WithFieldKind, WithPath, WithTask, WithDeletedCount, and
// WithMetaMap for a genuinely dynamic shape. Meta returns a defensive,
// message-plus-error merged copy.
//
// # Logged once
//
// AppError carries a log-once marker (Logged and MarkLogged). The logging
// package's boundary helpers skip an error already marked and mark it on emit,
// so a failure is recorded at most once even as it crosses several boundaries.
// Do not inspect or set the marker directly outside internal/platform/logging.
//
// # i18n workflow
//
// The code and localized message travel together: any new user-visible error
// must add or reuse the matching i18n.Message and i18n.Code in the same change,
// and codes stay stable and aligned with the translated catalog. Leave a
// structured error without a localized message only when the failure is
// intentionally internal-only.
//
// # Diagnostic formatting
//
// Raw fmt.Errorf belongs only inside WithCause as an internal diagnostic cause,
// never as the user-facing contract. Use errors.Join only to preserve two
// failures that must both survive (a primary failure plus a rollback failure);
// otherwise choose one primary error. Wrap with %w only when exposing the
// underlying error is a deliberate, stable part of the API. In cause strings,
// prefer %q for user input, IDs, and paths, and %T when the concrete type is
// part of the failure shape.
package errors
