package config

import (
	"fmt"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/autoblow"
)

// Autoblow's cloud devices (ADR 0069). The Autoblow Ultra is a stroker that
// takes positions; the VacuGlide follows pace and adjusts its stroke with air.
const (
	DeviceAutoblow  = "autoblow"
	DeviceVacuGlide = "vacuglide"
)

// How the VacuGlide's motor speed matches MagicHandy's speeds.
const (
	// VacuGlideSpeedHandyPace keeps the pace a speed has on The Handy.
	VacuGlideSpeedHandyPace = "handy_pace"
	// VacuGlideSpeedFullRange reaches the motor's top speed at MagicHandy's.
	VacuGlideSpeedFullRange = "full_range"
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
	SpeedMatch         string `json:"speed_match"`
	// StrokeFollow lets the stroke valves follow how far the motion reaches.
	StrokeFollow bool `json:"stroke_follow"`
}

// PublicAutoblowSettings is the API-safe view: whether a token is saved.
type PublicAutoblowSettings struct {
	TokenSet           bool `json:"token_set"`
	TimingOffsetMillis int  `json:"timing_offset_ms"`
}

// PublicVacuGlideSettings is the API-safe VacuGlide view.
type PublicVacuGlideSettings struct {
	TokenSet           bool   `json:"token_set"`
	TimingOffsetMillis int    `json:"timing_offset_ms"`
	SpeedMatch         string `json:"speed_match"`
	StrokeFollow       bool   `json:"stroke_follow"`
}

// AutoblowPatch changes individual Ultra settings. The token keeps its own
// explicit Save; ClearToken forgets it.
type AutoblowPatch struct {
	TimingOffsetMillis *int `json:"timing_offset_ms,omitempty"`
	ClearToken         bool `json:"clear_token,omitempty"`
}

// VacuGlidePatch changes individual VacuGlide settings.
type VacuGlidePatch struct {
	TimingOffsetMillis *int    `json:"timing_offset_ms,omitempty"`
	SpeedMatch         *string `json:"speed_match,omitempty"`
	StrokeFollow       *bool   `json:"stroke_follow,omitempty"`
	ClearToken         bool    `json:"clear_token,omitempty"`
}

// DefaultVacuGlideSettings keeps The Handy's pace and leaves the valves to
// the person.
func DefaultVacuGlideSettings() VacuGlideSettings {
	return VacuGlideSettings{SpeedMatch: VacuGlideSpeedHandyPace}
}

// Public returns the API-safe Ultra view.
func (s AutoblowSettings) Public() PublicAutoblowSettings {
	return PublicAutoblowSettings{TokenSet: s.DeviceToken != "", TimingOffsetMillis: s.TimingOffsetMillis}
}

// Public returns the API-safe VacuGlide view.
func (s VacuGlideSettings) Public() PublicVacuGlideSettings {
	return PublicVacuGlideSettings{TokenSet: s.DeviceToken != "", TimingOffsetMillis: s.TimingOffsetMillis,
		SpeedMatch: s.SpeedMatch, StrokeFollow: s.StrokeFollow}
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

func applyMissingVacuGlideDefaults(settings VacuGlideSettings) VacuGlideSettings {
	if settings.SpeedMatch == "" {
		settings.SpeedMatch = VacuGlideSpeedHandyPace
	}
	return settings
}

func validateAutoblowSettings(settings Settings) error {
	for name, offset := range map[string]int{"Autoblow Ultra": settings.Autoblow.TimingOffsetMillis, "VacuGlide": settings.VacuGlide.TimingOffsetMillis} {
		if offset < -AutoblowMaxTimingOffsetMs || offset > AutoblowMaxTimingOffsetMs {
			return fmt.Errorf("the %s timing offset must be between -%d and %d ms", name, AutoblowMaxTimingOffsetMs, AutoblowMaxTimingOffsetMs)
		}
	}
	if settings.VacuGlide.SpeedMatch != VacuGlideSpeedHandyPace && settings.VacuGlide.SpeedMatch != VacuGlideSpeedFullRange {
		return fmt.Errorf("unknown VacuGlide speed match %q", settings.VacuGlide.SpeedMatch)
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
		patchString(&next.VacuGlide.SpeedMatch, vacuglide.SpeedMatch)
		if vacuglide.StrokeFollow != nil {
			next.VacuGlide.StrokeFollow = *vacuglide.StrokeFollow
		}
		if vacuglide.ClearToken {
			next.VacuGlide.DeviceToken = ""
		}
	}
}
