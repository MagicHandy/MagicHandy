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
