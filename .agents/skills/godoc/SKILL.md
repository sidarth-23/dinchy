---
name: godoc
description: Detailed godoc conventions for Dinchy — package comments and where they live, when a dedicated doc.go is warranted versus a regular source file, exported-symbol doc comments enforced by revive, and how to add testable Example functions for utility packages. Use when documenting a package, deciding between doc.go and an inline package comment, or adding godoc Examples.
---

# Godoc conventions

The always-on principles live in `.rules`. This skill holds the mechanics.

## Package comments

* Every package carries exactly one `// Package <name> ...` comment.
  * Write it as a full sentence beginning with `Package <name>` (for `package main`, use `Command <binary>`), terse and intent-focused — say what the package is for, not how it works.
  * Enforced by revive's `package-comments` rule; a missing or malformed package comment fails lint.
* Put the comment at the top of a natural source file, not a dedicated `doc.go`.
  * The natural home is the package's primary file — the one named after the package or its central concern (e.g. `errors.go` in `internal/foundation/errors`, `text.go` in `internal/foundation/transform`).
  * Only one file per package should carry the package comment; revive flags duplicates.
* Where a sibling package is easy to confuse with this one, contrast them in the comment. The model is `internal/platform/jobs`, whose comment explicitly distinguishes durable Postgres-backed jobs from the ephemeral in-memory schedules in `internal/workers`.

## When a `doc.go` is warranted

* Reach for a dedicated `doc.go` (holding only the package comment and, at most, `//go:generate` directives) **only** when there is no natural home file:
  * a package whose Go source is entirely code-generated, so any hand-written comment would sit awkwardly in generated output, or
  * a package split across many files with no single primary file to host the comment.
* The two real cases in this repo are `internal/manifest` and `internal/foundation/permission`. Absent a reason like those, do **not** add a `doc.go` — the default is an inline package comment.

## Exported symbols

* Every exported symbol (type, function, method, constant, variable) carries a terse doc comment beginning with the symbol's name — `// Apply runs ...`, `// AppError is ...`.
  * Enforced by revive's `exported` rule.
  * This is the doc-comment side of the `.rules` "keep comments rare" principle: inline comments are reserved for non-obvious intent, but exported-symbol and package doc comments are always required.

## Testable Examples

Utility behavior is documented with godoc `Example` functions, which render on the
godoc page under the symbol they document and run as part of `go test`.

* Place examples in an `example_test.go` file colocated with the source, in the external black-box package (`package <name>_test`), matching the existing black-box tests such as `internal/foundation/errors/error_test.go`. Import the package under test by its normal path (alias it, e.g. `apperrors`, only where the package name would otherwise collide).
* Name them by convention so godoc attaches them correctly: `Example` (package-level), `ExampleApply` (function), `ExampleAppError` (type), `ExampleAppError_Status` (method).
* End a deterministic example with a trailing `// Output:` block; `go test` runs the example and fails if stdout does not match. This is the default — prefer it wherever the function's output is stable.
* For non-deterministic output (e.g. `security.RandomToken`, anything random- or time-based), omit the `// Output:` block. The example still compiles and appears on the godoc page but is not output-checked; print something stable (or nothing) so it reads as real usage.
* Keep examples focused on caller-visible usage — the same bar as tests in `.rules`: show the contract a caller depends on, not implementation trivia.

Reference implementations live in `internal/foundation/{transform,id,errors,security}/example_test.go`.

## Reference

* Always-on principles — `.rules`
* Domain vocabulary and module ownership map — `CONTEXT.md`
