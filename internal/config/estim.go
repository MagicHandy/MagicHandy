package config

import (
	"errors"
	"fmt"
	"strings"
)

// Device outputs. The output chooses what the shared motion frame drives; the
// dispatch owner still chooses how the Handy is reached when it is included.
const (
	// DeviceOutputHandy drives The Handy alone.
	DeviceOutputHandy = "handy"
	// DeviceOutputCoyote drives a DG-LAB Coyote alone (ADR 0044).
	DeviceOutputCoyote = "coyote"
	// DeviceOutputHandyCoyote drives both from the same frame.
	DeviceOutputHandyCoyote = "handy_coyote"
)

// Coyote conversion profiles: what the shared frame shapes beneath the
// user's strength. None of them maps position to channel strength.
const (
	EstimFollowStroke   = "stroke"
	EstimFollowSpeed    = "speed"
	EstimFollowPosition = "position"
)

// Coyote setting ranges.
const (
	EstimMaxStrength          = 200
	EstimDefaultMaxStrength   = 30
	EstimMinFrequencyHz       = 1
	EstimMaxFrequencyHz       = 100
	EstimDefaultMinHz         = 40
	EstimDefaultMaxHz         = 100
	EstimMaxBalance           = 100
	EstimMaxTimingOffsetMs    = 300
	EstimMaxDeviceBalance     = 255
	EstimDefaultFreqBalance   = 160
	EstimDefaultIntensityBias = 0
)

// EstimSettings configures DG-LAB Coyote output. The strength ceilings are
// written to the device itself (BF) on every connection; MagicHandy never
// raises channel strength on its own, so these are the user's hard limits.
// RememberedAddress lets recent Coyote firmware reconnect without pairing
// mode; it is a hardware identifier, never shown to non-host clients.
type EstimSettings struct {
	SafetyAcknowledged bool   `json:"safety_acknowledged"`
	MaxStrengthA       int    `json:"max_strength_a"`
	MaxStrengthB       int    `json:"max_strength_b"`
	Follow             string `json:"follow"`
	FrequencyMinHz     int    `json:"frequency_min_hz"`
	FrequencyMaxHz     int    `json:"frequency_max_hz"`
	Balance            int    `json:"balance"`
	SwapChannels       bool   `json:"swap_channels"`
	TimingOffsetMillis int    `json:"timing_offset_ms"`
	FrequencyBalanceA  int    `json:"frequency_balance_a"`
	FrequencyBalanceB  int    `json:"frequency_balance_b"`
	IntensityBalanceA  int    `json:"intensity_balance_a"`
	IntensityBalanceB  int    `json:"intensity_balance_b"`
	RememberedAddress  string `json:"remembered_address,omitempty"`
}

// PublicEstimSettings is the API view: the remembered address becomes a flag.
type PublicEstimSettings struct {
	SafetyAcknowledged bool   `json:"safety_acknowledged"`
	MaxStrengthA       int    `json:"max_strength_a"`
	MaxStrengthB       int    `json:"max_strength_b"`
	Follow             string `json:"follow"`
	FrequencyMinHz     int    `json:"frequency_min_hz"`
	FrequencyMaxHz     int    `json:"frequency_max_hz"`
	Balance            int    `json:"balance"`
	SwapChannels       bool   `json:"swap_channels"`
	TimingOffsetMillis int    `json:"timing_offset_ms"`
	FrequencyBalanceA  int    `json:"frequency_balance_a"`
	FrequencyBalanceB  int    `json:"frequency_balance_b"`
	IntensityBalanceA  int    `json:"intensity_balance_a"`
	IntensityBalanceB  int    `json:"intensity_balance_b"`
	RememberedDevice   bool   `json:"remembered_device"`
}

// EstimUpdate is the settings API write shape. ForgetDevice clears the
// remembered address; the server alone records a new one after connecting.
type EstimUpdate struct {
	SafetyAcknowledged bool   `json:"safety_acknowledged"`
	MaxStrengthA       int    `json:"max_strength_a"`
	MaxStrengthB       int    `json:"max_strength_b"`
	Follow             string `json:"follow"`
	FrequencyMinHz     int    `json:"frequency_min_hz"`
	FrequencyMaxHz     int    `json:"frequency_max_hz"`
	Balance            int    `json:"balance"`
	SwapChannels       bool   `json:"swap_channels"`
	TimingOffsetMillis int    `json:"timing_offset_ms"`
	FrequencyBalanceA  int    `json:"frequency_balance_a"`
	FrequencyBalanceB  int    `json:"frequency_balance_b"`
	IntensityBalanceA  int    `json:"intensity_balance_a"`
	IntensityBalanceB  int    `json:"intensity_balance_b"`
	ForgetDevice       bool   `json:"forget_device"`
}

// DefaultEstimSettings returns first-run Coyote settings: low ceilings, the
// stroke profile and DG-LAB's commonly used balance values.
func DefaultEstimSettings() EstimSettings {
	return EstimSettings{
		MaxStrengthA:      EstimDefaultMaxStrength,
		MaxStrengthB:      EstimDefaultMaxStrength,
		Follow:            EstimFollowStroke,
		FrequencyMinHz:    EstimDefaultMinHz,
		FrequencyMaxHz:    EstimDefaultMaxHz,
		FrequencyBalanceA: EstimDefaultFreqBalance,
		FrequencyBalanceB: EstimDefaultFreqBalance,
		IntensityBalanceA: EstimDefaultIntensityBias,
		IntensityBalanceB: EstimDefaultIntensityBias,
	}
}

// Public returns the API view.
func (s EstimSettings) Public() PublicEstimSettings {
	return PublicEstimSettings{
		SafetyAcknowledged: s.SafetyAcknowledged,
		MaxStrengthA:       s.MaxStrengthA,
		MaxStrengthB:       s.MaxStrengthB,
		Follow:             s.Follow,
		FrequencyMinHz:     s.FrequencyMinHz,
		FrequencyMaxHz:     s.FrequencyMaxHz,
		Balance:            s.Balance,
		SwapChannels:       s.SwapChannels,
		TimingOffsetMillis: s.TimingOffsetMillis,
		FrequencyBalanceA:  s.FrequencyBalanceA,
		FrequencyBalanceB:  s.FrequencyBalanceB,
		IntensityBalanceA:  s.IntensityBalanceA,
		IntensityBalanceB:  s.IntensityBalanceB,
		RememberedDevice:   s.RememberedAddress != "",
	}
}

func mergeEstimUpdate(current EstimSettings, update EstimUpdate) EstimSettings {
	next := EstimSettings{
		SafetyAcknowledged: update.SafetyAcknowledged,
		MaxStrengthA:       update.MaxStrengthA,
		MaxStrengthB:       update.MaxStrengthB,
		Follow:             strings.TrimSpace(update.Follow),
		FrequencyMinHz:     update.FrequencyMinHz,
		FrequencyMaxHz:     update.FrequencyMaxHz,
		Balance:            update.Balance,
		SwapChannels:       update.SwapChannels,
		TimingOffsetMillis: update.TimingOffsetMillis,
		FrequencyBalanceA:  update.FrequencyBalanceA,
		FrequencyBalanceB:  update.FrequencyBalanceB,
		IntensityBalanceA:  update.IntensityBalanceA,
		IntensityBalanceB:  update.IntensityBalanceB,
		RememberedAddress:  current.RememberedAddress,
	}
	if update.ForgetDevice {
		next.RememberedAddress = ""
	}
	return next
}

// applyMissingEstimDefaults fills a block that older settings files lack.
// An empty profile marks a block that was never saved; zero ceilings in a
// saved block are a deliberate user choice and stay.
func applyMissingEstimDefaults(settings EstimSettings) EstimSettings {
	defaults := DefaultEstimSettings()
	if strings.TrimSpace(settings.Follow) == "" {
		defaults.SafetyAcknowledged = settings.SafetyAcknowledged
		defaults.RememberedAddress = settings.RememberedAddress
		return defaults
	}
	if settings.FrequencyMinHz == 0 {
		settings.FrequencyMinHz = defaults.FrequencyMinHz
	}
	if settings.FrequencyMaxHz == 0 {
		settings.FrequencyMaxHz = defaults.FrequencyMaxHz
	}
	settings.RememberedAddress = strings.TrimSpace(settings.RememberedAddress)
	return settings
}

func validateDeviceOutput(output string) error {
	if !oneOf(output, DeviceOutputNone, DeviceOutputHandy, DeviceOutputCoyote, DeviceOutputHandyCoyote) {
		return fmt.Errorf("unknown device output %q", output)
	}
	return nil
}

func validateEstimSettings(settings EstimSettings) error {
	if !oneOf(settings.Follow, EstimFollowStroke, EstimFollowSpeed, EstimFollowPosition) {
		return fmt.Errorf("unknown Coyote profile %q", settings.Follow)
	}
	for _, ceiling := range []int{settings.MaxStrengthA, settings.MaxStrengthB} {
		if ceiling < 0 || ceiling > EstimMaxStrength {
			return fmt.Errorf("the Coyote strength ceilings must be between 0 and %d", EstimMaxStrength)
		}
	}
	if settings.FrequencyMinHz < EstimMinFrequencyHz || settings.FrequencyMaxHz > EstimMaxFrequencyHz || settings.FrequencyMinHz > settings.FrequencyMaxHz {
		return fmt.Errorf("the Coyote pulse rates must satisfy %d <= minimum <= maximum <= %d Hz", EstimMinFrequencyHz, EstimMaxFrequencyHz)
	}
	if settings.Balance < -EstimMaxBalance || settings.Balance > EstimMaxBalance {
		return fmt.Errorf("the Coyote channel balance must be between -%d and %d", EstimMaxBalance, EstimMaxBalance)
	}
	if settings.TimingOffsetMillis < -EstimMaxTimingOffsetMs || settings.TimingOffsetMillis > EstimMaxTimingOffsetMs {
		return fmt.Errorf("the Coyote timing offset must be between -%d and %d ms", EstimMaxTimingOffsetMs, EstimMaxTimingOffsetMs)
	}
	for _, value := range []int{settings.FrequencyBalanceA, settings.FrequencyBalanceB, settings.IntensityBalanceA, settings.IntensityBalanceB} {
		if value < 0 || value > EstimMaxDeviceBalance {
			return fmt.Errorf("the Coyote balance parameters must be between 0 and %d", EstimMaxDeviceBalance)
		}
	}
	if len(settings.RememberedAddress) > 64 {
		return errors.New("the remembered Coyote address is too long")
	}
	return nil
}

// UsesCoyote reports whether output includes the Coyote.
func UsesCoyote(output string) bool {
	return output == DeviceOutputCoyote || output == DeviceOutputHandyCoyote
}

// UsesHandy reports whether output includes The Handy.
func UsesHandy(output string) bool {
	return output == DeviceOutputHandy || output == DeviceOutputHandyCoyote
}
