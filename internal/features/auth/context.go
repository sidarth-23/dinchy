package auth

import "context"

type ctxKey int

const ctxKeySession ctxKey = iota

// WithSession attaches a validated session to the request context.
func WithSession(ctx context.Context, s *SessionWithUser) context.Context {
	return context.WithValue(ctx, ctxKeySession, s)
}

// SessionFrom retrieves the session from the context, or nil for anonymous requests.
func SessionFrom(ctx context.Context) *SessionWithUser {
	s, _ := ctx.Value(ctxKeySession).(*SessionWithUser)
	return s
}
