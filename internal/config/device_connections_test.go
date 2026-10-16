package config

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestDeviceInventoryMigratesLegacyOutputAndPreservesEmpty(t *testing.T) {
	for _, output := range []string{DeviceOutputHandy, DeviceOutputCoyote, DeviceOutputHandyCoyote, DeviceOutputNone} {
		s := DefaultSettings()
		s.Device.Output = output
		next, err := NormalizeSettings(s)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(next.Device.ConfiguredDevices, configuredFromOutput(output)) {
			t.Fatalf("migration: %+v", next.Device)
		}
		if next.Device.LinkedFailurePolicy != LinkedFailureStopAll {
			t.Fatal("unsafe policy default")
		}
		data, err := json.Marshal(next)
		if err != nil {
			t.Fatal(err)
		}
		reloaded, _, err := loadSettingsFromBytes(data)
		if err != nil || !reflect.DeepEqual(next.Device, reloaded.Device) {
			t.Fatalf("round trip: %+v, %v", reloaded.Device, err)
		}
	}
}

func TestDeviceInventoryEditsDoNotImplicitlySelectOrLoseConnections(t *testing.T) {
	s, err := NormalizeSettings(DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.Device.ChangeConnection(DeviceOutputCoyote, "add")
	if err != nil || d.Output != DeviceOutputHandy || !d.HasDevice(DeviceOutputCoyote) {
		t.Fatalf("add: %+v %v", d, err)
	}
	if s.Device.HasDevice(DeviceOutputCoyote) {
		t.Fatal("edit mutated its input")
	}
	d, _ = d.ChangeConnection(DeviceOutputCoyote, "include")
	if d.Output != DeviceOutputHandyCoyote {
		t.Fatal("include did not select both")
	}
	d, _ = d.ChangeConnection(DeviceOutputHandy, "exclude")
	if d.Output != DeviceOutputCoyote || !d.HasDevice(DeviceOutputHandy) {
		t.Fatal("exclude removed the connection")
	}
	d, _ = d.ChangeConnection(DeviceOutputCoyote, "remove")
	d, _ = d.ChangeConnection(DeviceOutputHandy, "remove")
	if d.Output != DeviceOutputNone || d.ConfiguredDevices == nil || len(d.ConfiguredDevices) != 0 {
		t.Fatalf("empty: %+v", d)
	}
	s.Device = d
	next, err := NormalizeSettings(cloneSettings(s))
	if err != nil || next.Device.Output != DeviceOutputNone || len(next.Device.ConfiguredDevices) != 0 {
		t.Fatalf("empty normalization: %+v, %v", next.Device, err)
	}
	if _, err := d.ChangeConnection(DeviceOutputHandy, "include"); err == nil {
		t.Fatal("included an unconfigured device")
	}
}

func TestDeviceInventoryRejectsInvalidStateAndPolicy(t *testing.T) {
	for _, change := range []func(*DeviceSettings){
		func(d *DeviceSettings) { d.ConfiguredDevices = []string{"handy", "handy"} },
		func(d *DeviceSettings) { d.ConfiguredDevices = []string{"unknown"} },
		func(d *DeviceSettings) { d.ConfiguredDevices = []string{} },
		func(d *DeviceSettings) { d.LinkedFailurePolicy = "ignore" },
	} {
		s := DefaultSettings()
		change(&s.Device)
		if _, err := NormalizeSettings(s); err == nil {
			t.Fatalf("accepted %+v", s.Device)
		}
	}
}

func TestDefaultPublicDeviceInventoryMatchesPlayback(t *testing.T) {
	public := DefaultSettings().Public()
	if !reflect.DeepEqual(public.Device.ConfiguredDevices, []string{DeviceOutputHandy}) {
		t.Fatalf("default public inventory: %+v", public.Device)
	}
}

func TestPlaybackSetMigratesFromOutputAndStaysAuthoritative(t *testing.T) {
	s := DefaultSettings()
	s.Device.Output = DeviceOutputHandyCoyote
	next, err := NormalizeSettings(s)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(next.Device.PlaybackDevices, []string{DeviceOutputHandy, DeviceOutputCoyote}) {
		t.Fatalf("migrated playback = %v", next.Device.PlaybackDevices)
	}
	if !next.Device.Plays(DeviceOutputCoyote) || !reflect.DeepEqual(next.Public().Device.PlaybackDevices, next.Device.PlaybackDevices) {
		t.Fatalf("public playback = %+v", next.Public().Device)
	}
	// A stored set is reordered into run-clock order; the output must agree.
	next.Device.PlaybackDevices = []string{DeviceOutputCoyote, DeviceOutputHandy}
	if reordered, err := NormalizeSettings(cloneSettings(next)); err != nil || reordered.Device.PlaybackDevices[0] != DeviceOutputHandy {
		t.Fatalf("reordered = %v, %v", reordered.Device.PlaybackDevices, err)
	}
	next.Device.Output = DeviceOutputCoyote
	if _, err := NormalizeSettings(cloneSettings(next)); err == nil {
		t.Fatal("accepted an output that disagrees with the playback set")
	}
}

func TestPlaybackSetUpdatesRejectConflictsAndKeepEmpty(t *testing.T) {
	s, err := NormalizeSettings(DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	s.Device, _ = s.Device.ChangeConnection(DeviceOutputCoyote, "add")
	update := settingsUpdateFrom(t, s)
	update.Device.Output = ""
	empty := []string{}
	update.Device.PlaybackDevices = &empty
	next, err := s.ApplyUpdate(update)
	if err != nil || next.Device.PlaybackDevices == nil || len(next.Device.PlaybackDevices) != 0 || next.Device.Output != DeviceOutputNone {
		t.Fatalf("empty set = %+v, %v", next.Device, err)
	}
	both := []string{DeviceOutputCoyote, DeviceOutputHandy}
	update.Device.PlaybackDevices = &both
	update.Device.Output = DeviceOutputHandy
	if _, err := s.ApplyUpdate(update); err == nil {
		t.Fatal("accepted conflicting output and playback devices")
	}
	update.Device.Output = DeviceOutputHandyCoyote
	if next, err = s.ApplyUpdate(update); err != nil || next.Device.Output != DeviceOutputHandyCoyote {
		t.Fatalf("agreeing write = %+v, %v", next.Device, err)
	}
	unconfigured := []string{DeviceOutputHandy}
	s.Device, _ = s.Device.ChangeConnection(DeviceOutputHandy, "remove")
	update.Device.Output = ""
	update.Device.PlaybackDevices = &unconfigured
	next, err = s.ApplyUpdate(update)
	if err == nil {
		_, err = NormalizeSettings(next)
	}
	if err == nil {
		t.Fatal("accepted a playback device missing from the inventory")
	}
}

func TestConnectionEditsKeepOutputAsTheLegacyProjection(t *testing.T) {
	s, err := NormalizeSettings(DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	d, _ := s.Device.ChangeConnection(DeviceOutputCoyote, "add")
	d, _ = d.ChangeConnection(DeviceOutputCoyote, "include")
	d, _ = d.ChangeConnection(DeviceOutputHandy, "exclude")
	if !reflect.DeepEqual(d.PlaybackDevices, []string{DeviceOutputCoyote}) || d.Output != DeviceOutputCoyote {
		t.Fatalf("after edits: %+v", d)
	}
	d = d.WithLegacyOutput(DeviceOutputHandyCoyote)
	if !reflect.DeepEqual(d.PlaybackDevices, []string{DeviceOutputHandy, DeviceOutputCoyote}) {
		t.Fatalf("legacy write: %+v", d)
	}
}
