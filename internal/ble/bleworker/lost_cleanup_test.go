package bleworker

import (
	"errors"
	"io"
	"testing"
)

type disposalOnlyPeripheral struct {
	Peripheral // Any attempted write to the lost link would panic in this test.
	closes     int
}

func (p *disposalOnlyPeripheral) Close() error { p.closes++; return nil }

func TestLostPeripheralStillClosesOnEveryTeardownPath(t *testing.T) {
	for _, kind := range []string{"disconnect", "shutdown", "wait"} {
		t.Run(kind, func(t *testing.T) {
			p := &disposalOnlyPeripheral{}
			s := &server{peripheral: p, cancel: func() {}, out: &lineWriter{out: io.Discard}, lastWill: []byte{1}}
			s.profile.StopOnDisconnect = true
			handlers := s.handlers(p)
			handlers.Lost(errors.New("radio lost"))
			handlers.Lost(errors.New("duplicate event"))
			if s.peripheral != nil || p.closes != 0 {
				t.Fatal("loss must fence writes immediately without disposing inside the native callback")
			}
			switch kind {
			case "disconnect":
				s.disconnect()
			case "shutdown":
				s.shutdown(true)
			case "wait":
				s.wait()
			}
			s.wait()
			if p.closes != 1 || s.retired != nil {
				t.Fatalf("native disposal count = %d", p.closes)
			}
		})
	}
}
