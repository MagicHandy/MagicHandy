package bleworker

import (
	"context"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/ble"
	"github.com/mapledaemon/MagicHandy/internal/ble/workerproto"
)

type discoveryAdapter interface {
	Discover(context.Context, ble.Profile) ([]ble.Candidate, error)
}

func (s *server) startDiscover(ctx context.Context, message workerproto.Message) {
	profile, ok := ble.LookupProfile(message.Profile)
	adapter, capable := s.adapter.(discoveryAdapter)
	if !ok || !capable {
		s.out.fail(message.ID, workerproto.CodeBadRequest, "discovery is unavailable for this profile")
		return
	}
	s.mu.Lock()
	if s.attempted {
		s.mu.Unlock()
		s.out.fail(message.ID, workerproto.CodeBusy, "this worker already owns an operation")
		return
	}
	s.attempted = true
	s.mu.Unlock()
	s.tasks.Add(1)
	go func() {
		defer s.tasks.Done()
		if err := s.enable(); err != nil {
			s.out.fail(message.ID, errorCode(err), err.Error())
			return
		}
		duration := max(time.Second, min(durationOr(message.ScanTimeout, defaultScanTimeout), 15*time.Second))
		scan, cancel := context.WithTimeout(ctx, duration)
		defer cancel()
		candidates, err := adapter.Discover(scan, profile)
		if err != nil {
			s.out.fail(message.ID, errorCode(err), "Bluetooth discovery did not complete")
			return
		}
		if len(candidates) > 64 {
			candidates = candidates[:64]
		}
		s.out.send(workerproto.Message{Type: workerproto.TypeDiscovered, ID: message.ID, Candidates: candidates})
	}()
}
