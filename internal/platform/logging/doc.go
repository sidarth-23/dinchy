// Package logging configures structured application logging and telemetry.
//
// It builds the slog.Logger the application runs on (New), carries a
// request-scoped logger through context, and exposes the small set of helpers
// every boundary uses to record events. The guiding rule is that each event is
// logged once, at the boundary that owns it: services and helpers return
// context-rich results, and only the first layer that can observe the full
// outcome logs it. Never log the same event in both a callee and its caller.
//
// # Events: Trace, Info, and Warn
//
// Use Info for completed milestones and state transitions (application started,
// worker registered, session cleanup finished). Use Warn for recoverable
// anomalies and fallbacks that do not fail the operation (a dev proxy URL is
// invalid, no workers are registered); a warning should say what was skipped,
// degraded, or fell back. If the condition should fail the request or process,
// return an error instead of warning. Use Trace (level LevelTrace, one step
// below Debug) for high-volume internals such as SQL queries and scheduler
// diagnostics, keeping Debug reserved for application-level detail.
//
// # Message style
//
// Keep messages short, factual, and stable. Use sentence case; present tense for
// starting or stopping actions and past tense for completed outcomes; capitalize
// acronyms such as HTTP, URL, and ID; and do not end a message with a period.
// Prefer structured fields over free-form text, using stable names — component,
// operation, task, status, duration, request_id, trace_id, span_id — and include
// only fields the caller cannot already infer:
//
//	logging.Info(ctx, logger, "worker run completed",
//		slog.String("task", "purge-sessions"),
//		slog.Duration("duration", elapsed))
//
// # Error logging is boundary-only
//
// Error and HTTPError are the only paths for logging an error value; never call
// slog's error methods with an error outside this package. Lower layers annotate
// and return structured *errors.AppError values, and the owning boundary logs
// the internal or fatal failure once: main logs failures that escape the
// application boundary, and HTTP handlers log only the final 5xx via HTTPError
// (expected client and validation failures are returned, not logged again).
//
// AppError carries a log-once marker: these helpers skip an error already marked
// and mark it on emit, so a failure is recorded at most once even across several
// boundaries. `go run ./cmd/validate logreturn ./...` enforces that no function
// both logs an error and returns one; exempt a deliberate dual-failure path (log
// the cleanup failure, return the primary) with a `//dinchy:allow-logreturn
// <reason>` comment on the function, where the reason is mandatory.
package logging
