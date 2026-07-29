// Command caddy is the Caddy build Dinchy runs in front of itself.
//
// It exists to pin the Caddy version: go.mod and go.sum lock it, so `mise run caddy:build`
// produces the same binary on every machine without a manifest of our own. The build is
// deliberately vanilla — plugins belong to the operator, who compiles them with xcaddy
// against this same pinned version (`mise run caddy:version`).
package main

import (
	caddycmd "github.com/caddyserver/caddy/v2/cmd"

	_ "github.com/caddyserver/caddy/v2/modules/standard"
)

func main() { caddycmd.Main() }
