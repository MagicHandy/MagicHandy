package ble

import "strings"

// Built-in profile IDs are stable worker protocol identifiers.
const (
	ProfileCoyote = "coyote3"
	ProfileHandy  = "handy4"
)

// Profile describes GATT discovery and framing bounds, never device commands.
// Profiles are allowlisted by ID rather than accepting arbitrary UUIDs over IPC.
type Profile struct {
	ID, Name                           string
	ServiceUUID, WriteUUID, NotifyUUID string
	BatteryServiceUUID, BatteryUUID    string
	MaxWriteBytes                      int
	ExactName                          bool
	NamePrefixes                       []string
	RememberedOnly                     bool
	StopOnDisconnect                   bool
	WriteWithoutResponseFallback       bool
	WriteWithoutResponseOnly           bool
}

// LookupProfile returns a fresh descriptor so callers cannot mutate a registry.
func LookupProfile(id string) (Profile, bool) {
	switch id {
	case ProfileCoyote:
		return Profile{ID: id, Name: "47L121000", ExactName: true, MaxWriteBytes: 64, WriteWithoutResponseOnly: true, RememberedOnly: true,
			ServiceUUID:        "0000180c-0000-1000-8000-00805f9b34fb",
			WriteUUID:          "0000150a-0000-1000-8000-00805f9b34fb",
			NotifyUUID:         "0000150b-0000-1000-8000-00805f9b34fb",
			BatteryServiceUUID: "0000180a-0000-1000-8000-00805f9b34fb",
			BatteryUUID:        "00001500-0000-1000-8000-00805f9b34fb"}, true
	case ProfileHandy:
		return Profile{ID: id, Name: "The Handy", MaxWriteBytes: 512,
			ServiceUUID:  "77834d26-40f7-11ee-be56-0242ac120002",
			WriteUUID:    "77835032-40f7-11ee-be56-0242ac120002",
			NotifyUUID:   "77835410-40f7-11ee-be56-0242ac120002",
			NamePrefixes: []string{"OHD", "Handy", "The Handy"}, RememberedOnly: true, StopOnDisconnect: true, WriteWithoutResponseFallback: true}, true
	default:
		return Profile{}, false
	}
}

// MatchesName filters advertisements; GATT discovery still verifies the device.
func (p Profile) MatchesName(name string) bool {
	if p.ExactName {
		return name == p.Name
	}
	for _, prefix := range p.NamePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
