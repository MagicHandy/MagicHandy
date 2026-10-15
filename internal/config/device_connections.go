package config

import (
	"fmt"
	"slices"
	"strings"
)

// DeviceOutputNone leaves the shared engine idle for chat-only use.
const DeviceOutputNone = "none"

// Linked session fault policies never change Emergency Stop behavior.
const (
	LinkedFailureStopAll         = "stop_all"
	LinkedFailureContinueHealthy = "continue_healthy"
)

// knownDevices lists the device IDs in run-clock order: the first playing
// device anchors a linked session's timeline.
var knownDevices = []string{DeviceOutputHandy, DeviceOutputCoyote}

// HasDevice checks the saved inventory, independent of connection state and
// playback participation. Nil migrates the old output-only configuration; an
// explicitly empty list must survive save and reload.
func (d DeviceSettings) HasDevice(device string) bool {
	if d.ConfiguredDevices == nil {
		return slices.Contains(playbackFromOutput(d.Output), device)
	}
	return slices.Contains(d.ConfiguredDevices, device)
}

// Plays reports whether device takes part in the next playback run.
func (d DeviceSettings) Plays(device string) bool {
	return slices.Contains(d.playbackSnapshot(), device)
}

// PlaybackSet returns the playback participants in run-clock order.
func (d DeviceSettings) PlaybackSet() []string {
	return d.playbackSnapshot()
}

// playbackSnapshot is authoritative when PlaybackDevices is set. Nil migrates
// the legacy output, which only ever named the Handy and the Coyote.
func (d DeviceSettings) playbackSnapshot() []string {
	if d.PlaybackDevices == nil {
		return playbackFromOutput(d.Output)
	}
	return canonicalDevices(d.PlaybackDevices)
}

func playbackFromOutput(output string) []string {
	devices := []string{}
	if UsesHandy(output) {
		devices = append(devices, DeviceOutputHandy)
	}
	if UsesCoyote(output) {
		devices = append(devices, DeviceOutputCoyote)
	}
	return devices
}

func configuredFromOutput(output string) []string {
	return playbackFromOutput(output)
}

// canonicalDevices orders known devices by run-clock order. Unknown IDs keep
// their relative order at the end so validation can still name them.
func canonicalDevices(devices []string) []string {
	ordered := make([]string, 0, len(devices))
	for _, known := range knownDevices {
		if slices.Contains(devices, known) {
			ordered = append(ordered, known)
		}
	}
	for _, device := range devices {
		if !slices.Contains(knownDevices, device) {
			ordered = append(ordered, device)
		}
	}
	return ordered
}

// legacyOutput projects the playback set onto the four-value output that
// older clients read. It cannot represent devices beyond the Handy and Coyote.
func legacyOutput(playback []string) string {
	handy, coyote := slices.Contains(playback, DeviceOutputHandy), slices.Contains(playback, DeviceOutputCoyote)
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

func (d DeviceSettings) configuredSnapshot() []string {
	if d.ConfiguredDevices == nil {
		return configuredFromOutput(d.Output)
	}
	return append([]string{}, d.ConfiguredDevices...)
}

func (d DeviceSettings) materialized() DeviceSettings {
	d.ConfiguredDevices = d.configuredSnapshot()
	d.PlaybackDevices = d.playbackSnapshot()
	return d
}

// WithPlaybackDevices replaces the playback set and keeps the legacy output
// in step with it.
func (d DeviceSettings) WithPlaybackDevices(devices []string) DeviceSettings {
	d.PlaybackDevices = canonicalDevices(devices)
	d.Output = legacyOutput(d.PlaybackDevices)
	return d
}

// WithLegacyOutput applies an output written by an older client: it sets the
// Handy and Coyote members and never removes any other playing device.
func (d DeviceSettings) WithLegacyOutput(output string) DeviceSettings {
	d = d.materialized()
	next := slices.DeleteFunc(append([]string{}, d.PlaybackDevices...), func(device string) bool {
		return device == DeviceOutputHandy || device == DeviceOutputCoyote
	})
	return d.WithPlaybackDevices(append(next, playbackFromOutput(output)...))
}

// WithPlaybackOutput retains existing connections. This also supports older
// clients that select an output before explicitly adding its connection.
func (d DeviceSettings) WithPlaybackOutput(output string) DeviceSettings {
	d = d.materialized()
	for _, device := range playbackFromOutput(output) {
		if !slices.Contains(d.ConfiguredDevices, device) {
			d.ConfiguredDevices = append(d.ConfiguredDevices, device)
		}
	}
	return d.WithLegacyOutput(output)
}

// ChangeConnection applies one inventory or participation edit atomically.
// Adding never includes a device in playback; removing also excludes it.
func (d DeviceSettings) ChangeConnection(device, action string) (DeviceSettings, error) {
	if !slices.Contains(knownDevices, device) {
		return d, fmt.Errorf("unknown device %q", device)
	}
	d = d.materialized()
	playback := d.PlaybackDevices
	switch action {
	case "add":
		if !slices.Contains(d.ConfiguredDevices, device) {
			d.ConfiguredDevices = append(d.ConfiguredDevices, device)
		}
	case "remove":
		d.ConfiguredDevices = slices.DeleteFunc(d.ConfiguredDevices, func(value string) bool { return value == device })
		playback = slices.DeleteFunc(playback, func(value string) bool { return value == device })
	case "include", "exclude":
		if !slices.Contains(d.ConfiguredDevices, device) {
			return d, fmt.Errorf("add the device before changing playback participation")
		}
		playback = slices.DeleteFunc(playback, func(value string) bool { return value == device })
		if action == "include" {
			playback = append(playback, device)
		}
	default:
		return d, fmt.Errorf("unknown connection action %q", action)
	}
	return d.WithPlaybackDevices(playback), nil
}

// normalizeDeviceParticipation migrates the inventory and the playback set
// from the legacy output when they are missing. A stored set is only
// reordered; validation rejects one that disagrees with the output.
func normalizeDeviceParticipation(d DeviceSettings, defaultOutput string) DeviceSettings {
	d.Output = strings.TrimSpace(d.Output)
	if d.Output == "" {
		d.Output = defaultOutput
	}
	if d.ConfiguredDevices == nil {
		d.ConfiguredDevices = configuredFromOutput(d.Output)
	}
	if d.PlaybackDevices == nil {
		d.PlaybackDevices = playbackFromOutput(d.Output)
	} else {
		d.PlaybackDevices = canonicalDevices(d.PlaybackDevices)
	}
	return d
}

func validateDeviceConnections(d DeviceSettings) error {
	if d.LinkedFailurePolicy != LinkedFailureStopAll && d.LinkedFailurePolicy != LinkedFailureContinueHealthy {
		return fmt.Errorf("unknown linked failure policy %q", d.LinkedFailurePolicy)
	}
	if err := validateDeviceList("configured device", d.ConfiguredDevices); err != nil {
		return err
	}
	if err := validateDeviceList("playback device", d.PlaybackDevices); err != nil {
		return err
	}
	for _, device := range d.playbackSnapshot() {
		if !d.HasDevice(device) {
			return fmt.Errorf("playback device %q is not configured", device)
		}
	}
	if d.PlaybackDevices != nil && d.Output != legacyOutput(d.playbackSnapshot()) {
		return fmt.Errorf("device output %q disagrees with the playback devices", d.Output)
	}
	return nil
}

func validateDeviceList(kind string, devices []string) error {
	seen := map[string]bool{}
	for _, device := range devices {
		if !slices.Contains(knownDevices, device) || seen[device] {
			return fmt.Errorf("invalid or duplicate %s %q", kind, device)
		}
		seen[device] = true
	}
	return nil
}
