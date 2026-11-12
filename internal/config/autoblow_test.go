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
	if settings.VacuGlide.SpeedMatch != VacuGlideSpeedHandyPace {
		t.Fatalf("default speed match = %q", settings.VacuGlide.SpeedMatch)
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
	offset, follow, full := 120, true, VacuGlideSpeedFullRange
	next, err := settings.ApplyDeviceOptionsPatch(DeviceOptionsPatch{VacuGlide: &VacuGlidePatch{TimingOffsetMillis: &offset, SpeedMatch: &full, StrokeFollow: &follow}})
	if err != nil {
		t.Fatal(err)
	}
	if next.VacuGlide.TimingOffsetMillis != 120 || next.VacuGlide.SpeedMatch != VacuGlideSpeedFullRange || !next.VacuGlide.StrokeFollow || next.VacuGlide.DeviceToken != "vacuglide-token" {
		t.Fatalf("patched = %+v", next.VacuGlide)
	}
	cleared, err := next.ApplyDeviceOptionsPatch(DeviceOptionsPatch{VacuGlide: &VacuGlidePatch{ClearToken: true}})
	if err != nil || cleared.VacuGlide.DeviceToken != "" || cleared.VacuGlide.SpeedMatch != VacuGlideSpeedFullRange {
		t.Fatalf("clearing the token = %+v, %v", cleared.VacuGlide, err)
	}
	unknown := "fastest"
	if _, err := settings.ApplyDeviceOptionsPatch(DeviceOptionsPatch{VacuGlide: &VacuGlidePatch{SpeedMatch: &unknown}}); err == nil {
		t.Fatal("an unknown speed match was accepted")
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
