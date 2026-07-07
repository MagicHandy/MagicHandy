// Package web embeds the static single-page application built by Vite into
// web/dist. The React frontend (docs/decisions/0009-react-frontend.md) is a
// build-time dependency only — Node/npm/Vite build the assets, and this Go
// binary serves the committed embedded output at runtime with no Node process.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var assets embed.FS

// FS returns the embedded browser UI filesystem rooted at the built output.
func FS() fs.FS {
	sub, err := fs.Sub(assets, "dist")
	if err != nil {
		panic("web: embedded dist is missing; run `npm run build` in web/: " + err.Error())
	}
	return sub
}
