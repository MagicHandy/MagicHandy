package web

import (
	"io/fs"
	"strings"
	"testing"
)

func TestEmbeddedAssetsExist(t *testing.T) {
	for _, name := range []string{"index.html", "app.css", "app.js", "motion-ui.js", "handy-ble-codec.js"} {
		if _, err := fs.Stat(FS(), name); err != nil {
			t.Fatalf("asset %s is missing: %v", name, err)
		}
	}
}

func TestEmbeddedBluetoothBridgeUIHooksExist(t *testing.T) {
	index, err := fs.ReadFile(FS(), "index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	app, err := fs.ReadFile(FS(), "app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}

	for _, fragment := range []string{
		`id="bluetooth-panel"`,
		`cloud-credential`,
	} {
		if !strings.Contains(string(index), fragment) {
			t.Fatalf("index.html missing %q", fragment)
		}
	}
	for _, fragment := range []string{
		`/api/transport/bluetooth/commands`,
		`/api/transport/bluetooth/ack`,
		`navigator.bluetooth.requestDevice`,
	} {
		if !strings.Contains(string(app), fragment) {
			t.Fatalf("app.js missing %q", fragment)
		}
	}
}

func TestEmbeddedMotionUIHooksExist(t *testing.T) {
	index, err := fs.ReadFile(FS(), "index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	css, err := fs.ReadFile(FS(), "app.css")
	if err != nil {
		t.Fatalf("read app.css: %v", err)
	}
	motionUI, err := fs.ReadFile(FS(), "motion-ui.js")
	if err != nil {
		t.Fatalf("read motion-ui.js: %v", err)
	}

	for _, fragment := range []string{
		`id="stop-button"`,
		`id="motion-start"`,
		`id="quick-speed-min"`,
		`id="quick-speed-max"`,
	} {
		if !strings.Contains(string(index), fragment) {
			t.Fatalf("index.html missing %q", fragment)
		}
	}
	if !strings.Contains(string(css), `[hidden]`) {
		t.Fatal("app.css must preserve hidden elements")
	}
	if !strings.Contains(string(motionUI), `normalizeQuickControls`) {
		t.Fatal("motion-ui.js must normalize quick control ranges before posting")
	}
}
