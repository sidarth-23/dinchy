package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/sidarth-23/dinchy/internal/foundation/requestmeta"
	"github.com/sidarth-23/dinchy/internal/transport/support"
)

// RequestInfo injects the resolved client IP address and User-Agent into the request context.
//
// The client address comes from X-Forwarded-For, but only when the peer that opened the connection
// falls inside trustedProxies. Reading it is not optional: the value is persisted to
// audit_logs.ip_address and sessions.ip_address, so using the connection's own address would record
// the reverse proxy for every request. Gating it is not optional either, for the same reason in
// reverse: an untrusted peer must not be able to author the address recorded against it.
func RequestInfo(trustedProxies []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := requestmeta.WithRequestInfo(r.Context(), clientIP(r, trustedProxies), r.UserAgent())
			ctx = support.WithRequestCookies(ctx, r.Cookies())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// clientIP resolves the address to record, falling back to the direct peer whenever the forwarded
// header cannot be trusted — because no proxy is in front of the app, because the peer is not one
// of the trusted ones, or because the header is absent.
//
// Only the first entry is read and no chain is walked: the edge replaces X-Forwarded-For rather
// than appending to it, so a trusted peer's value is the client address and there is nothing
// behind it. Putting a header-appending CDN in front would mean changing what the edge writes,
// not walking the chain here.
func clientIP(r *http.Request, trustedProxies []netip.Prefix) string {
	peer, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return peerHost(r)
	}
	if !isTrustedProxy(peer.Addr(), trustedProxies) {
		return peer.Addr().String()
	}

	forwarded := r.Header.Get("X-Forwarded-For")
	if first, _, found := strings.Cut(forwarded, ","); found {
		forwarded = first
	}
	if address := strings.TrimSpace(forwarded); address != "" {
		return address
	}
	return peer.Addr().String()
}

// isTrustedProxy reports whether an address is one whose forwarded headers the app honors.
//
// The address is unmapped first: on a dual-stack listener a loopback peer arrives as
// ::ffff:127.0.0.1, which no 127.0.0.1/32 prefix matches, and the whole trust set would silently
// never apply.
func isTrustedProxy(address netip.Addr, trustedProxies []netip.Prefix) bool {
	address = address.Unmap()
	for _, prefix := range trustedProxies {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

// peerHost returns the connection's address when RemoteAddr is not an IP:port pair a trust check
// can be made against — a Unix socket, or whatever a test's synthetic request carries.
func peerHost(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
