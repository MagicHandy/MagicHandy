package transport

import (
	"testing"
	"time"
)

// Health failure marks a connection stale before its asynchronous teardown
// gets the command lane. A covered frame must not revive it in that gap.
func TestAutoblowPacerCannotReviveStaleConnection(t *testing.T) {
	t.Run("Ultra", func(t *testing.T) {
		f := newAutoblowFixture(t)
		now := time.Now()
		f.options.Tick, f.options.HealthInterval = time.Hour, time.Hour
		f.options.Clock = func() time.Time { return now }
		u := connectedUltra(t, f)
		play(t, u, sweep(12, 400))
		u.mu.Lock()
		u.stream.origin.StartAt(now.Add(-time.Second))
		u.stream.started = true
		u.state = "stale"
		u.mu.Unlock()
		before := len(commandsTo(f.sim, "autoblow", "goto"))
		u.paceOnce()
		if state := u.Status().PlaybackState; state != "stale" {
			t.Errorf("covered frame revived stale connection: %s", state)
		}
		if after := len(commandsTo(f.sim, "autoblow", "goto")); after != before {
			t.Errorf("stale connection sent a go-to: %d -> %d", before, after)
		}
	})
	t.Run("VacuGlide", func(t *testing.T) {
		f := newAutoblowFixture(t)
		now := time.Now()
		f.options.Tick, f.options.HealthInterval = time.Hour, time.Hour
		f.options.Clock = func() time.Time { return now }
		v := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 5, TempoMaxPercent: 100})
		play(t, v, sweep(12, 400))
		v.mu.Lock()
		v.stream.origin.StartAt(now.Add(-time.Second))
		v.state = "stale"
		v.mu.Unlock()
		before := len(commandsTo(f.sim, "vacuglide", "target-speed"))
		v.paceOnce()
		if state := v.Status().PlaybackState; state != "stale" {
			t.Errorf("covered frame revived stale connection: %s", state)
		}
		if after := len(commandsTo(f.sim, "vacuglide", "target-speed")); after != before {
			t.Errorf("stale connection sent motor speed: %d -> %d", before, after)
		}
	})
}
