package intiface

import (
	"encoding/json"
	"fmt"
)

func parseMessages(raw []byte) []map[string]any {
	var data any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil
	}
	var items []any
	switch typed := data.(type) {
	case map[string]any:
		if messages, ok := typed["Messages"].([]any); ok {
			items = messages
		} else {
			items = []any{typed}
		}
	case []any:
		items = typed
	default:
		return nil
	}

	messages := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		for key, value := range entry {
			body, ok := value.(map[string]any)
			if !ok {
				continue
			}
			merged := make(map[string]any, len(body)+2)
			for k, v := range body {
				merged[k] = v
			}
			merged["Id"] = body["Id"]
			merged[key] = body
			messages = append(messages, merged)
		}
	}
	return messages
}

func parseDevices(response map[string]any) []DeviceCapabilities {
	rawList := response["DeviceList"]
	list, ok := rawList.(map[string]any)
	if !ok {
		return nil
	}
	devicesRaw, ok := list["Devices"].([]any)
	if !ok {
		return nil
	}
	devices := make([]DeviceCapabilities, 0, len(devicesRaw))
	for index, item := range devicesRaw {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		devices = append(devices, deviceFromEntry(index, entry))
	}
	return devices
}

func deviceFromEntry(index int, entry map[string]any) DeviceCapabilities {
	if id, ok := entry["Id"].(float64); ok {
		index = int(id)
	}
	messages := entry["DeviceMessages"]
	hasLinear, hasVibrate, hasRotate := capabilityFlags(messages)
	deviceID := stringValue(entry["DeviceId"], fmt.Sprintf("%d", index))
	name := stringValue(entry["DeviceName"], fmt.Sprintf("device_%d", index))
	return DeviceCapabilities{
		DeviceIndex: index,
		DeviceID:    deviceID,
		Name:        name,
		HasLinear:   hasLinear,
		HasVibrate:  hasVibrate,
		HasRotate:   hasRotate,
	}
}

func capabilityFlags(messages any) (linear, vibrate, rotate bool) {
	switch typed := messages.(type) {
	case []any:
		for _, item := range typed {
			name, _ := item.(string)
			switch name {
			case "LinearCmd":
				linear = true
			case "VibrateCmd":
				vibrate = true
			case "RotateCmd":
				rotate = true
			}
		}
	case map[string]any:
		linear = typed["LinearCmd"] != nil
		vibrate = typed["VibrateCmd"] != nil
		rotate = typed["RotateCmd"] != nil
	}
	return linear, vibrate, rotate
}

func stringValue(value any, fallback string) string {
	if text, ok := value.(string); ok && text != "" {
		return text
	}
	return fallback
}
