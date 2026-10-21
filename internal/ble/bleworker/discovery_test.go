package bleworker

import (
	"context"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/ble"
	"github.com/mapledaemon/MagicHandy/internal/ble/workerproto"
)

func TestDiscoveryNeverConnectsAndRetiresWorker(t *testing.T) {
	h := startHarness(t)
	h.send(workerproto.Message{Type: workerproto.TypeDiscover, Profile: ble.ProfileCoyote, ID: "scan"})
	reply := h.next(workerproto.TypeDiscovered)
	if len(reply.Candidates) != 1 || h.connector.LastLink() != nil {
		t.Fatal("discovery must only list advertisements")
	}
	h.send(workerproto.Message{Type: workerproto.TypeConnect, Profile: ble.ProfileCoyote, ID: "connect"})
	if reply := h.next(workerproto.TypeError); reply.Code != workerproto.CodeBusy {
		t.Fatalf("discovery worker admitted connection: %+v", reply)
	}
}

type unavailableSelectedAdapter struct {
	*SimulatedAdapter
	scans int
}

func (a *unavailableSelectedAdapter) ConnectAddress(context.Context, string, ble.Profile) (Peripheral, error) {
	return nil, ErrNotFound
}
func (a *unavailableSelectedAdapter) ScanName(context.Context, ble.Profile) (string, error) {
	a.scans++
	return simulatedAddress, nil
}

func TestSelectedAddressNeverFallsBackToAnotherDevice(t *testing.T) {
	a := &unavailableSelectedAdapter{SimulatedAdapter: NewSimulatedAdapter(nil)}
	profile, _ := ble.LookupProfile(ble.ProfileCoyote)
	// Exercise explicit-selection behavior independently of the profile's
	// separate remembered-address protection.
	profile.RememberedOnly = false
	s := &server{adapter: a, profile: profile}
	_, err := s.connect(context.Background(), workerproto.Message{Address: "selected", SelectedAddress: true})
	if err != ErrNotFound || a.scans != 0 {
		t.Fatalf("selected failure fell back: err=%v scans=%d", err, a.scans)
	}
}

func TestRememberedCoyoteNeverFallsBackToAnotherDevice(t *testing.T) {
	a := &unavailableSelectedAdapter{SimulatedAdapter: NewSimulatedAdapter(nil)}
	profile, _ := ble.LookupProfile(ble.ProfileCoyote)
	s := &server{adapter: a, profile: profile}
	_, err := s.connect(context.Background(), workerproto.Message{Address: "remembered"})
	if err != ErrNotFound || a.scans != 0 {
		t.Fatalf("remembered failure fell back: err=%v scans=%d", err, a.scans)
	}
}

type unacknowledgedConnector struct{ ble.Connector }
type unacknowledgedLink struct{ ble.Link }

func (c unacknowledgedConnector) Connect(ctx context.Context, request ble.ConnectRequest) (ble.Link, error) {
	link, err := c.Connector.Connect(ctx, request)
	if err != nil {
		return nil, err
	}
	return unacknowledgedLink{link}, nil
}
func (l unacknowledgedLink) Device() ble.DeviceInfo {
	info := l.Link.Device()
	info.UnacknowledgedWrites = true
	return info
}

func TestWorkerReportsUnacknowledgedWritesCapability(t *testing.T) {
	h := startHarness(t)
	h.adapter.Connectors[ble.ProfileCoyote] = unacknowledgedConnector{h.connector}
	if reply := h.connect(); !reply.UnacknowledgedWrites {
		t.Fatal("worker hid the absence of GATT write acknowledgments")
	}
}
