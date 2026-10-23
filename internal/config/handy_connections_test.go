package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestNamedHandyInventoryPersistenceAndPrivacy(t *testing.T) {
	s, err := NormalizeSettings(DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	id, name, key := "handy-010203040506", "Partner's Handy", "synthetic-private-handy-key"
	next, err := s.AddHandyConnection(id, HandyConnectionPatch{Name: &name, ConnectionKey: &key})
	if err != nil {
		t.Fatal(err)
	}
	if next.Device.Plays(id) || len(s.Device.HandyConnections) != 0 {
		t.Fatal("adding changed playback or its input")
	}
	next.Device, err = next.Device.ChangeConnection(id, "include")
	if err != nil {
		t.Fatal(err)
	}
	next.Device.HandyConnections[0].NativeBluetoothAddress = "AA:BB:CC:DD:EE:FA"
	data, _ := json.Marshal(next)
	reloaded, _, err := loadSettingsFromBytes(data)
	if err != nil || !reflect.DeepEqual(next.Device, reloaded.Device) {
		t.Fatalf("round trip: %v", err)
	}
	public, _ := json.Marshal(reloaded.Public())
	if strings.Contains(string(public), key) || strings.Contains(string(public), "AA:BB") {
		t.Fatal("private Handy identity leaked")
	}
	removed, err := next.RemoveHandyConnection(id)
	if err != nil {
		t.Fatal(err)
	}
	if removed.Device.HasDevice(id) || removed.Device.Plays(id) || !next.Device.Plays(id) || len(next.Device.HandyConnections) != 1 {
		t.Fatal("remove mutated input or retained device")
	}
}

func TestNamedHandyRejectsDuplicatesAndLimits(t *testing.T) {
	s, _ := NormalizeSettings(DefaultSettings())
	key := "synthetic-unique-key"
	s.Device.HandyConnectionKey = key
	if _, err := s.AddHandyConnection("handy-010203040506", HandyConnectionPatch{ConnectionKey: &key}); err == nil {
		t.Fatal("duplicate key accepted")
	}
	browser := DispatchOwnerBrowserBluetooth
	if _, err := s.AddHandyConnection("handy-010203040506", HandyConnectionPatch{Method: &browser}); err == nil {
		t.Fatal("second browser bridge accepted")
	}
	for n := 1; n < MaxHandys; n++ {
		var err error
		s, err = s.AddHandyConnection(fmt.Sprintf("handy-%012x", n), HandyConnectionPatch{})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.AddHandyConnection("handy-ffffffffffff", HandyConnectionPatch{}); err == nil {
		t.Fatal("inventory bound not enforced")
	}
	s.Device.HandyConnections[0].NativeBluetoothAddress = "AA:BB"
	s.Device.HandyConnections[1].NativeBluetoothAddress = "aa:bb"
	if _, err := NormalizeSettings(s); err == nil {
		t.Fatal("duplicate native device accepted")
	}
}

func TestNamedHandySharedCalibrationOnlyIncludesParticipants(t *testing.T) {
	s, _ := NormalizeSettings(DefaultSettings())
	s.Motion.HandyModel = HandyModelOriginal
	id, model := "handy-010203040506", HandyModel2Standard
	s, err := s.AddHandyConnection(id, HandyConnectionPatch{HandyModel: &model})
	if err != nil {
		t.Fatal(err)
	}
	if s.EffectiveMotion().HandyModel != HandyModelOriginal {
		t.Fatal("excluded device changed pace")
	}
	s.Device, _ = s.Device.ChangeConnection(id, "include")
	if s.EffectiveMotion().HandyModel != HandyModel2Standard {
		t.Fatal("slowest normalized profile not selected")
	}
	if s.Motion.HandyModel != HandyModelOriginal {
		t.Fatal("effective policy changed stored primary model")
	}
	s.Device, _ = s.Device.ChangeConnection(id, "exclude")
	if s.EffectiveMotion() != s.Motion {
		t.Fatal("single device behavior changed")
	}
}

func TestNamedHandyDefaultNamesRemainDistinctAfterRemoval(t *testing.T) {
	s, _ := NormalizeSettings(DefaultSettings())
	s, _ = s.AddHandyConnection("handy-010203040506", HandyConnectionPatch{})
	s, _ = s.AddHandyConnection("handy-010203040507", HandyConnectionPatch{})
	s, _ = s.RemoveHandyConnection("handy-010203040506")
	s, err := s.AddHandyConnection("handy-010203040508", HandyConnectionPatch{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Device.HandyConnections[0].Name == s.Device.HandyConnections[1].Name {
		t.Fatal("adding reused a name still in the inventory")
	}
}
