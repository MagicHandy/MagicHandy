package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCloudCompanionUsesElapsedTimelineAfterClockRefresh(t *testing.T) {
	origin := time.Now().Add(-1500 * time.Millisecond)
	var offset atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/servertime" {
			time.Sleep(50 * time.Millisecond)
			_, _ = fmt.Fprintf(w, `{"server_time":%d}`, time.Now().UnixMilli())
			return
		}
		var body struct {
			Start int64 `json:"start_time"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		offset.Store(body.Start)
		_, _ = w.Write([]byte(`{"result":{"play_state":1}}`))
	}))
	defer server.Close()
	owner := newTestCloudTransport(t, server.URL)
	if _, err := owner.PlayAligned(t.Context(), PlayCommand{StreamID: "shared", StartTimeMillis: 1000}, origin); err != nil {
		t.Fatal(err)
	}
	if offset.Load() < 1550 {
		t.Fatalf("companion offset %d omitted shared elapsed time or clock refresh", offset.Load())
	}
	if delta := owner.PlaybackStartTime().Sub(origin); delta < -100*time.Millisecond || delta > 100*time.Millisecond {
		t.Fatalf("companion origin shifted by %v", delta)
	}
}

func TestLinkedDiagnosticsBudgetSlowestConnectionLatency(t *testing.T) {
	a, b := NewFake(WithLatency(5*time.Millisecond)), NewFake(WithLatency(170*time.Millisecond))
	linked := NewLinkedSession([]LinkedParticipant{{ID: "a", Transport: a}, {ID: "b", Transport: b}}, false)
	result, err := linked.AppendPoints(t.Context(), strokeCommand("shared", 5000))
	if err != nil {
		t.Fatal(err)
	}
	if result.LatencyMillis < 170 || linked.Diagnostics().LastLatencyMillis < 170 {
		t.Fatal("slow companion omitted from refill latency budget")
	}
}

type gatedHandy struct {
	*Fake
	entered chan struct{}
	release chan struct{}
}

func (o *gatedHandy) AppendPoints(ctx context.Context, c AppendPointsCommand) (CommandResult, error) {
	close(o.entered)
	select {
	case <-o.release:
		return o.Fake.AppendPoints(ctx, c)
	case <-ctx.Done():
		return CommandResult{}, ctx.Err()
	}
}
func TestMultipleHandysAppendConcurrentlyAndDrainOnCancel(t *testing.T) {
	a := &gatedHandy{NewFake(), make(chan struct{}), make(chan struct{})}
	b := &gatedHandy{NewFake(), make(chan struct{}), make(chan struct{})}
	linked := NewLinkedSession([]LinkedParticipant{{ID: "a", Label: "A", Transport: a}, {ID: "b", Label: "B", Transport: b}}, false)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := linked.AppendPoints(ctx, strokeCommand("shared", 5000)); done <- err }()
	for _, entered := range []chan struct{}{a.entered, b.entered} {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("append serialized behind a peer")
		}
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled batch succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled batch did not drain")
	}
	for _, owner := range []*gatedHandy{a, b} {
		for _, c := range owner.Commands() {
			if c.Kind == CommandKindPointsAdd {
				t.Fatal("canceled append escaped")
			}
		}
	}
}

func TestNamedIntifaceOwnersClaimReleaseAndRebindOnSharedHub(t *testing.T) {
	server := newFakeButtplugServer(t, 0)
	server.extraDevices = []map[string]any{fakeLinearDevice(8, "Second Handy"), fakeLinearDevice(9, "Third Handy")}
	defer server.Close()
	hub := connectTestHub(t, server)
	if err := hub.Linear().Select(t.Context(), 7, 0); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 3; n++ {
		second := hub.LinearFor("second")
		third := hub.LinearFor("third")
		if !second.Status().Connected {
			t.Fatal("late-bound owner did not become ready")
		}
		if err := second.Select(t.Context(), 7, 0); !errors.Is(err, errIntifaceDeviceClaimed) {
			t.Fatalf("duplicate device selection: %v", err)
		}
		if err := second.Select(t.Context(), 8, 0); err != nil {
			t.Fatal(err)
		}
		if err := third.Select(t.Context(), 9, 0); err != nil {
			t.Fatal(err)
		}
		if err := hub.RemoveLinear(t.Context(), "second"); err != nil {
			t.Fatal(err)
		}
		if !hub.Status().Connected || hub.Linear().Status().SelectedDeviceIndex == nil || third.Status().SelectedDeviceIndex == nil {
			t.Fatal("removing one owner interrupted peers")
		}
		if replacement := hub.LinearFor("second"); replacement == second {
			t.Fatal("removed owner reused")
		}
	}
	if err := hub.EmergencyStop(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestHandyLeaseRejectsEveryStaleWriteButDoesNotStopReplacement(t *testing.T) {
	owner := NewFake()
	var valid atomic.Bool
	valid.Store(true)
	lease := NewLeasedTransport(owner, valid.Load)
	if _, err := lease.SetStrokeWindow(t.Context(), StrokeWindowCommand{MaxPercent: 100}); err != nil {
		t.Fatal(err)
	}
	before := len(owner.Commands())
	valid.Store(false)
	if _, err := lease.AppendPoints(t.Context(), strokeCommand("late", 1000)); err == nil {
		t.Fatal("stale append accepted")
	}
	if _, err := lease.Play(t.Context(), PlayCommand{}); err == nil {
		t.Fatal("stale play accepted")
	}
	if _, err := lease.SetStrokeWindow(t.Context(), StrokeWindowCommand{}); err == nil {
		t.Fatal("stale window accepted")
	}
	if _, err := lease.Stop(t.Context(), StopCommand{}); err != nil {
		t.Fatal(err)
	}
	if len(owner.Commands()) != before || lease.Diagnostics().Connected {
		t.Fatal("stale lease touched transport or reports connected")
	}
}

func TestThreeHandysFailurePolicyAndRetiredPeerCannotRejoin(t *testing.T) {
	for _, keep := range []bool{false, true} {
		t.Run(map[bool]string{false: "stop_all", true: "continue"}[keep], func(t *testing.T) {
			a, b, c := &failingLinkedOwner{Fake: NewFake()}, &failingLinkedOwner{Fake: NewFake()}, &failingLinkedOwner{Fake: NewFake()}
			session := NewLinkedSession([]LinkedParticipant{{ID: "a", Label: "A", Transport: a}, {ID: "b", Label: "B", Transport: b}, {ID: "c", Label: "C", Transport: c}}, keep)
			_, _ = session.AppendPoints(t.Context(), strokeCommand("shared", 5000))
			if _, err := session.Play(t.Context(), PlayCommand{StreamID: "shared"}); err != nil {
				t.Fatal(err)
			}
			b.fail = CommandKindPointsAdd
			_, err := session.AppendPoints(t.Context(), strokeCommand("shared", 8000))
			if (err == nil) != keep {
				t.Fatalf("policy outcome: %v", err)
			}
			if !keep {
				if a.Diagnostics().PlaybackState == "playing" || c.Diagnostics().PlaybackState == "playing" {
					t.Fatal("peer kept playing after failure")
				}
				return
			}
			b.fail = ""
			before := len(b.Commands())
			if _, err := session.AppendPoints(t.Context(), strokeCommand("shared", 9000)); err != nil {
				t.Fatal(err)
			}
			if len(b.Commands()) != before {
				t.Fatal("retired device hot-rejoined")
			}
			_, _ = session.Stop(t.Context(), StopCommand{})
		})
	}
}
