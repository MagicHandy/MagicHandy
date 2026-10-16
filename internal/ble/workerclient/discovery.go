package workerclient

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/ble"
	"github.com/mapledaemon/MagicHandy/internal/ble/workerproto"
)

var _ ble.Discoverer = (*Connector)(nil)

// Discover owns a short-lived scan worker, independent of any connected device.
func (c *Connector) Discover(ctx context.Context, duration time.Duration) ([]ble.Candidate, error) {
	if strings.TrimSpace(c.options.Path) == "" {
		return nil, ble.ErrHelperMissing
	}
	proc, err := startProcess(c.options)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ble.ErrHelperMissing, err)
	}
	link := &workerLink{session: proc.session, process: proc}
	defer link.stop()
	if err := link.hello(ctx, c.options.Profile); err != nil {
		return nil, err
	}
	duration = max(time.Second, min(duration, 15*time.Second))
	scan, cancel := context.WithTimeout(ctx, duration+connectSlack)
	defer cancel()
	reply, err := link.session.request(scan, workerproto.Message{
		Type: workerproto.TypeDiscover, Profile: c.options.Profile, ScanTimeout: int(duration / time.Millisecond),
	})
	if err != nil {
		return nil, err
	}
	if reply.Type != workerproto.TypeDiscovered || len(reply.Candidates) > 64 {
		return nil, fmt.Errorf("%w: invalid discovery reply", ble.ErrHelperMissing)
	}
	return reply.Candidates, nil
}
