// Package web embeds the static single-page application assets.
package web

import (
	"embed"
	"io/fs"
)

//go:embed index.html app.css app.js motion-ui.js chat-ui.js handy-ble-codec.js
var assets embed.FS

// FS returns the embedded browser UI filesystem.
func FS() fs.FS {
	return assets
}
