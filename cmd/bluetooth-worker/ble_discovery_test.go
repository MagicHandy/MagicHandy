//go:build windows || linux

package main

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/ble"
	"tinygo.org/x/bluetooth"
)

func TestDiscoveryAdmitsPrivateAdvertisementsWithoutCallingThemIdentity(t *testing.T) {
	coyote, _ := ble.LookupProfile(ble.ProfileCoyote)
	handy, _ := ble.LookupProfile(ble.ProfileHandy)
	for _, test := range []struct {
		name       string
		profile    ble.Profile
		advertised string
		length     int
		want       bool
	}{
		{"normal name", coyote, "47L121000", 0, true},
		{"private identifier", coyote, "", 9, true},
		{"unrelated named device", coyote, "other device", 9, false},
		{"too short", coyote, "", 5, false},
		{"too long", coyote, "", 15, false},
		{"Handy remains name based", handy, "", 9, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := []bluetooth.ManufacturerDataElement{{CompanyID: 0x1234, Data: make([]byte, test.length)}}
			if got := matchesDiscovery(test.profile, test.advertised, data); got != test.want {
				t.Fatalf("candidate=%t want %t", got, test.want)
			}
		})
	}
}
