//go:build windows || linux

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"

	"github.com/mapledaemon/MagicHandy/internal/ble"
	"github.com/mapledaemon/MagicHandy/internal/ble/bleworker"
)

// radioAdapter uses the platform radio through tinygo.org/x/bluetooth: WinRT
// on Windows and BlueZ over D-Bus on Linux, both without cgo.
type radioAdapter struct {
	adapter *bluetooth.Adapter

	mu   sync.Mutex
	lost map[string]func(error)
}

func newAdapter() bleworker.Adapter {
	return &radioAdapter{adapter: bluetooth.DefaultAdapter, lost: map[string]func(error){}}
}

func (a *radioAdapter) Enable() error {
	if err := a.adapter.Enable(); err != nil {
		return err
	}
	// The library reports link loss through one adapter-wide handler, which
	// must be registered before the first connection.
	a.adapter.SetConnectHandler(func(device bluetooth.Device, connected bool) {
		if connected {
			return
		}
		a.mu.Lock()
		handler := a.lost[normalizeAddress(device.Address.String())]
		a.mu.Unlock()
		if handler != nil {
			handler(errors.New("link_lost"))
		}
	})
	return nil
}

func (a *radioAdapter) ScanName(ctx context.Context, profile ble.Profile) (string, error) {
	var foundMu sync.Mutex
	found := make(map[string]struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- a.adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
			// COYOTE requires an exact normal-mode name. Handy firmware uses
			// several prefixes; collect the scan window to detect ambiguity.
			if !profile.MatchesName(result.LocalName()) {
				return
			}
			foundMu.Lock()
			found[normalizeAddress(result.Address.String())] = struct{}{}
			foundMu.Unlock()
			if profile.ExactName {
				_ = adapter.StopScan()
			}
		})
	}()
	select {
	case err := <-finished:
		if err != nil {
			return "", err
		}
	case <-ctx.Done():
		_ = a.adapter.StopScan()
		<-finished
	}
	// Cancellation (unlike the scan timeout) must never start a connection.
	if errors.Is(ctx.Err(), context.Canceled) {
		return "", ctx.Err()
	}
	foundMu.Lock()
	defer foundMu.Unlock()
	if len(found) > 1 {
		return "", bleworker.ErrAmbiguous
	}
	for address := range found {
		return address, nil
	}
	return "", bleworker.ErrNotFound
}

func (a *radioAdapter) ConnectAddress(ctx context.Context, address string, profile ble.Profile) (bleworker.Peripheral, error) {
	mac, err := bluetooth.ParseMAC(address)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", bleworker.ErrNotFound, err)
	}
	type outcome struct {
		device bluetooth.Device
		err    error
	}
	results := make(chan outcome, 1)
	go func() {
		device, err := a.adapter.Connect(bluetooth.Address{MACAddress: bluetooth.MACAddress{MAC: mac}}, bluetooth.ConnectionParams{})
		results <- outcome{device: device, err: err}
	}()
	select {
	case result := <-results:
		if result.err != nil {
			return nil, fmt.Errorf("%w: %v", bleworker.ErrNotFound, result.err)
		}
		return &radioPeripheral{adapter: a, device: result.device, address: normalizeAddress(address), profile: profile}, nil
	case <-ctx.Done():
		// The platform call cannot be canceled; release a late success.
		go func() {
			if result := <-results; result.err == nil {
				_ = result.device.Disconnect()
			}
		}()
		return nil, bleworker.ErrNotFound
	}
}

type radioPeripheral struct {
	adapter             *radioAdapter
	device              bluetooth.Device
	address             string
	profile             ble.Profile
	writeLimit          int
	withoutResponseOnly bool

	mu      sync.Mutex
	write   bluetooth.DeviceCharacteristic
	battery *bluetooth.DeviceCharacteristic
	lost    bool
	closed  bool
}

func (p *radioPeripheral) Open(handlers bleworker.Handlers) error {
	write, notify, err := p.discoverService()
	if err != nil {
		return err
	}
	p.write = write
	// WinRT exposes characteristic properties. Choose the supported mode before
	// writing; never retry an uncertain motion packet in a different write mode.
	if properties, ok := any(write).(interface{ Properties() uint32 }); ok && p.profile.WriteWithoutResponseFallback {
		flags := properties.Properties()
		if flags&0x08 == 0 {
			if flags&0x04 == 0 {
				return bleworker.ErrUnsupported
			}
			p.withoutResponseOnly = true
		}
	}
	p.battery = p.discoverBattery()
	p.adapter.mu.Lock()
	p.adapter.lost[p.address] = func(cause error) {
		p.mu.Lock()
		first := !p.lost && !p.closed
		p.lost = true
		p.mu.Unlock()
		if first {
			handlers.Lost(cause)
		}
	}
	p.adapter.mu.Unlock()
	if err := notify.EnableNotifications(func(payload []byte) {
		handlers.Notify(append([]byte(nil), payload...))
	}); err != nil {
		return err
	}
	// Query after notification setup has exercised GATT. Reserve the ATT header;
	// never assume that the firmware's 512-byte maximum was negotiated.
	p.writeLimit = 20
	if mtu, mtuErr := write.GetMTU(); mtuErr == nil && mtu > 3 {
		p.writeLimit = min(int(mtu)-3, p.profile.MaxWriteBytes)
	}
	return nil
}

// discoverService verifies both characteristics of the selected profile.
func (p *radioPeripheral) discoverService() (bluetooth.DeviceCharacteristic, bluetooth.DeviceCharacteristic, error) {
	var none bluetooth.DeviceCharacteristic
	service, _ := bluetooth.ParseUUID(p.profile.ServiceUUID)
	writeUUID, _ := bluetooth.ParseUUID(p.profile.WriteUUID)
	notifyUUID, _ := bluetooth.ParseUUID(p.profile.NotifyUUID)
	services, err := p.device.DiscoverServices([]bluetooth.UUID{service})
	if err != nil || len(services) == 0 {
		return none, none, fmt.Errorf("%w: %v", bleworker.ErrUnsupported, err)
	}
	characteristics, err := services[0].DiscoverCharacteristics([]bluetooth.UUID{writeUUID, notifyUUID})
	if err != nil {
		return none, none, fmt.Errorf("%w: %v", bleworker.ErrUnsupported, err)
	}
	var write, notify *bluetooth.DeviceCharacteristic
	for index := range characteristics {
		switch characteristics[index].UUID() {
		case writeUUID:
			write = &characteristics[index]
		case notifyUUID:
			notify = &characteristics[index]
		}
	}
	if write == nil || notify == nil {
		return none, none, bleworker.ErrUnsupported
	}
	return *write, *notify, nil
}

func (p *radioPeripheral) discoverBattery() *bluetooth.DeviceCharacteristic {
	if p.profile.BatteryUUID == "" {
		return nil
	}
	service, _ := bluetooth.ParseUUID(p.profile.BatteryServiceUUID)
	level, _ := bluetooth.ParseUUID(p.profile.BatteryUUID)
	services, err := p.device.DiscoverServices([]bluetooth.UUID{service})
	if err != nil || len(services) == 0 {
		return nil
	}
	characteristics, err := services[0].DiscoverCharacteristics([]bluetooth.UUID{level})
	if err != nil || len(characteristics) == 0 {
		return nil
	}
	return &characteristics[0]
}

func (p *radioPeripheral) Write(payload []byte, withResponse bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.lost || p.closed {
		return errors.New("not connected")
	}
	var err error
	var count int
	if withResponse && !p.withoutResponseOnly {
		count, err = p.write.Write(payload)
	} else {
		count, err = p.write.WriteWithoutResponse(payload)
		if p.withoutResponseOnly && err == nil {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if err == nil && count != len(payload) {
		return io.ErrShortWrite
	}
	return err
}

func (p *radioPeripheral) ReadBattery() (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.battery == nil {
		return 0, errors.New("no battery characteristic")
	}
	buffer := make([]byte, 1)
	count, err := p.battery.Read(buffer)
	if err != nil || count < 1 {
		return 0, fmt.Errorf("battery read failed: %v", err)
	}
	return int(buffer[0]), nil
}

func (p *radioPeripheral) Address() string { return p.address }

func (p *radioPeripheral) Name() string { return p.profile.Name }

func (p *radioPeripheral) MaxWriteBytes() int { return p.writeLimit }

// Close disconnects once. On Windows the library already released a device
// whose link was lost, so a second release is skipped.
func (p *radioPeripheral) Close() error {
	p.adapter.mu.Lock()
	delete(p.adapter.lost, p.address)
	p.adapter.mu.Unlock()
	p.mu.Lock()
	alreadyGone := p.lost || p.closed
	p.closed = true
	p.mu.Unlock()
	if alreadyGone {
		return nil
	}
	return p.device.Disconnect()
}

func normalizeAddress(address string) string {
	return strings.ToUpper(strings.TrimSpace(address))
}
