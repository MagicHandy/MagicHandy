package config

import (
	"errors"
	"strings"
)

// DeviceOptionsPatch changes individual device options as Settings > Devices
// edits them, so each one saves when it changes. A nil field keeps its value.
// A new Handy connection key keeps its own explicit save; this patch can only
// forget the saved one. The device list and the playback set keep their
// connection endpoint.
type DeviceOptionsPatch struct {
	LinkedFailurePolicy      *string         `json:"linked_failure_policy,omitempty"`
	IntifaceServerAddress    *string         `json:"intiface_server_address,omitempty"`
	APIApplicationIDSource   *string         `json:"api_application_id_source,omitempty"`
	APIApplicationIDOverride *string         `json:"api_application_id_override,omitempty"`
	ClearConnectionKey       bool            `json:"clear_connection_key,omitempty"`
	Estim                    *EstimPatch     `json:"estim,omitempty"`
	Vibration                *VibrationPatch `json:"vibration,omitempty"`
}

// EstimPatch changes individual Coyote settings. The safety notes keep their
// own endpoint; ForgetDevice clears the remembered address.
type EstimPatch struct {
	MaxStrengthA       *int    `json:"max_strength_a,omitempty"`
	MaxStrengthB       *int    `json:"max_strength_b,omitempty"`
	Follow             *string `json:"follow,omitempty"`
	FrequencyMinHz     *int    `json:"frequency_min_hz,omitempty"`
	FrequencyMaxHz     *int    `json:"frequency_max_hz,omitempty"`
	Balance            *int    `json:"balance,omitempty"`
	SwapChannels       *bool   `json:"swap_channels,omitempty"`
	TimingOffsetMillis *int    `json:"timing_offset_ms,omitempty"`
	FrequencyBalanceA  *int    `json:"frequency_balance_a,omitempty"`
	FrequencyBalanceB  *int    `json:"frequency_balance_b,omitempty"`
	IntensityBalanceA  *int    `json:"intensity_balance_a,omitempty"`
	IntensityBalanceB  *int    `json:"intensity_balance_b,omitempty"`
	ForgetDevice       bool    `json:"forget_device,omitempty"`
}

// VibrationPatch changes individual vibrator settings.
type VibrationPatch struct {
	Follow             *string `json:"follow,omitempty"`
	DepthEnd           *string `json:"depth_end,omitempty"`
	MinimumPercent     *int    `json:"minimum_percent,omitempty"`
	CeilingPercent     *int    `json:"ceiling_percent,omitempty"`
	TimingOffsetMillis *int    `json:"timing_offset_ms,omitempty"`
	LevelControl       *string `json:"level_control,omitempty"`
}

var errEmptyDeviceOptionsPatch = errors.New("the device options change names no setting")

// ApplyDeviceOptionsPatch returns the settings with the patch applied and
// validated. An invalid result is an error and leaves the settings unchanged.
func (s Settings) ApplyDeviceOptionsPatch(patch DeviceOptionsPatch) (Settings, error) {
	if patch.empty() {
		return s, errEmptyDeviceOptionsPatch
	}
	next := cloneSettings(s)
	patchString(&next.Device.LinkedFailurePolicy, patch.LinkedFailurePolicy)
	patchString(&next.Device.IntifaceServerAddress, patch.IntifaceServerAddress)
	patchString(&next.Device.APIApplicationIDSource, patch.APIApplicationIDSource)
	patchString(&next.Device.APIApplicationIDOverride, patch.APIApplicationIDOverride)
	if next.Device.APIApplicationIDSource == ApplicationIDSourceBundled {
		next.Device.APIApplicationIDOverride = ""
	}
	if patch.ClearConnectionKey {
		next.Device.HandyConnectionKey = ""
	}
	if estim := patch.Estim; estim != nil {
		patchInt(&next.Estim.MaxStrengthA, estim.MaxStrengthA)
		patchInt(&next.Estim.MaxStrengthB, estim.MaxStrengthB)
		patchString(&next.Estim.Follow, estim.Follow)
		patchInt(&next.Estim.FrequencyMinHz, estim.FrequencyMinHz)
		patchInt(&next.Estim.FrequencyMaxHz, estim.FrequencyMaxHz)
		patchInt(&next.Estim.Balance, estim.Balance)
		if estim.SwapChannels != nil {
			next.Estim.SwapChannels = *estim.SwapChannels
		}
		patchInt(&next.Estim.TimingOffsetMillis, estim.TimingOffsetMillis)
		patchInt(&next.Estim.FrequencyBalanceA, estim.FrequencyBalanceA)
		patchInt(&next.Estim.FrequencyBalanceB, estim.FrequencyBalanceB)
		patchInt(&next.Estim.IntensityBalanceA, estim.IntensityBalanceA)
		patchInt(&next.Estim.IntensityBalanceB, estim.IntensityBalanceB)
		if estim.ForgetDevice {
			next.Estim.RememberedAddress = ""
		}
	}
	if vibration := patch.Vibration; vibration != nil {
		patchString(&next.Vibration.Follow, vibration.Follow)
		patchString(&next.Vibration.DepthEnd, vibration.DepthEnd)
		patchInt(&next.Vibration.MinimumPercent, vibration.MinimumPercent)
		patchInt(&next.Vibration.CeilingPercent, vibration.CeilingPercent)
		patchInt(&next.Vibration.TimingOffsetMillis, vibration.TimingOffsetMillis)
		patchString(&next.Vibration.LevelControl, vibration.LevelControl)
	}
	return NormalizeSettings(next)
}

func (p DeviceOptionsPatch) empty() bool {
	estimEmpty := p.Estim == nil || *p.Estim == EstimPatch{}
	vibrationEmpty := p.Vibration == nil || *p.Vibration == VibrationPatch{}
	return p.LinkedFailurePolicy == nil && p.IntifaceServerAddress == nil && p.APIApplicationIDSource == nil &&
		p.APIApplicationIDOverride == nil && !p.ClearConnectionKey && estimEmpty && vibrationEmpty
}

func patchString(target *string, value *string) {
	if value != nil {
		*target = strings.TrimSpace(*value)
	}
}

func patchInt(target *int, value *int) {
	if value != nil {
		*target = *value
	}
}
