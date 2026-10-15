package transport

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/ble"
	"github.com/mapledaemon/MagicHandy/internal/estim"
)

type unacknowledgedCoyoteConnector struct{ ble.Connector }
type unacknowledgedCoyoteLink struct{ ble.Link }

func (c unacknowledgedCoyoteConnector) Connect(ctx context.Context, request ble.ConnectRequest) (ble.Link, error) {
	link, err := c.Connector.Connect(ctx, request)
	if err != nil {
		return nil, err
	}
	return unacknowledgedCoyoteLink{link}, nil
}

func (l unacknowledgedCoyoteLink) Device() ble.DeviceInfo {
	device := l.Link.Device()
	device.UnacknowledgedWrites = true
	return device
}

func TestCoyoteUnacknowledgedZeroNeedsDeviceReport(t *testing.T) {
	f := newCoyoteFixture(t)
	f.owner.connector = unacknowledgedCoyoteConnector{f.connector}
	link := f.connect() // A device already at zero may send no B1 at all.
	f.clock.Advance(2 * time.Second)
	f.owner.paceOnce()
	status := f.owner.Status()
	if status.StrengthKnown || status.StrengthSource != estim.StrengthUnknown {
		t.Fatalf("queued zero was mistaken for acknowledged delivery: %+v", status)
	}
	if _, err := f.owner.MotionTransport(); !errors.Is(err, errCoyoteNotReady) {
		t.Fatalf("playback admitted without a strength report: %v", err)
	}
	if _, err := f.owner.AdjustStrength(estim.ChannelA, 1); !errors.Is(err, estim.ErrStrengthUnconfirmed) {
		t.Fatalf("strength increase admitted without a report: %v", err)
	}
	link.TurnSwitch(estim.ChannelA, 1)
	link.TurnSwitch(estim.ChannelA, -1)
	f.waitFor("reported zero", func(status CoyoteStatus) bool {
		return status.StrengthKnown && status.StrengthSource == estim.StrengthReported && status.StrengthA == 0 && status.StrengthB == 0
	})
	if _, err := f.owner.MotionTransport(); err != nil {
		t.Fatalf("reported zero should admit a silent motion session: %v", err)
	}
	if err := f.owner.EmergencyZero(); err != nil {
		t.Fatal(err)
	}
	if status := f.owner.Status(); status.StrengthKnown {
		t.Fatalf("a second unacknowledged zero reused an old report: %+v", status)
	}
}
