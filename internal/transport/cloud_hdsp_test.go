package transport

import (
	"encoding/json"
	"testing"
)

func TestBuildHDSPMoveUsesV3FieldNames(t *testing.T) {
	builder := newCloudBuilder(t, CloudBuildOptions{})
	request := builder.BuildHDSPMove(53.15, 120, true)

	body, ok := request.Body.(map[string]any)
	if !ok {
		t.Fatalf("body type = %T, want map[string]any", request.Body)
	}
	if body["xa"] != 53.15 {
		t.Fatalf("xa = %v, want 53.15", body["xa"])
	}
	if body["va"] != 120 {
		t.Fatalf("va = %v, want 120", body["va"])
	}
	if body["stop_on_target"] != true {
		t.Fatalf("stop_on_target = %v, want true", body["stop_on_target"])
	}
	if body["immediate_rsp"] != true {
		t.Fatalf("immediate_rsp = %v, want true", body["immediate_rsp"])
	}
	if _, exists := body["position"]; exists {
		t.Fatal("legacy v2 field position must not be sent")
	}
	if request.Path != "hdsp/xava" {
		t.Fatalf("path = %q, want hdsp/xava", request.Path)
	}
}

func TestBuildHDSPModeUsesMode2Endpoint(t *testing.T) {
	builder := newCloudBuilder(t, CloudBuildOptions{})
	request := builder.BuildHDSPMode()
	if request.Path != "mode2" {
		t.Fatalf("path = %q, want mode2", request.Path)
	}
}

func TestCloudDeviceResponseError(t *testing.T) {
	t.Parallel()
	okBody := []byte(`{"result":"ok"}`)
	if err := cloudDeviceResponseError(okBody); err != nil {
		t.Fatalf("ok body returned error: %v", err)
	}

	errBody := []byte(`{"error":{"code":1001,"name":"DeviceNotConnected","message":"Device not connected","connected":false}}`)
	err := cloudDeviceResponseError(errBody)
	if err == nil {
		t.Fatal("expected device error")
	}
	if got := err.Error(); got == "" || got == "{}" {
		t.Fatalf("error = %q, want descriptive message", got)
	}

	encoded, marshalErr := json.Marshal(map[string]any{"result": map[string]any{"mode": 2}})
	if marshalErr != nil {
		t.Fatalf("marshal: %v", marshalErr)
	}
	if err := cloudDeviceResponseError(encoded); err != nil {
		t.Fatalf("mode response should be ok: %v", err)
	}
}
