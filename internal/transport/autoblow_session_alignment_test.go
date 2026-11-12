package transport

import (
	"context"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/autoblow"
)

type gatedAutoblowAlignment struct {
	autoblowOwner
	entered, resume chan struct{}
}

func (g *gatedAutoblowAlignment) AlignPlaybackStart(start time.Time) {
	close(g.entered)
	<-g.resume
	g.autoblowOwner.AlignPlaybackStart(start)
}

func (g *gatedAutoblowAlignment) alignPlaybackStart(client *autoblow.Client, start time.Time) {
	close(g.entered)
	<-g.resume
	g.autoblowOwner.alignPlaybackStart(client, start)
}

func TestAutoblowRetiredSessionCannotAlignNewPlayback(t *testing.T) {
	for _, kind := range []string{"Ultra", "VacuGlide"} {
		t.Run(kind, func(t *testing.T) {
			f := newAutoblowFixture(t)
			f.options.Tick, f.options.HealthInterval = time.Hour, time.Hour
			var owner autoblowOwner
			var reconnect func() error
			if kind == "Ultra" {
				u := connectedUltra(t, f)
				owner = u
				reconnect = func() error {
					if _, err := u.Disconnect(); err != nil {
						return err
					}
					_, err := u.Connect(context.Background())
					return err
				}
			} else {
				v := connectedVacuGlide(t, f, AutoblowConfig{TempoMinPercent: 5, TempoMaxPercent: 100})
				owner = v
				reconnect = func() error {
					if _, err := v.Disconnect(); err != nil {
						return err
					}
					_, err := v.Connect(context.Background())
					return err
				}
			}
			gate := &gatedAutoblowAlignment{autoblowOwner: owner, entered: make(chan struct{}), resume: make(chan struct{})}
			base := owner.shared()
			base.mu.Lock()
			old := &autoblowSession{owner: gate, client: base.client}
			base.mu.Unlock()
			done := make(chan struct{})
			go func() { old.AlignPlaybackStart(time.Now().Add(-time.Hour)); close(done) }()
			<-gate.entered
			if err := reconnect(); err != nil {
				close(gate.resume)
				<-done
				t.Fatal(err)
			}
			play(t, owner, sweep(8, 400))
			fresh := owner.PlaybackStartTime()
			close(gate.resume)
			<-done
			if got := owner.PlaybackStartTime(); !got.Equal(fresh) {
				t.Fatalf("retired session re-anchored fresh playback: %s -> %s", fresh, got)
			}
		})
	}
}
