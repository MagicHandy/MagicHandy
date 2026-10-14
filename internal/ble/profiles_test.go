package ble_test

import (
	"github.com/mapledaemon/MagicHandy/internal/ble"
	"github.com/mapledaemon/MagicHandy/internal/estim"
	"testing"
)

func TestProfilesPreserveCoyoteIdentityAndRejectRecoveryMode(t *testing.T) {
	coyote, ok := ble.LookupProfile(ble.ProfileCoyote)
	if !ok || coyote.ServiceUUID != estim.ServiceUUID || coyote.WriteUUID != estim.WriteCharacteristicUUID || coyote.NotifyUUID != estim.NotifyCharacteristicUUID || coyote.BatteryServiceUUID != estim.BatteryServiceUUID || coyote.BatteryUUID != estim.BatteryCharacteristicUUID {
		t.Fatal("Coyote GATT profile drifted from its protocol")
	}
	if !coyote.MatchesName(estim.DeviceName) || coyote.MatchesName(estim.DeviceName+"_O3") {
		t.Fatal("recovery mode matched normal Coyote")
	}
	handy, ok := ble.LookupProfile(ble.ProfileHandy)
	if !ok || !handy.MatchesName("OHD1234") || !handy.MatchesName("Handy1234") || handy.MatchesName("random device") || !handy.RememberedOnly {
		t.Fatal("unsafe Handy discovery")
	}
	if _, ok := ble.LookupProfile("arbitrary-uuid"); ok {
		t.Fatal("unregistered profile was accepted")
	}
}
