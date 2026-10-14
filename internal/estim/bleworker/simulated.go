package bleworker

import (
	"context"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/estim"
)

const simulatedAddress = "00:00:00:00:00:00"

// SimulatedAdapter models a Coyote behind the worker protocol. It backs the
// worker's -simulate flag and the process-level tests; no radio is used.
type SimulatedAdapter struct {
	Connector *estim.SimulatedConnector
}

// NewSimulatedAdapter returns an adapter over a fresh simulated connector.
func NewSimulatedAdapter() *SimulatedAdapter {
	return &SimulatedAdapter{Connector: estim.NewSimulatedConnector()}
}

// Enable always succeeds.
func (a *SimulatedAdapter) Enable() error {
	return nil
}

// ScanName finds the simulated device only by its exact Coyote name.
func (a *SimulatedAdapter) ScanName(ctx context.Context, name string) (string, error) {
	if name != estim.DeviceName {
		<-ctx.Done()
		return "", ErrNotFound
	}
	return simulatedAddress, nil
}

// ConnectAddress connects to the simulated device.
func (a *SimulatedAdapter) ConnectAddress(ctx context.Context, address string) (Peripheral, error) {
	if address != simulatedAddress {
		return nil, ErrNotFound
	}
	link, err := a.Connector.Connect(ctx, estim.ConnectRequest{})
	if err != nil {
		return nil, err
	}
	return &simulatedPeripheral{link: link, done: make(chan struct{})}, nil
}

type simulatedPeripheral struct {
	link estim.Link
	once sync.Once
	done chan struct{}
}

func (p *simulatedPeripheral) Open(handlers Handlers) error {
	go func() {
		for {
			select {
			case <-p.done:
				return
			case event := <-p.link.Events():
				switch event.Kind {
				case estim.LinkNotification:
					handlers.Notify(event.Payload)
				case estim.LinkBattery:
					handlers.Battery(event.Battery)
				case estim.LinkLost:
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

func (p *simulatedPeripheral) Name() string { return estim.DeviceName }

func (p *simulatedPeripheral) Close() error {
	p.once.Do(func() { close(p.done) })
	return p.link.Close()
}
