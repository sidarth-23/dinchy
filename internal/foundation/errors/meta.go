package errors

import (
	stdErrors "errors"
	"maps"
	"reflect"
)

// MetaKey is a stable key under which a typed value is stored in error metadata.
type MetaKey string

// Metadata keys for the value-carrying options attached to an AppError.
const (
	MetaKeyConflictingOwner MetaKey = "conflicting_owner"
	MetaKeyDeletedCount     MetaKey = "deleted_count"
	MetaKeyFieldKind        MetaKey = "field_kind"
	MetaKeyFieldName        MetaKey = "field_name"
	MetaKeyHostname         MetaKey = "hostname"
	MetaKeyModule           MetaKey = "module"
	MetaKeyOwner            MetaKey = "owner"
	MetaKeyPath             MetaKey = "path"
	MetaKeyTask             MetaKey = "task"
	MetaKeyUpstream         MetaKey = "upstream"
)

type metaValue interface {
	metaKey() MetaKey
	metaValue() any
}

func withMetaValue(v metaValue) Option {
	return func(e *AppError) {
		if e.meta == nil {
			e.meta = make(map[string]any)
		}
		e.meta[string(v.metaKey())] = v.metaValue()
	}
}

// WithCause sets the wrapped cause on the error.
func WithCause(err error) Option {
	return func(e *AppError) {
		e.cause = err
	}
}

// WithMetaMap copies the provided metadata map into the error.
func WithMetaMap(meta map[string]any) Option {
	return func(e *AppError) {
		if len(meta) == 0 {
			return
		}
		if e.meta == nil {
			e.meta = make(map[string]any, len(meta))
		}
		maps.Copy(e.meta, meta)
	}
}

// DeletedCount records how many rows an operation deleted.
type DeletedCount int

func (deletedCount DeletedCount) metaKey() MetaKey { return MetaKeyDeletedCount }
func (deletedCount DeletedCount) metaValue() any   { return int(deletedCount) }

// WithDeletedCount attaches a deletion count to the error.
func WithDeletedCount(deletedCount DeletedCount) Option { return withMetaValue(deletedCount) }

// FieldName records the name of the field a failure concerns.
type FieldName string

func (fieldName FieldName) metaKey() MetaKey { return MetaKeyFieldName }
func (fieldName FieldName) metaValue() any   { return string(fieldName) }

// WithFieldName attaches a field name to the error.
func WithFieldName(fieldName FieldName) Option { return withMetaValue(fieldName) }

// FieldKind records a reflected type kind in metadata.
type FieldKind string

// FieldKindOf derives a FieldKind from a reflect.Kind.
func FieldKindOf(kind reflect.Kind) FieldKind { return FieldKind(kind.String()) }
func (fieldKind FieldKind) metaKey() MetaKey  { return MetaKeyFieldKind }
func (fieldKind FieldKind) metaValue() any    { return string(fieldKind) }

// WithFieldKind attaches a reflected field kind to the error.
func WithFieldKind(fieldKind FieldKind) Option { return withMetaValue(fieldKind) }

// Path records a filesystem path a failure concerns.
type Path string

func (path Path) metaKey() MetaKey { return MetaKeyPath }
func (path Path) metaValue() any   { return string(path) }

// WithPath attaches a filesystem path to the error.
func WithPath(path Path) Option { return withMetaValue(path) }

// Hostname records the hostname a failure concerns.
type Hostname string

func (hostname Hostname) metaKey() MetaKey { return MetaKeyHostname }
func (hostname Hostname) metaValue() any   { return string(hostname) }

// WithHostname attaches a hostname to the error.
func WithHostname(hostname Hostname) Option { return withMetaValue(hostname) }

// Upstream records the backend address a failure concerns.
type Upstream string

func (upstream Upstream) metaKey() MetaKey { return MetaKeyUpstream }
func (upstream Upstream) metaValue() any   { return string(upstream) }

// WithUpstream attaches an upstream address to the error.
func WithUpstream(upstream Upstream) Option { return withMetaValue(upstream) }

// Module records the identifier of a pluggable module a failure concerns, such as a
// Caddy module that is not compiled into the running binary.
type Module string

func (module Module) metaKey() MetaKey { return MetaKeyModule }
func (module Module) metaValue() any   { return string(module) }

// WithModule attaches a module identifier to the error.
func WithModule(module Module) Option { return withMetaValue(module) }

// Owner records the component that contributed the value a failure concerns.
type Owner string

func (owner Owner) metaKey() MetaKey { return MetaKeyOwner }
func (owner Owner) metaValue() any   { return string(owner) }

// WithOwner attaches an owning component name to the error.
func WithOwner(owner Owner) Option { return withMetaValue(owner) }

// ConflictingOwner records the second claimant in a conflict, so both sides of the
// collision are visible without inspecting the cause chain.
type ConflictingOwner string

func (conflictingOwner ConflictingOwner) metaKey() MetaKey { return MetaKeyConflictingOwner }
func (conflictingOwner ConflictingOwner) metaValue() any   { return string(conflictingOwner) }

// WithConflictingOwner attaches the conflicting component name to the error.
func WithConflictingOwner(conflictingOwner ConflictingOwner) Option {
	return withMetaValue(conflictingOwner)
}

// Task records the durable background task a failure concerns.
type Task string

func (task Task) metaKey() MetaKey { return MetaKeyTask }
func (task Task) metaValue() any   { return string(task) }

// WithTask attaches a durable task name to the error.
func WithTask(task Task) Option { return withMetaValue(task) }

func appErrorFrom(err error) (*AppError, bool) {
	var appErr *AppError
	if !stdErrors.As(err, &appErr) {
		return nil, false
	}
	return appErr, true
}

func mergeMeta(base, extra map[string]any) map[string]any {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	out := make(map[string]any, len(base)+len(extra))
	maps.Copy(out, base)
	maps.Copy(out, extra)
	return out
}
