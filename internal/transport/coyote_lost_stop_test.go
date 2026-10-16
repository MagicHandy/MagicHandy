package transport

import (
	"errors"
	"testing"
)

func TestCoyoteLostLinkCannotConfirmStopOrZero(t *testing.T) {
	f := newCoyoteFixture(t)
	if err := f.owner.EmergencyZero(); err != nil {
		t.Fatalf("never-connected owner needs no device Stop: %v", err)
	}
	link := f.connect()
	f.confirm()
	session, err := f.owner.MotionTransport()
	if err != nil {
		t.Fatal(err)
	}
	link.Lose(errors.New("test radio loss"))
	f.waitFor("link loss", func(s CoyoteStatus) bool { return !s.Connected })
	if err := f.owner.EmergencyZero(); !errors.Is(err, errCoyoteStopUnconfirmed) {
		t.Fatalf("lost zero confirmed: %v", err)
	}
	for _, owner := range []Transport{f.owner, session} {
		result, err := owner.Stop(t.Context(), StopCommand{Fault: true})
		if result.OK || !errors.Is(err, errCoyoteStopUnconfirmed) {
			t.Fatalf("lost Stop confirmed: %+v, %v", result, err)
		}
	}
	// A replacement link still must not receive commands from the old engine.
	replacement := f.connect()
	f.confirm()
	before := replacement.Snapshot().Packets
	if _, err := session.Stop(t.Context(), StopCommand{Fault: true}); err != nil {
		t.Fatal(err)
	}
	if replacement.Snapshot().Packets != before {
		t.Fatal("old session commanded its replacement")
	}
	if err := f.owner.EmergencyZero(); err != nil {
		t.Fatalf("fresh connection could not zero: %v", err)
	}
}
