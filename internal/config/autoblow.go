package config

import (
	"fmt"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/autoblow"
)

// Autoblow's cloud devices (ADR 0069). The Autoblow Ultra is a stroker that
// takes positions; the VacuGlide plays tempo and sets its stroke with air
// (ADR 0071).
const (
	DeviceAutoblow  = "autoblow"
	DeviceVacuGlide = "vacuglide"
)

// The VacuGlide's Tempo range is in percent of its motor's speed. Tempo 1
// plays at the minimum and tempo 100 at the maximum. The floor keeps the
// motor above its stall point; the device's speed buttons move the maximum
// by VacuGlideTempoButtonStep.
const (
	VacuGlideTempoFloorPercent = 5
	VacuGlideTempoButtonStep   = 10
)

// AutoblowMaxTimingOffsetMs bounds a cloud device's timing offset.
const AutoblowMaxTimingOffsetMs = 500

// AutoblowSettings configures the Autoblow Ultra. The device token gives full
// control of the device: it is private and never leaves the backend.
type AutoblowSettings struct {
	DeviceToken        string `json:"device_token,omitempty"`
	TimingOffsetMillis int    `json:"timing_offset_ms"`
}

// VacuGlideSettings configures the VacuGlide. Its token is private too.
type VacuGlideSettings struct {
	DeviceToken        string `json:"device_token,omitempty"`
	TimingOffsetMillis int    `json:"timing_offset_ms"`
	// TempoMinPercent and TempoMaxPercent are the motor speeds tempo 1 and
	// tempo 100 play at.
	TempoMinPercent int `json:"tempo_min_percent"`
	TempoMaxPercent int `json:"tempo_max_percent"`
	// MotionAir lets chat, Autopilot and routines step the stroke valves.
	MotionAir bool `json:"motion_air"`
}

// PublicAutoblowSettings is the API-safe view: whether a token is saved.
type PublicAutoblowSettings struct {
	TokenSet           bool `json:"token_set"`
	TimingOffsetMillis int  `json:"timing_offset_ms"`
}

// PublicVacuGlideSettings is the API-safe VacuGlide view.
type PublicVacuGlideSettings struct {
	TokenSet           bool `json:"token_set"`
	TimingOffsetMillis int  `json:"timing_offset_ms"`
	TempoMinPercent    int  `json:"tempo_min_percent"`
	TempoMaxPercent    int  `json:"tempo_max_percent"`
	MotionAir          bool `json:"motion_air"`
}

// AutoblowPatch changes individual Ultra settings. The token keeps its own
// explicit Save; ClearToken forgets it.
type AutoblowPatch struct {
	TimingOffsetMillis *int `json:"timing_offset_ms,omitempty"`
	ClearToken         bool `json:"clear_token,omitempty"`
}

// VacuGlidePatch changes individual VacuGlide settings.
type VacuGlidePatch struct {
	TimingOffsetMillis *int  `json:"timing_offset_ms,omitempty"`
	TempoMinPercent    *int  `json:"tempo_min_percent,omitempty"`
	TempoMaxPercent    *int  `json:"tempo_max_percent,omitempty"`
	MotionAir          *bool `json:"motion_air,omitempty"`
	ClearToken         bool  `json:"clear_token,omitempty"`
}

// DefaultVacuGlideSettings spans the motor's full range and leaves the air to
// the person.
func DefaultVacuGlideSettings() VacuGlideSettings {
	return VacuGlideSettings{TempoMinPercent: VacuGlideTempoFloorPercent, TempoMaxPercent: 100}
}

// Public returns the API-safe Ultra view.
func (s AutoblowSettings) Public() PublicAutoblowSettings {
	return PublicAutoblowSettings{TokenSet: s.DeviceToken != "", TimingOffsetMillis: s.TimingOffsetMillis}
}

// Public returns the API-safe VacuGlide view.
func (s VacuGlideSettings) Public() PublicVacuGlideSettings {
	return PublicVacuGlideSettings{TokenSet: s.DeviceToken != "", TimingOffsetMillis: s.TimingOffsetMillis,
		TempoMinPercent: s.TempoMinPercent, TempoMaxPercent: s.TempoMaxPercent, MotionAir: s.MotionAir}
}

// AutoblowToken returns the saved token of an Autoblow device.
func (s Settings) AutoblowToken(device string) string {
	if device == DeviceVacuGlide {
		return s.VacuGlide.DeviceToken
	}
	return s.Autoblow.DeviceToken
}

// WithAutoblowToken saves a device token. The token is checked for shape so a
// pasted value cannot carry header or query syntax.
func (s Settings) WithAutoblowToken(device, token string) (Settings, error) {
	token = strings.TrimSpace(token)
	if !autoblow.ValidToken(token) {
		return s, autoblow.ErrBadToken
	}
	switch device {
	case DeviceAutoblow:
		s.Autoblow.DeviceToken = token
	case DeviceVacuGlide:
		s.VacuGlide.DeviceToken = token
	default:
		return s, fmt.Errorf("unknown Autoblow device %q", device)
	}
	return s, nil
}

// Settings saved before the Tempo range (ADR 0071) have neither bound.
func applyMissingVacuGlideDefaults(settings VacuGlideSettings) VacuGlideSettings {
	defaults := DefaultVacuGlideSettings()
	if settings.TempoMinPercent == 0 {
		settings.TempoMinPercent = defaults.TempoMinPercent
	}
	if settings.TempoMaxPercent == 0 {
		settings.TempoMaxPercent = defaults.TempoMaxPercent
	}
	return settings
}

func validateAutoblowSettings(settings Settings) error {
	for name, offset := range map[string]int{"Autoblow Ultra": settings.Autoblow.TimingOffsetMillis, "VacuGlide": settings.VacuGlide.TimingOffsetMillis} {
		if offset < -AutoblowMaxTimingOffsetMs || offset > AutoblowMaxTimingOffsetMs {
			return fmt.Errorf("the %s timing offset must be between -%d and %d ms", name, AutoblowMaxTimingOffsetMs, AutoblowMaxTimingOffsetMs)
		}
	}
	if low, high := settings.VacuGlide.TempoMinPercent, settings.VacuGlide.TempoMaxPercent; low < VacuGlideTempoFloorPercent || high > 100 || low > high {
		return fmt.Errorf("the VacuGlide's Tempo range must satisfy %d <= minimum <= maximum <= 100", VacuGlideTempoFloorPercent)
	}
	for _, token := range []string{settings.Autoblow.DeviceToken, settings.VacuGlide.DeviceToken} {
		if token != "" && !autoblow.ValidToken(token) {
			return autoblow.ErrBadToken
		}
	}
	return nil
}

func (p *AutoblowPatch) empty() bool {
	return p == nil || *p == AutoblowPatch{}
}

func (p *VacuGlidePatch) empty() bool {
	return p == nil || *p == VacuGlidePatch{}
}

func applyAutoblowPatches(next *Settings, ultra *AutoblowPatch, vacuglide *VacuGlidePatch) {
	if ultra != nil {
		patchInt(&next.Autoblow.TimingOffsetMillis, ultra.TimingOffsetMillis)
		if ultra.ClearToken {
			next.Autoblow.DeviceToken = ""
		}
	}
	if vacuglide != nil {
		patchInt(&next.VacuGlide.TimingOffsetMillis, vacuglide.TimingOffsetMillis)
		patchInt(&next.VacuGlide.TempoMinPercent, vacuglide.TempoMinPercent)
		patchInt(&next.VacuGlide.TempoMaxPercent, vacuglide.TempoMaxPercent)
		if vacuglide.MotionAir != nil {
			next.VacuGlide.MotionAir = *vacuglide.MotionAir
		}
		if vacuglide.ClearToken {
			next.VacuGlide.DeviceToken = ""
		}
	}
}
