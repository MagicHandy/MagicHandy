package config

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/autoblow"
)

func TestAutoblowTokensStayPrivate(t *testing.T) {
	settings := DefaultSettings()
	if vg := settings.VacuGlide; vg.TempoMinPercent != VacuGlideTempoFloorPercent || vg.TempoMaxPercent != 100 || vg.MotionAir {
		t.Fatalf("default VacuGlide = %+v, want the full motor range and air left to the person", vg)
	}
	var err error
	if settings, err = settings.WithAutoblowToken(DeviceAutoblow, "  ultra-secret-token  "); err != nil {
		t.Fatal(err)
	}
	if settings, err = settings.WithAutoblowToken(DeviceVacuGlide, "vacuglide-secret-token"); err != nil {
		t.Fatal(err)
	}
	if settings.AutoblowToken(DeviceAutoblow) != "ultra-secret-token" {
		t.Fatalf("token = %q, want it trimmed", settings.AutoblowToken(DeviceAutoblow))
	}
	public, err := json.Marshal(settings.Public())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(public), "secret-token") {
		t.Fatal("the public settings view carries a device token")
	}
	view := settings.Public()
	if !view.Autoblow.TokenSet || !view.VacuGlide.TokenSet {
		t.Fatalf("public view = %+v %+v, want both tokens reported as set", view.Autoblow, view.VacuGlide)
	}
}

func TestAutoblowTokensAreChecked(t *testing.T) {
	settings := DefaultSettings()
	if _, err := settings.WithAutoblowToken(DeviceVacuGlide, "has space"); !errors.Is(err, autoblow.ErrBadToken) {
		t.Fatalf("a token with a space = %v", err)
	}
	if _, err := settings.WithAutoblowToken("vacupump", "token"); err == nil {
		t.Fatal("an unknown Autoblow device was accepted")
	}
	settings.VacuGlide.DeviceToken = "bad\ntoken"
	if _, err := NormalizeSettings(settings); !errors.Is(err, autoblow.ErrBadToken) {
		t.Fatalf("a stored malformed token = %v", err)
	}
}

func TestAutoblowOptionsPatch(t *testing.T) {
	settings := DefaultSettings()
	settings.VacuGlide.DeviceToken = "vacuglide-token"
	offset, low, high, air := 120, 20, 70, true
	next, err := settings.ApplyDeviceOptionsPatch(DeviceOptionsPatch{VacuGlide: &VacuGlidePatch{TimingOffsetMillis: &offset, TempoMinPercent: &low, TempoMaxPercent: &high, MotionAir: &air}})
	if err != nil {
		t.Fatal(err)
	}
	if vg := next.VacuGlide; vg.TimingOffsetMillis != 120 || vg.TempoMinPercent != 20 || vg.TempoMaxPercent != 70 || !vg.MotionAir || vg.DeviceToken != "vacuglide-token" {
		t.Fatalf("patched = %+v", next.VacuGlide)
	}
	cleared, err := next.ApplyDeviceOptionsPatch(DeviceOptionsPatch{VacuGlide: &VacuGlidePatch{ClearToken: true}})
	if err != nil || cleared.VacuGlide.DeviceToken != "" || cleared.VacuGlide.TempoMaxPercent != 70 {
		t.Fatalf("clearing the token = %+v, %v", cleared.VacuGlide, err)
	}
	for _, bounds := range [][2]int{{4, 50}, {60, 50}, {20, 101}} {
		low, high := bounds[0], bounds[1]
		if _, err := settings.ApplyDeviceOptionsPatch(DeviceOptionsPatch{VacuGlide: &VacuGlidePatch{TempoMinPercent: &low, TempoMaxPercent: &high}}); err == nil {
			t.Fatalf("Tempo range %d..%d was accepted", low, high)
		}
	}
	tooFar := AutoblowMaxTimingOffsetMs + 1
	if _, err := settings.ApplyDeviceOptionsPatch(DeviceOptionsPatch{Autoblow: &AutoblowPatch{TimingOffsetMillis: &tooFar}}); err == nil {
		t.Fatal("an out-of-range offset was accepted")
	}
	if _, err := settings.ApplyDeviceOptionsPatch(DeviceOptionsPatch{Autoblow: &AutoblowPatch{}}); !errors.Is(err, errEmptyDeviceOptionsPatch) {
		t.Fatalf("an empty patch = %v", err)
	}
}

func TestAutoblowDevicesJoinTheRunClockOrder(t *testing.T) {
	named := "handy-0123456789ab"
	ordered := canonicalDevices([]string{DeviceVibrator, DeviceOutputCoyote, DeviceVacuGlide, named, DeviceAutoblow, DeviceOutputHandy})
	want := []string{DeviceOutputHandy, named, DeviceAutoblow, DeviceVacuGlide, DeviceOutputCoyote, DeviceVibrator}
	if !slices.Equal(ordered, want) {
		t.Fatalf("order = %v, want %v", ordered, want)
	}
	device := DefaultSettings().Device
	device, err := device.ChangeConnection(DeviceVacuGlide, "add")
	if err != nil {
		t.Fatal(err)
	}
	if device.Plays(DeviceVacuGlide) {
		t.Fatal("adding a device must not include it in playback")
	}
	device, err = device.ChangeConnection(DeviceVacuGlide, "include")
	if err != nil || !device.Plays(DeviceVacuGlide) {
		t.Fatalf("include = %v, plays %v", err, device.Plays(DeviceVacuGlide))
	}
	if device.Output != DeviceOutputHandy {
		t.Fatalf("legacy output = %q; the VacuGlide must not change it", device.Output)
	}
}

func TestVacuGlideSettingsFromBeforeTheTempoRange(t *testing.T) {
	// A store saved by alpha.85 has speed_match and stroke_follow but no
	// Tempo range; it loads on the full range with the air left alone.
	var saved VacuGlideSettings
	if err := json.Unmarshal([]byte(`{"timing_offset_ms":40,"speed_match":"handy_pace","stroke_follow":true}`), &saved); err != nil {
		t.Fatal(err)
	}
	loaded := applyMissingVacuGlideDefaults(saved)
	if loaded.TempoMinPercent != VacuGlideTempoFloorPercent || loaded.TempoMaxPercent != 100 || loaded.MotionAir || loaded.TimingOffsetMillis != 40 {
		t.Fatalf("loaded = %+v", loaded)
	}
}
