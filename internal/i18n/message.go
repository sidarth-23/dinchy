package i18n

// newMessage constructs a typed message with optional interpolation metadata.
func newMessage(code string, meta map[string]any) Message {
	return Message{code: code, meta: cloneMeta(meta)}
}

func cloneMeta(meta map[string]any) map[string]any {
	if len(meta) == 0 {
		return nil
	}
	out := make(map[string]any, len(meta))
	for k, v := range meta {
		out[k] = v
	}
	return out
}
