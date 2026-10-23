package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// Called with the instance operation lock held; do not acquire it again while
// tearing down a canceled connection. Shared Intiface peers retain their leases.
func (s *Server) disposeNamedHandy(ctx context.Context, id, method string, owner transport.Transport) error {
	var err error
	if owner != nil {
		_, err = owner.Stop(ctx, transport.StopCommand{Reason: "handy_disconnected"})
	}
	switch method {
	case config.DispatchOwnerNativeBluetooth:
		err = errors.Join(err, s.handyEntry(id).native.Disconnect(ctx))
	case config.DispatchOwnerIntiface:
		err = errors.Join(err, s.releaseNamedHandyIntiface(ctx, id))
	}
	return err
}

// Stop reaches connected instances outside the active engine, including a new
// incarnation that the old run is deliberately unable to address.
func (s *Server) stopNamedHandysOutsideEngine(ctx context.Context, engineOwner, reason string) error {
	settings, _ := s.store.Snapshot()
	var participants []transport.LinkedParticipant
	for _, c := range settings.Device.HandyConnections {
		owner, generation := s.handyEntry(c.ID).current()
		if owner == nil {
			continue
		}
		bound := engineOwnerPlays(engineOwner, c.ID) && strings.Contains(engineOwner+"/", fmt.Sprintf("/%s-%d/", c.ID, generation))
		if !bound {
			participants = append(participants, transport.LinkedParticipant{ID: c.ID, Label: c.Name, Transport: owner})
		}
	}
	if len(participants) == 0 {
		return nil
	}
	_, err := transport.NewLinkedSession(participants, false).Stop(ctx, transport.StopCommand{Reason: reason})
	return err
}
