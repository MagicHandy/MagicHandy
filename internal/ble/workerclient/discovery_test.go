package workerclient

import (
	"context"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/ble"
)

func TestWorkerProcessDiscoveryThenIndependentConnection(t *testing.T) {
	c := NewConnector(Options{Profile: ble.ProfileCoyote, Path: simulatedWorker(t), Args: []string{"-simulate"}})
	candidates, err := c.Discover(context.Background(), time.Second)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("discovery = %+v %v", candidates, err)
	}
	link, err := c.Connect(context.Background(), ble.ConnectRequest{RememberedAddress: candidates[0].Address, SelectedAddress: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := link.Close(); err != nil {
		t.Fatal(err)
	}
}
