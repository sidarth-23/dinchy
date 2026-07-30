# Container images built from this module. The build context is the module root because every
# target here compiles Go against this repository's go.mod and go.sum — that pinning is the whole
# reason cmd/caddy exists, and an image built from anywhere else would lose it.
#
# Targets:
#   caddy        the shared edge — vanilla Caddy at the version go.mod locks
#   backend-dev  the development backend, running under Delve
#   dinchy       the production binary
#
# The versions below are pinned to match mise.toml, so the container and the host compile with the
# same toolchain and debug with the same Delve. They are ARGs rather than literals so a bisect can
# move them without editing the file.
ARG GO_VERSION=1.26.3
ARG DELVE_VERSION=1.27.0
ARG GOOSE_VERSION=3.27.1

# --- the shared Caddy edge ---------------------------------------------------------------------

FROM docker.io/library/golang:${GO_VERSION}-bookworm AS caddy-build
WORKDIR /src
# Only the module graph and cmd/caddy are copied, so a change anywhere else in the repository does
# not invalidate this layer.
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/caddy ./cmd/caddy
RUN CGO_ENABLED=0 go build -trimpath -o /out/caddy ./cmd/caddy

FROM docker.io/library/alpine:3.22 AS caddy
# ACME validates the certificate authority's chain against the public roots.
RUN apk add --no-cache ca-certificates wget
COPY --from=caddy-build /out/caddy /usr/local/bin/caddy
# Caddy resolves both directories once, at process start: ConfigAutosavePath and DefaultStorage are
# package-level variables initialised from the environment. Setting them here rather than in a
# compose file or a unit means every consumer gets the same layout, and an operator cannot silently
# end up with a different one.
#
# The two are not interchangeable. /data holds the local CA, ACME account keys and every issued
# certificate — losing it re-registers with the authority and re-issues everything. /config holds
# only the autosave, which is what `run --resume` reads to bring back the routes its tenants
# pushed.
ENV XDG_DATA_HOME=/data \
	XDG_CONFIG_HOME=/config
VOLUME ["/data", "/config"]
EXPOSE 80 443 443/udp 2019
ENTRYPOINT ["caddy"]
# --resume restores the last configuration its tenants pushed; --config seeds the first start and
# any start after the autosave has been removed. Caddy ignores --config when an autosave exists,
# warning that it did, which is why passing both is safe and self-documenting.
CMD ["run", "--resume", "--config", "/etc/caddy/base.json"]

# --- the development backend ------------------------------------------------------------------

FROM docker.io/library/golang:${GO_VERSION}-bookworm AS gotools
ARG DELVE_VERSION
ARG GOOSE_VERSION
RUN go install github.com/go-delve/delve/cmd/dlv@v${DELVE_VERSION} && \
	go install github.com/pressly/goose/v3/cmd/goose@v${GOOSE_VERSION}

FROM docker.io/library/golang:${GO_VERSION}-bookworm AS backend-dev
# git is needed by the Go toolchain to resolve modules; ca-certificates for outbound TLS.
RUN apt-get update -qq && \
	apt-get install -y --no-install-recommends git ca-certificates && \
	rm -rf /var/lib/apt/lists/*
COPY --from=gotools /go/bin/dlv /go/bin/goose /usr/local/bin/
# The caches are volume mount points, so a restart recompiles incrementally instead of from cold.
# HOME is set because the toolchain writes there and the container runs as the invoking user, whose
# home directory does not exist inside the image.
ENV GOMODCACHE=/gomodcache \
	GOCACHE=/gocache \
	HOME=/tmp
# No source is copied. It arrives at runtime as a bind mount at the repository's host path, which
# is what makes Delve report paths the editor can resolve without a source-path substitution.

# --- the production binary ----------------------------------------------------------------------

FROM docker.io/library/golang:${GO_VERSION}-bookworm AS dinchy-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# cgo-free and trimmed, so the result runs on a distroless base and carries no build paths.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/dinchy ./cmd/dinchy

# wget comes from busybox rather than the base image, for the readiness probe the quadlet runs.
FROM docker.io/library/busybox:1.37-musl AS probe

FROM gcr.io/distroless/static-debian12:nonroot AS dinchy
COPY --from=probe /bin/wget /bin/wget
COPY --from=dinchy-build /out/dinchy /usr/local/bin/dinchy
# The app serves no documents and reads no web assets: a separate container serves the built
# frontend, and the edge proxies to it. So there is nothing to copy here but the binary.
EXPOSE 8080 9090
ENTRYPOINT ["/usr/local/bin/dinchy"]
