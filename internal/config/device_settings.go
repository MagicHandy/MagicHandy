package config

import "strings"

// DeviceSettings contains device transport configuration.
type DeviceSettings struct {
	// NativeBluetoothAddress is a private hardware identifier, never a public setting.
	NativeBluetoothAddress string   `json:"native_bluetooth_address,omitempty"`
	ConfiguredDevices      []string `json:"configured_devices"`
	LinkedFailurePolicy    string   `json:"linked_failure_policy"`
	// Output selects playback participants; ConfiguredDevices is the saved inventory.
	Output                   string `json:"output"`
	HSPDispatchOwner         string `json:"hsp_dispatch_owner"`
	IntifaceServerAddress    string `json:"intiface_server_address"`
	FirmwareAPIRequirement   string `json:"firmware_api_requirement"`
	APIApplicationIDSource   string `json:"api_application_id_source"`
	APIApplicationIDOverride string `json:"api_application_id_override,omitempty"`
	HandyConnectionKey       string `json:"handy_connection_key,omitempty"`
}

// PublicDeviceSettings is the API-safe device settings view.
type PublicDeviceSettings struct {
	ConfiguredDevices        []string `json:"configured_devices"`
	LinkedFailurePolicy      string   `json:"linked_failure_policy"`
	Output                   string   `json:"output"`
	HSPDispatchOwner         string   `json:"hsp_dispatch_owner"`
	IntifaceServerAddress    string   `json:"intiface_server_address"`
	FirmwareAPIRequirement   string   `json:"firmware_api_requirement"`
	APIApplicationIDSource   string   `json:"api_application_id_source"`
	APIApplicationIDOverride string   `json:"api_application_id_override,omitempty"`
	ConnectionKeySet         bool     `json:"connection_key_set"`
}

// DeviceUpdate is the API write payload for device settings.
type DeviceUpdate struct {
	ConfiguredDevices   []string `json:"configured_devices,omitempty"`
	LinkedFailurePolicy string   `json:"linked_failure_policy,omitempty"`
	// Output is preserved when an older client omits it.
	Output                   string  `json:"output,omitempty"`
	HSPDispatchOwner         string  `json:"hsp_dispatch_owner"`
	IntifaceServerAddress    string  `json:"intiface_server_address"`
	FirmwareAPIRequirement   string  `json:"firmware_api_requirement"`
	APIApplicationIDSource   string  `json:"api_application_id_source"`
	APIApplicationIDOverride string  `json:"api_application_id_override"`
	HandyConnectionKey       *string `json:"handy_connection_key,omitempty"`
}

// applyDeviceUpdate merges the device and Coyote sections. An update without
// an output keeps the current one, so older clients cannot reset it.
func applyDeviceUpdate(next Settings, update SettingsUpdate) Settings {
	if update.Device.LinkedFailurePolicy != "" {
		next.Device.LinkedFailurePolicy = update.Device.LinkedFailurePolicy
	}
	if update.Device.ConfiguredDevices != nil {
		next.Device.ConfiguredDevices = append([]string{}, update.Device.ConfiguredDevices...)
	}
	if output := strings.TrimSpace(update.Device.Output); output != "" {
		if update.Device.ConfiguredDevices == nil {
			next.Device = next.Device.WithPlaybackOutput(output)
		} else {
			next.Device.Output = output
		}
	}
	if update.Estim != nil {
		next.Estim = mergeEstimUpdate(next.Estim, *update.Estim)
	}
	next.Device.HSPDispatchOwner = update.Device.HSPDispatchOwner
	next.Device.IntifaceServerAddress = strings.TrimSpace(update.Device.IntifaceServerAddress)
	next.Device.FirmwareAPIRequirement = update.Device.FirmwareAPIRequirement
	next.Device.APIApplicationIDSource = update.Device.APIApplicationIDSource
	next.Device.APIApplicationIDOverride = strings.TrimSpace(update.Device.APIApplicationIDOverride)
	if update.ClearConnectionKey {
		next.Device.HandyConnectionKey = ""
	} else if update.Device.HandyConnectionKey != nil {
		next.Device.HandyConnectionKey = strings.TrimSpace(*update.Device.HandyConnectionKey)
	}
	if next.Device.APIApplicationIDSource == ApplicationIDSourceBundled {
		next.Device.APIApplicationIDOverride = ""
	}
	return next
}
