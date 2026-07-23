---
name: logging
description: Detailed logging rules for Dinchy — when to use Info vs Warn, message wording and casing, structured field names, boundary-only error logging, and the log-once marker / coderules logreturn / allow-logreturn machinery. Use when adding or editing any log statement.
---

# Logging

The always-on principles live in `.rules`. This skill holds the mechanics.

## What to log, and where

* Log each application event once, at the boundary that owns it.
  * Services and helpers return context-rich results.
  * Callers log only when they are the first layer that can observe the full outcome.
  * Do not log the same event in both a callee and its caller.
* Use `Info` for completed milestones and state transitions.
  * Examples: application started, worker registered, worker run completed, session cleanup finished.
* Use `Warn` for recoverable anomalies and fallbacks that do not fail the operation.
  * Examples: dev proxy URL is invalid, no workers are registered, a non-fatal subsystem is unavailable.
* Treat warnings as recoverable signals, not soft errors.
  * A warning should explain what was skipped, degraded, or fell back.
  * If the condition should fail the request or process, use an error instead of `Warn`.

## Message style

* Keep application log messages consistent.
  * Use sentence case.
  * Use present tense for starting or stopping actions.
  * Use past tense for completed outcomes.
  * Capitalize acronyms such as `HTTP`, `URL`, and `ID`.
  * Do not end log messages with a period.
  * Keep messages short, factual, and stable.
* Prefer structured fields over repeating free-form text.
  * Use stable names such as `component`, `operation`, `task`, `status`, `duration`, `request_id`, `trace_id`, and `span_id`.
  * Include only fields the caller cannot already infer.

## Error logging

* Keep error logging boundary-only.
  * Lower layers annotate and return structured `AppError` values.
  * The owning boundary logs internal and fatal failures once.
  * Expected client and validation failures are returned, not logged again.
  * `main` logs startup and shutdown failures when they escape the application boundary.
  * HTTP handlers log only the final 5xx failure at the transport boundary.
* Error logging goes only through `internal/platform/logging.Error` and `logging.HTTPError`; never call `slog` error methods directly with an error value outside that package.
  * `AppError` carries a log-once marker: the logging helpers skip errors already marked and mark on emit, so a failure is recorded at most once even when it crosses multiple boundaries.
  * `go run ./cmd/coderules logreturn ./...` enforces that no function both logs an error and returns one.
  * Exempt a deliberate dual-failure path (log cleanup failure, return primary) with `//dinchy:allow-logreturn <reason>` on the function; the reason is mandatory.
