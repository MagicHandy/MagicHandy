package config

import (
	"fmt"
	"slices"
)

// DeviceOutputNone leaves the shared engine idle for chat-only use.
const DeviceOutputNone = "none"

// Linked session fault policies never change Emergency Stop behavior.
const (
	LinkedFailureStopAll         = "stop_all"
	LinkedFailureContinueHealthy = "continue_healthy"
)

// HasDevice checks the saved inventory, independent of connection state and
// Output (participation in the next playback). Nil migrates the old output-only
// configuration; an explicitly empty list must survive save and reload.
func (d DeviceSettings) HasDevice(device string) bool {
	if d.ConfiguredDevices == nil {
		return deviceIncluded(d.Output, device)
	}
	return slices.Contains(d.ConfiguredDevices, device)
}

func deviceIncluded(output, device string) bool {
	return device == DeviceOutputHandy && UsesHandy(output) || device == DeviceOutputCoyote && UsesCoyote(output)
}

func configuredFromOutput(output string) []string {
	devices := []string{}
	for _, device := range []string{DeviceOutputHandy, DeviceOutputCoyote} {
		if deviceIncluded(output, device) {
			devices = append(devices, device)
		}
	}
	return devices
}

func (d DeviceSettings) configuredSnapshot() []string {
	if d.ConfiguredDevices == nil {
		return configuredFromOutput(d.Output)
	}
	return append([]string{}, d.ConfiguredDevices...)
}

// WithPlaybackOutput retains existing connections. This also supports older
// clients that select an output before explicitly adding its connection.
func (d DeviceSettings) WithPlaybackOutput(output string) DeviceSettings {
	if d.ConfiguredDevices == nil {
		d.ConfiguredDevices = configuredFromOutput(d.Output)
	}
	d.ConfiguredDevices = append([]string{}, d.ConfiguredDevices...)
	for _, device := range configuredFromOutput(output) {
		if !d.HasDevice(device) {
			d.ConfiguredDevices = append(d.ConfiguredDevices, device)
		}
	}
	d.Output = output
	return d
}

// ChangeConnection applies one inventory or participation edit atomically.
// Adding never includes a device in playback; removing also excludes it.
func (d DeviceSettings) ChangeConnection(device, action string) (DeviceSettings, error) {
	if device != DeviceOutputHandy && device != DeviceOutputCoyote {
		return d, fmt.Errorf("unknown device %q", device)
	}
	if d.ConfiguredDevices == nil {
		d.ConfiguredDevices = configuredFromOutput(d.Output)
	}
	d.ConfiguredDevices = append([]string{}, d.ConfiguredDevices...)
	switch action {
	case "add":
		if !d.HasDevice(device) {
			d.ConfiguredDevices = append(d.ConfiguredDevices, device)
		}
	case "remove":
		d.ConfiguredDevices = slices.DeleteFunc(d.ConfiguredDevices, func(value string) bool { return value == device })
		d.Output = outputWithDevice(d.Output, device, false)
	case "include", "exclude":
		if !d.HasDevice(device) {
			return d, fmt.Errorf("add the device before changing playback participation")
		}
		d.Output = outputWithDevice(d.Output, device, action == "include")
	default:
		return d, fmt.Errorf("unknown connection action %q", action)
	}
	return d, nil
}

func outputWithDevice(output, device string, included bool) string {
	handy, coyote := UsesHandy(output), UsesCoyote(output)
	if device == DeviceOutputHandy {
		handy = included
	} else {
		coyote = included
	}
	switch {
	case handy && coyote:
		return DeviceOutputHandyCoyote
	case handy:
		return DeviceOutputHandy
	case coyote:
		return DeviceOutputCoyote
	default:
		return DeviceOutputNone
	}
}

func validateDeviceConnections(d DeviceSettings) error {
	if d.LinkedFailurePolicy != LinkedFailureStopAll && d.LinkedFailurePolicy != LinkedFailureContinueHealthy {
		return fmt.Errorf("unknown linked failure policy %q", d.LinkedFailurePolicy)
	}
	seen := map[string]bool{}
	for _, device := range d.ConfiguredDevices {
		if device != DeviceOutputHandy && device != DeviceOutputCoyote || seen[device] {
			return fmt.Errorf("invalid or duplicate configured device %q", device)
		}
		seen[device] = true
	}
	for _, device := range configuredFromOutput(d.Output) {
		if !d.HasDevice(device) {
			return fmt.Errorf("playback device %q is not configured", device)
		}
	}
	return nil
}
