package bleworker

import (
	"context"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/ble"
)

const simulatedAddress = "00:00:00:00:00:00"

// SimulatedAdapter models named device profiles behind the worker protocol. It backs the
// worker's -simulate flag and the process-level tests; no radio is used.
type SimulatedAdapter struct {
	Connectors map[string]ble.Connector
}

// NewSimulatedAdapter returns an adapter over a fresh simulated connector.
func NewSimulatedAdapter(connectors map[string]ble.Connector) *SimulatedAdapter {
	return &SimulatedAdapter{Connectors: connectors}
}

// Enable always succeeds.
func (a *SimulatedAdapter) Enable() error {
	return nil
}

// ScanName finds only profiles supplied by the test host.
func (a *SimulatedAdapter) ScanName(ctx context.Context, profile ble.Profile) (string, error) {
	if a.Connectors[profile.ID] == nil {
		<-ctx.Done()
		return "", ErrNotFound
	}
	return simulatedAddress, nil
}

// ConnectAddress connects to the simulated device.
func (a *SimulatedAdapter) ConnectAddress(ctx context.Context, address string, profile ble.Profile) (Peripheral, error) {
	if address != simulatedAddress || a.Connectors[profile.ID] == nil {
		return nil, ErrNotFound
	}
	link, err := a.Connectors[profile.ID].Connect(ctx, ble.ConnectRequest{})
	if err != nil {
		return nil, err
	}
	return &simulatedPeripheral{link: link, name: profile.Name, limit: profile.MaxWriteBytes, done: make(chan struct{})}, nil
}

type simulatedPeripheral struct {
	link  ble.Link
	name  string
	limit int
	once  sync.Once
	done  chan struct{}
}

func (p *simulatedPeripheral) Open(handlers Handlers) error {
	go func() {
		for {
			select {
			case <-p.done:
				return
			case event, ok := <-p.link.Events():
				if !ok {
					return
				}
				switch event.Kind {
				case ble.LinkNotification:
					handlers.Notify(event.Payload)
				case ble.LinkBattery:
					handlers.Battery(event.Battery)
				case ble.LinkLost:
					handlers.Lost(event.Err)
					return
				}
			}
		}
	}()
	return nil
}

func (p *simulatedPeripheral) Write(payload []byte, _ bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return p.link.Write(ctx, payload, true)
}

func (p *simulatedPeripheral) ReadBattery() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return p.link.ReadBattery(ctx)
}

func (p *simulatedPeripheral) Address() string { return simulatedAddress }

func (p *simulatedPeripheral) Name() string { return p.name }

func (p *simulatedPeripheral) MaxWriteBytes() int { return p.limit }

func (p *simulatedPeripheral) Close() error {
	p.once.Do(func() { close(p.done) })
	return p.link.Close()
}
