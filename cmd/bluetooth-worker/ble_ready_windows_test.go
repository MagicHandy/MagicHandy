package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
	"tinygo.org/x/bluetooth"
)

func TestConnectionReadinessCannotPassOnObjectCreationAlone(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	checks := 0
	err := awaitConnected(ctx, func() (bool, error) { checks++; return checks == 3, nil })
	if err != nil || checks != 3 {
		t.Fatalf("connection admitted early: checks=%d err=%v", checks, err)
	}
}

type readinessScanner struct {
	started   chan struct{}
	stopped   chan struct{}
	once      sync.Once
	addresses []string
}

func (s *readinessScanner) Scan(callback func(*bluetooth.Adapter, bluetooth.ScanResult)) error {
	close(s.started)
	for _, address := range s.addresses {
		mac, _ := bluetooth.ParseMAC(address)
		callback(nil, bluetooth.ScanResult{Address: bluetooth.Address{MACAddress: bluetooth.MACAddress{MAC: mac}}})
	}
	<-s.stopped
	return nil
}
func (s *readinessScanner) StopScan() error { s.once.Do(func() { close(s.stopped) }); return nil }

func TestConnectionScanRequiresSelectedAddressAndHonorsCancellation(t *testing.T) {
	const selected = "AA:BB:CC:DD:EE:01"
	scan := &readinessScanner{started: make(chan struct{}), stopped: make(chan struct{}), addresses: []string{"AA:BB:CC:DD:EE:02"}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- prepareNativeConnection(ctx, scan, selected) }()
	<-scan.started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("unrelated advertisement admitted a connection: %v", err)
	}
	scan = &readinessScanner{started: make(chan struct{}), stopped: make(chan struct{}), addresses: []string{selected}}
	if err := prepareNativeConnection(context.Background(), scan, selected); err != nil {
		t.Fatal(err)
	}
}

func TestConnectionReadinessCancellationAndFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	checks := 0
	err := awaitConnected(ctx, func() (bool, error) { checks++; cancel(); return false, nil })
	if !errors.Is(err, context.Canceled) || checks != 1 {
		t.Fatalf("canceled connection continued: checks=%d err=%v", checks, err)
	}
	failure := errors.New("native state unavailable")
	if err := awaitConnected(context.Background(), func() (bool, error) { return false, failure }); !errors.Is(err, failure) {
		t.Fatalf("native failure hidden: %v", err)
	}
}
