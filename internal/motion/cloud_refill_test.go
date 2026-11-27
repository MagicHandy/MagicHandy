package motion

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// In-process HSP peer. The eighth playing append reaches the owner after the
// injected delay. Acknowledged points, play_state and Stop travel through the
// production Cloud encoder and the engine's normal ticker and recovery path.
type refillPeer struct {
	delay          time.Duration
	entered        chan struct{}
	playingSince   time.Time
	tail           int64
	activeAdds     int
	movementWrites int
	atArrival      int64
	stopCount      int
	state          int
}

func (p *refillPeer) RoundTrip(r *http.Request) (*http.Response, error) {
	body := "{}"
	switch strings.Trim(r.URL.Path, "/") {
	case "servertime":
		body = fmt.Sprintf(`{"server_time":%d}`, time.Now().UnixMilli())
	case "slider/state":
		body = `{"result":{"position":0,"position_absolute":0,"speed_absolute":0}}`
	case "slider/stroke":
		body = `{"result":{"min":0,"max":1,"min_absolute":0,"max_absolute":110}}`
	case "hsp/stop":
		p.stopCount++
		p.state, p.playingSince = 2, time.Time{}
	case "hsp/setup":
		p.tail, p.state = 0, 2
	case "hsp/play":
		p.movementWrites++
		p.playingSince, p.state = time.Now(), 1
	case "hsp/add":
		p.movementWrites++
		var add struct{ Points []transport.CloudHSPPoint }
		if err := json.NewDecoder(r.Body).Decode(&add); err != nil {
			return nil, err
		}
		if len(add.Points) == 0 {
			return nil, fmt.Errorf("empty HSP append")
		}
		if !p.playingSince.IsZero() {
			p.activeAdds++
			if p.activeAdds == 8 {
				close(p.entered)
				select {
				case <-r.Context().Done():
					return nil, r.Context().Err()
				case <-time.After(p.delay):
				}
				p.atArrival = p.tail - time.Since(p.playingSince).Milliseconds()
				if p.atArrival < 0 {
					p.state = 4
				}
			}
		}
		p.tail = add.Points[len(add.Points)-1].T
	case "hsp/state":
	default:
		return nil, fmt.Errorf("unexpected Cloud path %s", r.URL.Path)
	}
	if body == "{}" {
		body = fmt.Sprintf(`{"result":{"play_state":%d}}`, p.state)
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
}

func TestCloudRefillArrivalAndStop(t *testing.T) {
	for _, delay := range []time.Duration{time.Second, 1400 * time.Millisecond, 1800 * time.Millisecond, time.Hour} {
		t.Run(delay.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				peer := &refillPeer{delay: delay, entered: make(chan struct{}), state: 2}
				owner, err := transport.NewCloudRESTTransport(transport.CloudPrerequisites{
					ApplicationID: "fixture", ConnectionKey: "inertPeer", FirmwareMajor: 4, APIMajor: 3, HSPAvailable: true,
				}, transport.CloudBuildOptions{}, transport.CloudEndpointConfig{BaseURL: "http://127.0.0.1/"}, &http.Client{Transport: peer})
				if err != nil {
					t.Fatal(err)
				}
				engine, err := NewEngine(EngineOptions{Transport: owner})
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _, _ = engine.Stop(context.Background(), "cleanup") }()
				target := MotionTarget{PatternID: PatternFullSweeps, SpeedPercent: 25}
				if _, err = engine.Start(t.Context(), target, config.DefaultSettings().Motion); err != nil {
					t.Fatal(err)
				}
				select {
				case <-peer.entered:
				case <-time.After(time.Minute):
					t.Fatal("eighth playing refill never reached peer")
				}
				if delay <= 1400*time.Millisecond {
					retargetDuringCloudRefill(t, engine, target, delay)
				}
				if delay == time.Hour {
					before := time.Now()
					if _, err = engine.Stop(t.Context(), "emergency"); err != nil || time.Since(before) > 200*time.Millisecond {
						t.Fatalf("Stop waited for the delayed append: %v", err)
					}
				} else {
					time.Sleep(delay + 250*time.Millisecond)
					synctest.Wait()
					wantRunning := delay <= 1400*time.Millisecond
					t.Logf("previous-tail margin at acceptance: %d ms, running: %t", peer.atArrival, engine.Snapshot().Running)
					if (peer.atArrival >= 0) != wantRunning || engine.Snapshot().Running != wantRunning {
						t.Fatal("refill arrival or starvation recovery disagrees with the delay boundary")
					}
					if wantRunning {
						if _, err = engine.Stop(t.Context(), "done"); err != nil {
							t.Fatal(err)
						}
					}
				}
				writes := peer.movementWrites
				time.Sleep(8 * time.Second)
				synctest.Wait()
				if peer.state != 2 || peer.stopCount == 0 || peer.movementWrites != writes || engine.Snapshot().Running {
					t.Fatal("movement continued after Stop")
				}
			})
		})
	}
}

func retargetDuringCloudRefill(t *testing.T, engine *Engine, target MotionTarget, delay time.Duration) {
	t.Helper()
	edited := make(chan error, 1)
	target.SpeedPercent = 35
	go func() {
		_, err := engine.ApplyTarget(t.Context(), target, "new pace")
		edited <- err
	}()
	synctest.Wait()
	select {
	case err := <-edited:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(delay + 200*time.Millisecond):
		t.Fatal("retarget stalled behind an acknowledged refill")
	}
	engine.mu.Lock()
	handoff := engine.plan.HandoffMillis - engine.estimatedPlaybackMillisLocked(engine.now())
	bound := maxLeadMillis + int64(engine.chunkSize)*engine.sampleInterval.Milliseconds()
	engine.mu.Unlock()
	if engine.Snapshot().Target.SpeedPercent != 35 || handoff > bound {
		t.Fatalf("retarget missed the bounded interactive handoff: lead=%d", handoff)
	}
}
