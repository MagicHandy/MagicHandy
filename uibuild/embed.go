// Package uibuild embeds the production React build from frontend/dist.
package uibuild

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the embedded frontend/dist filesystem for http.FileServer-style serving.
func FS() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}
