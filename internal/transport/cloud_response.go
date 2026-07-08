package transport

import (
	"encoding/json"
	"fmt"
	"strings"
)

func cloudDeviceResponseError(body []byte) error {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	raw, ok := payload["error"]
	if !ok || string(raw) == "null" {
		return nil
	}
	var deviceErr struct {
		Code    int    `json:"code"`
		Name    string `json:"name"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &deviceErr); err != nil {
		return fmt.Errorf("cloud device error: %s", trimmed)
	}
	if deviceErr.Message == "" && deviceErr.Name == "" {
		return fmt.Errorf("cloud device error: %s", trimmed)
	}
	if deviceErr.Name == "" {
		return fmt.Errorf("cloud device error: %s", deviceErr.Message)
	}
	if deviceErr.Code != 0 {
		return fmt.Errorf("cloud device error %s (%d): %s", deviceErr.Name, deviceErr.Code, deviceErr.Message)
	}
	return fmt.Errorf("cloud device error %s: %s", deviceErr.Name, deviceErr.Message)
}

func truncateDebugBody(body []byte, limit int) string {
	text := strings.TrimSpace(string(body))
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}
