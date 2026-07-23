---
name: error-handling
description: Detailed rules for constructing, annotating, and returning errors in Dinchy — AppError constructors, Annotate/WithCause, errors.Join, %w/%q/%T formatting, the i18n message/code workflow, and typed error metadata. Use when writing, reviewing, or refactoring any error-return path.
---

# Error handling

The always-on principles live in `.rules`. This skill holds the mechanics.

## Adding context

* Always add context when returning an error.
  * Context should include what the code was trying to do, plus any loop iteration, computed value, or other data the caller cannot infer.
  * Context must uniquely identify the code path when multiple returns can fail in the same function.
  * Include all context the caller does not already have. Omit context the caller clearly already knows.
* If there is no new context to add, leave a comment explaining why the existing error already carries enough detail.
  * This is a last resort for paths where the returned error already fully identifies the failure.
  * Do not use this as a substitute for adding missing information when the caller would benefit from it.
* Never return `err` by itself when a richer error can be returned.
  * Preserve the original error as context when it adds value, but add the missing details around it.

## Structured errors

* Use `internal/foundation/errors.AppError` for user-visible failures and anything that crosses a layer boundary.
  * Prefer the typed constructors such as `BadRequest`, `Unauthorized`, `Forbidden`, `Conflict`, `UnprocessableEntity`, and `Internal`.
  * Keep the stable error code and localized message at the source of the failure.
  * Attach metadata through typed helpers, not ad hoc stringly-typed maps.
  * Any new user-visible structured error must add or reuse the matching `i18n.Message` and `i18n.Code` in the same change.
  * Do not leave a structured error without a localized message unless the failure is intentionally internal-only.
  * Keep error codes stable and aligned with the translated message catalog.
* Add context at the boundary where the code gains information.
  * Use `Annotate` when a lower layer has failed and the caller needs feature, stage, operation, task, path, or field context.
  * Preserve the original code and metadata when re-annotating structured errors.
  * If an error is not already structured, convert it into a structured internal error before it leaves the layer.
* Leaf helpers and parsers may return plain errors or sentinel-style booleans when they do not cross a layer boundary.
  * Convert them to structured app errors at the first boundary that can add useful context.
  * Keep this exception narrow; do not let raw errors leak through application layers.

## Metadata

* When adding error metadata, prefer the typed helpers already defined in `internal/foundation/errors` over ad hoc maps.
  * Use the typed helpers so metadata stays consistent across handlers, services, and stores.
  * Only use raw maps when the shape is genuinely dynamic and cannot be expressed with the existing helpers.
* Preserve error codes and metadata through annotation and wrapping paths.
  * Do not lose metadata unless there is a deliberate reason to sanitize it at a boundary.
  * When a layer adds context, keep the original code visible to callers unless the boundary intentionally transforms it.

## Raw errors, joining, and wrapping

* Use raw `fmt.Errorf` only for internal diagnostic cause messages.
  * Keep those messages inside `WithCause` or other internal error plumbing.
  * Do not expose those raw strings as the primary user-facing error contract when a structured app error is available.
* Use `errors.Join` only when two failures must be preserved together.
  * Typical examples are a primary failure plus rollback/cleanup failure.
  * Do not join errors just to avoid choosing one; choose a clear primary error unless preserving both is important.
* Do not wrap with `%w` unless the wrapped error is part of the API contract you intend to keep stable.
  * If the caller should inspect the underlying error, only wrap when that exposure is deliberate.

## Formatting

* Use `%T` when the type is unknown or may vary.
  * Prefer it when the concrete type is part of the failure shape.
* Use `%q` for strings unless you are certain they are already clean and non-empty.
  * Prefer quoted strings for user input, IDs, paths, and any value that may contain spaces or empties.
* Do not start error text with `failed to` or `error` unless you are logging.
  * Write the action or condition instead of prefixing the sentence with a generic label.
