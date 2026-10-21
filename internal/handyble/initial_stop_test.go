package handyble

import (
	"context"
	"errors"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/ble"
)

// Firmware 4.3 rejects HSP Stop when the physical buttons or another client
// left the device in a different mode. Connection must stop that mode without
// starting a stream or relying on HSP being initialized already.
type otherModeLink struct {
	*SimulatedLink
	lastWill []byte
}

func (l *otherModeLink) Write(ctx context.Context, payload []byte, ack bool) error {
	outer, err := parseFields(payload)
	if err != nil {
		return err
	}
	request, err := parseFields(outer.bytes(2))
	if err != nil {
		return err
	}
	for _, method := range []Method{Stop, Setup, Add, Play} {
		if request.bytes(uint32(method)) != nil {
			return errors.New("wrong mode or unexpected motion command before playback")
		}
	}
	return l.SimulatedLink.Write(ctx, payload, ack)
}

func (l *otherModeLink) SetLastWill(payload []byte) error {
	l.lastWill = append([]byte(nil), payload...)
	return nil
}

func TestInitializeAndLastWillStopWhateverModeIsActive(t *testing.T) {
	connector := NewSimulatedConnector()
	if _, err := connector.Connect(t.Context(), ble.ConnectRequest{}); err != nil {
		t.Fatal(err)
	}
	link := &otherModeLink{SimulatedLink: connector.LastLink()}
	session, err := NewSession(link)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	if err := session.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, initial, _ := requestFields(t, link.Writes()[0])
	_, lastWill, _ := requestFields(t, link.lastWill)
	if initial != StopCurrentMode || lastWill != StopCurrentMode || !session.Snapshot().Ready {
		t.Fatalf("mode-independent Stop missing: initial=%d lastWill=%d status=%+v", initial, lastWill, session.Snapshot())
	}
}
