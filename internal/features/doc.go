// Package features provides the shared service base that every feature service embeds.
//
// A feature package under internal/features/* owns one slice of the product
// (auth, session, audit, health) and keeps its handlers, services, types, and
// helpers together. Prefer a single feature package over splitting a concern
// across packages unless the split materially improves ownership or clarity, and
// keep package boundaries aligned with the feature surface rather than incidental
// implementation details.
//
// # File classes
//
// Files use the *_*.go naming pattern on purpose. The approved classes are:
//
//	*_service.go         business logic, orchestration, stateful operations
//	*_helpers.go         shared feature-local helpers and private glue
//	*_middleware.go      HTTP request/response middleware guarding the surface
//	*_type.go            feature-local types, request/response shapes, enums
//	handler.go           the transport entrypoint (single primary handler file)
//	*_background_job.go  workers, schedulers, and queue consumers the feature owns
//	events.json          hand-written event catalog fragment (one top-level module,
//	                     validated against platform/events/catalog.schema.json)
//	events_generated.go  generated event constants and metadata; never edited by hand
//	*_mockdata_test.go   mock data kept local to the feature or test surface
//
// A file that fits none of these is a design decision, not a naming accident:
// any new class must be approved and recorded in .rules before use.
//
// # Validation at the transport boundary
//
// Input is validated and normalized at the transport boundary, and handlers and
// services only ever operate on validated, transformed values — never re-run ad
// hoc normalization in business logic. Validation tags on the input type reject
// malformed payloads, and a Resolve method canonicalizes user input via
// internal/foundation/transform before validation reaches services. See
// LoginBody.Resolve in internal/features/auth/auth_type.go for the exemplar.
//
// # Configuration
//
// All environment variables come from internal/config; features never read env
// vars ad hoc. Add a new variable to the config layer first so its default and
// validation stay centralized, then wire every use through that config value.
//
// # Tests
//
// Tests cover caller-visible contracts — error codes, metadata, and visible
// responses — not implementation trivia. Name a test file after the parent
// feature or source file it validates, and keep each file focused on that one
// concern.
package features
