package config

import (
	"fmt"
	"slices"
	"strings"
)

// DeviceSettings contains device transport configuration.
type DeviceSettings struct {
	HandyConnections []HandyConnection `json:"handy_connections,omitempty"`
	// NativeBluetoothAddress is a private hardware identifier, never a public setting.
	NativeBluetoothAddress string   `json:"native_bluetooth_address,omitempty"`
	ConfiguredDevices      []string `json:"configured_devices"`
	// PlaybackDevices is the playback participant set. Nil migrates the legacy
	// Output; an explicitly empty set means chat only.
	PlaybackDevices     []string `json:"playback_devices"`
	LinkedFailurePolicy string   `json:"linked_failure_policy"`
	// Output is the legacy projection of the Handy and Coyote members, kept in
	// step with PlaybackDevices for older clients and older versions.
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
	HandyConnections         []PublicHandyConnection `json:"handy_connections"`
	ConfiguredDevices        []string                `json:"configured_devices"`
	PlaybackDevices          []string                `json:"playback_devices"`
	LinkedFailurePolicy      string                  `json:"linked_failure_policy"`
	Output                   string                  `json:"output"`
	HSPDispatchOwner         string                  `json:"hsp_dispatch_owner"`
	IntifaceServerAddress    string                  `json:"intiface_server_address"`
	FirmwareAPIRequirement   string                  `json:"firmware_api_requirement"`
	APIApplicationIDSource   string                  `json:"api_application_id_source"`
	APIApplicationIDOverride string                  `json:"api_application_id_override,omitempty"`
	ConnectionKeySet         bool                    `json:"connection_key_set"`
}

// DeviceUpdate is the API write payload for device settings.
type DeviceUpdate struct {
	ConfiguredDevices []string `json:"configured_devices,omitempty"`
	// PlaybackDevices replaces the playback set; nil keeps it, and an empty
	// set means chat only.
	PlaybackDevices     *[]string `json:"playback_devices,omitempty"`
	LinkedFailurePolicy string    `json:"linked_failure_policy,omitempty"`
	// Output is the legacy participation write. It is preserved when an older
	// client omits it, and changes only the Handy and Coyote members.
	Output                   string  `json:"output,omitempty"`
	HSPDispatchOwner         string  `json:"hsp_dispatch_owner"`
	IntifaceServerAddress    string  `json:"intiface_server_address"`
	FirmwareAPIRequirement   string  `json:"firmware_api_requirement"`
	APIApplicationIDSource   string  `json:"api_application_id_source"`
	APIApplicationIDOverride string  `json:"api_application_id_override"`
	HandyConnectionKey       *string `json:"handy_connection_key,omitempty"`
}

// applyDeviceUpdate merges the device and Coyote sections. An update without
// a playback set or output keeps the current participants, so older clients
// cannot reset them. A write naming both must agree.
func applyDeviceUpdate(next Settings, update SettingsUpdate) (Settings, error) {
	if update.Device.LinkedFailurePolicy != "" {
		next.Device.LinkedFailurePolicy = update.Device.LinkedFailurePolicy
	}
	if update.Device.ConfiguredDevices != nil {
		next.Device.ConfiguredDevices = append([]string{}, update.Device.ConfiguredDevices...)
		// Older full-settings writers cannot remove named instances.
		for _, connection := range next.Device.HandyConnections {
			if !slices.Contains(next.Device.ConfiguredDevices, connection.ID) {
				next.Device.ConfiguredDevices = append(next.Device.ConfiguredDevices, connection.ID)
			}
		}
	}
	output := strings.TrimSpace(update.Device.Output)
	switch {
	case update.Device.PlaybackDevices != nil:
		playback := canonicalDevices(*update.Device.PlaybackDevices)
		if output != "" && output != legacyOutput(playback) {
			return next, fmt.Errorf("device output %q conflicts with the playback devices", output)
		}
		next.Device = next.Device.materialized().WithPlaybackDevices(playback)
	case output != "" && update.Device.ConfiguredDevices == nil:
		next.Device = next.Device.WithPlaybackOutput(output)
	case output != "":
		next.Device = next.Device.WithLegacyOutput(output)
	}
	if update.Estim != nil {
		next.Estim = mergeEstimUpdate(next.Estim, *update.Estim)
	}
	if update.Vibration != nil {
		next.Vibration = *update.Vibration
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
	return next, nil
}
