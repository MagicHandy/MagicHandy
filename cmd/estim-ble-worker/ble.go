//go:build windows || linux

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"tinygo.org/x/bluetooth"

	"github.com/mapledaemon/MagicHandy/internal/estim"
	"github.com/mapledaemon/MagicHandy/internal/estim/bleworker"
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

func (a *radioAdapter) ScanName(ctx context.Context, name string) (string, error) {
	found := make(chan string, 1)
	finished := make(chan error, 1)
	go func() {
		finished <- a.adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
			// Exact match: the recovery-mode name has a suffix and no pulse service.
			if result.LocalName() != name {
				return
			}
			select {
			case found <- result.Address.String():
				_ = adapter.StopScan()
			default:
			}
		})
	}()
	select {
	case address := <-found:
		<-finished
		return normalizeAddress(address), nil
	case err := <-finished:
		if err != nil {
			return "", err
		}
		return "", bleworker.ErrNotFound
	case <-ctx.Done():
		_ = a.adapter.StopScan()
		<-finished
		return "", bleworker.ErrNotFound
	}
}

func (a *radioAdapter) ConnectAddress(ctx context.Context, address string) (bleworker.Peripheral, error) {
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
		return &radioPeripheral{adapter: a, device: result.device, address: normalizeAddress(address)}, nil
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
	adapter *radioAdapter
	device  bluetooth.Device
	address string

	mu      sync.Mutex
	write   bluetooth.DeviceCharacteristic
	battery *bluetooth.DeviceCharacteristic
	lost    bool
	closed  bool
}

func (p *radioPeripheral) Open(handlers bleworker.Handlers) error {
	write, notify, err := p.discoverPulse()
	if err != nil {
		return err
	}
	p.write = write
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
	return notify.EnableNotifications(func(payload []byte) {
		handlers.Notify(append([]byte(nil), payload...))
	})
}

// discoverPulse finds the documented service and both characteristics. A
// device without them is not a Coyote 3.0 (or is in recovery mode).
func (p *radioPeripheral) discoverPulse() (bluetooth.DeviceCharacteristic, bluetooth.DeviceCharacteristic, error) {
	var none bluetooth.DeviceCharacteristic
	service, _ := bluetooth.ParseUUID(estim.ServiceUUID)
	writeUUID, _ := bluetooth.ParseUUID(estim.WriteCharacteristicUUID)
	notifyUUID, _ := bluetooth.ParseUUID(estim.NotifyCharacteristicUUID)
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
	service, _ := bluetooth.ParseUUID(estim.BatteryServiceUUID)
	level, _ := bluetooth.ParseUUID(estim.BatteryCharacteristicUUID)
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
	if withResponse {
		_, err = p.write.Write(payload)
	} else {
		_, err = p.write.WriteWithoutResponse(payload)
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

func (p *radioPeripheral) Name() string { return estim.DeviceName }

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
