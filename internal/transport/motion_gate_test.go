package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMotionCommandGateInvalidatesOldAndConcurrentAdmissions(t *testing.T) {
	var gate motionCommandGate
	oldAdmission, err := gate.admit()
	if err != nil {
		t.Fatalf("initial admission: %v", err)
	}

	gate.beginStop()
	gate.beginStop()
	if err := gate.validate(oldAdmission); !errors.Is(err, errMotionCommandInvalidated) {
		t.Fatalf("old admission error = %v, want invalidated", err)
	}
	if _, err := gate.admit(); !errors.Is(err, errMotionCommandInvalidated) {
		t.Fatalf("admission during Stop error = %v, want invalidated", err)
	}
	gate.endStop()
	if _, err := gate.admit(); !errors.Is(err, errMotionCommandInvalidated) {
		t.Fatalf("admission while second Stop remains = %v, want invalidated", err)
	}
	gate.endStop()
	newAdmission, err := gate.admit()
	if err != nil {
		t.Fatalf("admission after Stops: %v", err)
	}
	if err := gate.validate(newAdmission); err != nil {
		t.Fatalf("new admission validation: %v", err)
	}
}

func TestHandyOwnersRejectMotionWhileStopGateIsActive(t *testing.T) {
	t.Run("cloud_rest", func(t *testing.T) {
		requestCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			requestCount++
		}))
		defer server.Close()
		owner := newTestCloudTransport(t, server.URL)
		owner.motionGate.beginStop()
		defer owner.motionGate.endStop()

		_, err := owner.AppendPoints(context.Background(), AppendPointsCommand{
			StreamID: "blocked",
			Points:   []TimedPoint{{PositionPercent: 50}},
		})
		if !errors.Is(err, errMotionCommandInvalidated) {
			t.Fatalf("AppendPoints error = %v, want Stop invalidation", err)
		}
		if requestCount != 0 {
			t.Fatalf("Cloud request count = %d, want 0", requestCount)
		}
	})

	t.Run("browser_bluetooth", func(t *testing.T) {
		bridge := NewBrowserBluetoothBridge()
		connected := true
		bridge.ConnectClient(BrowserBluetoothClientStatus{ClientID: "gate-client", Connected: &connected})
		owner := newTestBrowserBluetoothTransport(t, bridge, BrowserBluetoothOptions{})
		owner.motionGate.beginStop()
		defer owner.motionGate.endStop()

		_, err := owner.AppendPoints(context.Background(), AppendPointsCommand{
			StreamID: "blocked",
			Points:   []TimedPoint{{PositionPercent: 50}},
		})
		if !errors.Is(err, errMotionCommandInvalidated) {
			t.Fatalf("AppendPoints error = %v, want Stop invalidation", err)
		}
		if pending := bridge.Snapshot().Pending; pending != 0 {
			t.Fatalf("Bluetooth pending commands = %d, want 0", pending)
		}
	})
}
