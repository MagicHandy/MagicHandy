package web

import (
	"io/fs"
	"testing"
)

func TestEmbeddedAssetsExist(t *testing.T) {
	for _, name := range []string{"index.html", "app.css", "app.js"} {
		if _, err := fs.Stat(FS(), name); err != nil {
			t.Fatalf("asset %s is missing: %v", name, err)
		}
	}
}
