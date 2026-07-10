package cache

import "strings"

// Keyer builds colon-delimited Redis keys under a shared prefix.
type Keyer struct {
	prefix string
}

// NewKeyer creates a Keyer that namespaces keys under prefix.
func NewKeyer(prefix string) Keyer {
	return Keyer{prefix: strings.Trim(prefix, ":")}
}

// Key joins the prefix and parts with colons, skipping empty segments.
func (k Keyer) Key(parts ...string) string {
	all := make([]string, 0, len(parts)+1)
	if k.prefix != "" {
		all = append(all, k.prefix)
	}
	for _, part := range parts {
		part = strings.Trim(part, ":")
		if part != "" {
			all = append(all, part)
		}
	}
	return strings.Join(all, ":")
}
