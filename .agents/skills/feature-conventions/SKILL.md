---
name: feature-conventions
description: Detailed structural conventions for Dinchy feature packages — the approved *_*.go file classes (service, helpers, middleware, type, handler, background_job, events), the Resolve transport-boundary validation pattern, environment variables via internal/config, and test/mockdata file naming. Use when creating files in or wiring up a feature package under internal/features/*.
---

# Feature conventions

The always-on principles live in `.rules`. This skill holds the mechanics.

## Feature file classes

* Feature packages under `internal/features/*` use the `*_*.go` naming pattern on purpose.
  * The approved feature file classes are listed below and must be used consistently.
    * `*_helpers.go` for shared feature-local helpers, small utilities, and private glue that supports the feature without being the main entrypoint.
    * `*_middleware.go` for HTTP or request/response middleware that wraps or guards the feature surface.
    * `*_service.go` for feature business logic, orchestration, and stateful operations that are not transport-specific.
    * `*_type.go` for feature-local types, request/response shapes, enums, and data structures that define the feature contract.
    * `handler.go` for the feature's transport entrypoint when the feature has a single primary handler file and the name is clearer without a prefix.
    * `*_background_job.go` for background workers, schedulers, queue consumers, and task execution logic owned by the feature.
    * `events.json` for the feature's hand-written event catalog fragment; it holds exactly one top-level module named after the feature and is validated against `internal/platform/events/catalog.schema.json`.
    * `events_generated.go` for the generated event constants, `EventDefinitions`, typed metadata, and event aliases produced from `events.json`; it is code-generated and must not be edited by hand.
  * If a file does not fit one of these classes, treat that as a design decision, not a naming accident.
  * Any additional filename class must be explicitly requested, approved, and added to `.rules` before use.

(For the event system's domain model — the bus, feature-owned definitions, and the registration seam — see `CONTEXT.md`.)

## Package cohesion

* Keep feature-local helpers, handlers, services, and types together under the same feature package when that reduces indirection.
  * Prefer a single feature package over splitting a concern across multiple packages unless the split materially improves ownership or clarity.
  * Keep package boundaries aligned with the feature surface, not with incidental implementation details.

## Validation and normalization

* Define API input validation and normalization at the transport boundary.
  * Put validation tags on request input types so the API rejects malformed payloads before handlers run.
  * Implement `Resolve` on input bodies to canonicalize user input with `internal/foundation/transform` before validation reaches services.
  * Keep handlers and services operating on validated, transformed values only; do not repeat ad hoc normalization in business logic.

## Environment variables

* All environment variables must come from `internal/config`.
  * Do not define environment variable names ad hoc in feature code, helpers, or startup glue.
  * Add new environment variables only through the config layer so defaults and validation stay centralized.
  * If a new env var is needed, add it to config first and wire every use through that config value.

## Tests

* Tests should cover core behavior, not implementation trivia.
  * Focus on caller-visible contracts, especially error codes, metadata, and visible responses.
  * Prefer tests that prove the behavior a caller depends on.
* Test files should be named after the parent feature or source file they validate.
  * Keep each test file focused on one parent concern.
  * Test functions inside that file should stay within that concern.
* Mock data test files must use the `*_mockdata_test.go` suffix.
  * Keep mock data local to the feature or test surface it supports.
  * Any different mock-data naming pattern must be explicitly approved first.
