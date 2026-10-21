package transport

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
)

// The session's device inventory: discovery, the incarnation each device
// gets when it appears, and the per-device claims that keep two owners off
// one device.

// Devices returns the current safe discovered-device snapshot.
func (s *intifaceSession) Devices() []IntifaceDevice {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneIntifaceDevicesLocked(s.devices)
}

// StartScanning asks Intiface to begin device discovery.
func (s *intifaceSession) StartScanning(ctx context.Context) error {
	if err := s.requestOK(ctx, "StartScanning", nil); err != nil {
		return err
	}
	s.mu.Lock()
	if s.closed || !s.connected {
		s.mu.Unlock()
		return errors.New("Intiface owner closed while scanning started")
	}
	s.mu.Unlock()
	return nil
}

// StopScanning asks Intiface to end device discovery.
func (s *intifaceSession) StopScanning(ctx context.Context) error {
	return s.requestOK(ctx, "StopScanning", nil)
}

func (s *intifaceSession) handleDeviceAdded(payload json.RawMessage) {
	var device buttplugDevice
	if json.Unmarshal(payload, &device) != nil {
		return
	}
	s.mu.Lock()
	s.devices[device.DeviceIndex] = intifaceDeviceFromProtocol(device)
	s.incarnations[device.DeviceIndex] = s.nextIncarnationLocked()
	s.mu.Unlock()
}

func (s *intifaceSession) nextIncarnationLocked() uint64 {
	s.lastIncarnation++
	return s.lastIncarnation
}

// incarnationLocked reports the current incarnation of a present device.
func (s *intifaceSession) incarnationLocked(deviceIndex uint32) (uint64, bool) {
	if _, ok := s.devices[deviceIndex]; !ok {
		return 0, false
	}
	incarnation, ok := s.incarnations[deviceIndex]
	return incarnation, ok
}

// claimLocked gives owner a device. A device claimed by another owner is
// refused; an owner's own earlier claims stay until it releases them.
func (s *intifaceSession) claimLocked(owner intifaceSessionOwner, deviceIndex uint32) error {
	if holder, ok := s.claims[deviceIndex]; ok && holder != owner {
		return errIntifaceDeviceClaimed
	}
	s.claims[deviceIndex] = owner
	return nil
}

// releaseClaimLocked returns a device owner holds; other claims are kept.
func (s *intifaceSession) releaseClaimLocked(owner intifaceSessionOwner, deviceIndex uint32) {
	if s.claims[deviceIndex] == owner {
		delete(s.claims, deviceIndex)
	}
}

func (s *intifaceSession) handleDeviceRemoved(payload json.RawMessage) {
	var removed buttplugDeviceRemoved
	if json.Unmarshal(payload, &removed) != nil {
		return
	}
	s.mu.Lock()
	delete(s.devices, removed.DeviceIndex)
	delete(s.incarnations, removed.DeviceIndex)
	delete(s.claims, removed.DeviceIndex)
	var followUps []func()
	for _, owner := range s.owners {
		if followUp := owner.deviceRemovedLocked(removed.DeviceIndex); followUp != nil {
			followUps = append(followUps, followUp)
		}
	}
	s.mu.Unlock()
	for _, followUp := range followUps {
		followUp()
	}
}

func cloneIntifaceDevicesLocked(devices map[uint32]IntifaceDevice) []IntifaceDevice {
	clones := make([]IntifaceDevice, 0, len(devices))
	for _, device := range devices {
		clone := device
		clone.LinearActuators = append([]IntifaceLinearActuator{}, device.LinearActuators...)
		clone.ScalarActuators = append([]IntifaceScalarActuator{}, device.ScalarActuators...)
		clones = append(clones, clone)
	}
	sort.Slice(clones, func(a, b int) bool { return clones[a].DeviceIndex < clones[b].DeviceIndex })
	return clones
}
