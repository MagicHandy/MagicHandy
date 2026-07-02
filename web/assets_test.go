package web

import (
	"io/fs"
	"strings"
	"testing"
)

func TestEmbeddedAssetsExist(t *testing.T) {
	for _, name := range []string{"index.html", "app.css", "app.js", "motion-ui.js", "chat-ui.js", "bluetooth-ui.js", "handy-ble-codec.js"} {
		if _, err := fs.Stat(FS(), name); err != nil {
			t.Fatalf("asset %s is missing: %v", name, err)
		}
	}
}

func TestEmbeddedChatUIHooksExist(t *testing.T) {
	index, err := fs.ReadFile(FS(), "index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	chatUI, err := fs.ReadFile(FS(), "chat-ui.js")
	if err != nil {
		t.Fatalf("read chat-ui.js: %v", err)
	}

	for _, fragment := range []string{
		`id="chat-form"`,
		`id="chat-jump"`,
		`id="chat-malformed"`,
		`id="llm-provider"`,
		`id="llm-mode"`,
		`id="llm-runner-path"`,
		`id="llm-model-path"`,
		`id="llm-load"`,
		`id="llm-unload"`,
		`id="llm-prompt-set"`,
	} {
		if !strings.Contains(string(index), fragment) {
			t.Fatalf("index.html missing %q", fragment)
		}
	}
	for _, fragment := range []string{
		`/api/chat/stream`,
		`repair_delta`,
		`Malformed model JSON`,
		`JSON.stringify(assistantContract)`,
		`shouldStickToBottom`,
		`X-MagicHandy-Client-ID`,
		`magichandy:controller-state`,
	} {
		if !strings.Contains(string(chatUI), fragment) {
			t.Fatalf("chat-ui.js missing %q", fragment)
		}
	}
}

func TestEmbeddedSettingsUIDoesNotClobberDirtyForm(t *testing.T) {
	app, err := fs.ReadFile(FS(), "app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	for _, fragment := range []string{
		`settingsDirty`,
		`markSettingsDirty`,
		`if (settingsDirty && !options.force)`,
		`/api/llm/load`,
		`/api/llm/unload`,
	} {
		if !strings.Contains(string(app), fragment) {
			t.Fatalf("app.js missing %q", fragment)
		}
	}
}

func TestEmbeddedBackendLossUIHooksExist(t *testing.T) {
	index, err := fs.ReadFile(FS(), "index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	app, err := fs.ReadFile(FS(), "app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}

	for _, fragment := range []string{
		`id="backend-banner"`,
		`id="transport-status"`,
		`id="controller-status"`,
		`data-requires-backend`,
		`data-allow-backend-offline`,
	} {
		if !strings.Contains(string(index), fragment) {
			t.Fatalf("index.html missing %q", fragment)
		}
	}
	for _, fragment := range []string{
		`setBackendAvailability`,
		`setControllerState`,
		`magichandy:backend-availability`,
		`magichandy:controller-state`,
		`backendRequiredControls`,
		`controllerRequiredControls`,
	} {
		if !strings.Contains(string(app), fragment) {
			t.Fatalf("app.js missing %q", fragment)
		}
	}
}

func TestEmbeddedConnectionAndDiagnosticsUIHooksExist(t *testing.T) {
	index, err := fs.ReadFile(FS(), "index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	app, err := fs.ReadFile(FS(), "app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}

	for _, fragment := range []string{
		`id="connection-check"`,
		`id="diagnostics-copy"`,
		`Estimated position`,
		`class="shortcut-hint"`,
	} {
		if !strings.Contains(string(index), fragment) {
			t.Fatalf("index.html missing %q", fragment)
		}
	}
	for _, fragment := range []string{
		`/api/transport/cloud/check`,
		`/api/transport/bluetooth/check`,
		`copyDiagnosticsSummary`,
		`writeClipboard`,
	} {
		if !strings.Contains(string(app), fragment) {
			t.Fatalf("app.js missing %q", fragment)
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
	bluetoothUI, err := fs.ReadFile(FS(), "bluetooth-ui.js")
	if err != nil {
		t.Fatalf("read bluetooth-ui.js: %v", err)
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
		`./bluetooth-ui.js`,
		`renderBluetoothStatus`,
		`maybePostUnsupportedBluetoothStatus`,
	} {
		if !strings.Contains(string(app), fragment) {
			t.Fatalf("app.js missing %q", fragment)
		}
	}
	for _, fragment := range []string{
		`/api/transport/bluetooth/commands`,
		`/api/transport/bluetooth/ack`,
		`navigator.bluetooth.requestDevice`,
		`handyBluetoothRequestOptions`,
		`HANDY_BLE_NAME_PREFIXES = ["OHD", "Handy", "The Handy"]`,
		`optionalServices: [HANDY_BLE_SERVICE_UUID]`,
	} {
		if !strings.Contains(string(bluetoothUI), fragment) {
			t.Fatalf("bluetooth-ui.js missing %q", fragment)
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
	if !strings.Contains(string(motionUI), `Commanded position estimate`) {
		t.Fatal("motion-ui.js must label visualizer position as an estimate")
	}
	for _, fragment := range []string{
		`/api/motion/events?client_id=`,
		`new EventSource`,
		`X-MagicHandy-Client-ID`,
		`magichandy:controller-state`,
	} {
		if !strings.Contains(string(motionUI), fragment) {
			t.Fatalf("motion-ui.js missing %q", fragment)
		}
	}
}
