// Package frontend embeds and serves the compiled web UI assets.
package frontend

import (
	"embed"
	"io/fs"
)

// WebFS is the embedded filesystem containing the frontend build output.
//go:embed dist/*
var WebFS embed.FS

// DistFS returns the dist subdirectory as an fs.FS suitable for serving via http.FileServer.
func DistFS() (fs.FS, error) {
	return fs.Sub(WebFS, "dist")
}
