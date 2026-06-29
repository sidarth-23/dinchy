// Package frontend serves the compiled web UI assets from the top-level web workspace.
package frontend

import (
	"io/fs"
	"os"
)

// DistFS returns the dist subdirectory as an fs.FS suitable for serving via http.FileServer.
func DistFS() (fs.FS, error) {
	return os.DirFS("web/dist"), nil
}
