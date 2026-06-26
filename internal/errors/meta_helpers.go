package errors

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
