//go:build magichandy_labs

package motion

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

type speedReviewTransport struct{ *transport.Fake }

func (*speedReviewTransport) MotionSamplingCapabilities() transport.MotionSamplingCapabilities {
	return (&transport.CloudRESTTransport{}).MotionSamplingCapabilities()
}

func (*speedReviewTransport) MotionTimingCapabilities() transport.MotionTimingCapabilities {
	return (&transport.CloudRESTTransport{}).MotionTimingCapabilities()
}

func (o *speedReviewTransport) AppendPoints(ctx context.Context, command transport.AppendPointsCommand) (transport.CommandResult, error) {
	timer := time.NewTimer(350 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
		return o.Fake.AppendPoints(ctx, command)
	case <-ctx.Done():
		return transport.CommandResult{}, ctx.Err()
	}
}

// TestSpeedReviewRuntime captures the shared engine at Cloud's actual sampling
// and buffering capabilities, with simulated 350ms append latency. It never
// constructs a network client or contacts hardware.
func TestSpeedReviewRuntime(t *testing.T) {
	path := os.Getenv("MAGICHANDY_SPEED_RUNTIME_REPORT")
	if path == "" {
		t.Skip("set MAGICHANDY_SPEED_RUNTIME_REPORT for an inert runtime capture")
	}
	initial, next := fullSpeedGesture(), fullSpeedGesture()
	initial.SpeedPercent, next.SpeedPercent = 75, 90
	runSpeedReviewRuntime(t, path, initial, next)
}

func TestLayeredSpeedSupportRuntime(t *testing.T) {
	path := os.Getenv("MAGICHANDY_LAYERED_SPEED_RUNTIME_REPORT")
	if path == "" {
		t.Skip("set MAGICHANDY_LAYERED_SPEED_RUNTIME_REPORT for an inert Cloud capture")
	}
	runSpeedReviewRuntime(t, path, speedSupportSpec("full", 100), speedSupportSpec("two-upper-one-full", 100))
}

func runSpeedReviewRuntime(t *testing.T, path string, initial, next FlowSpec) {
	t.Helper()
	owner := &speedReviewTransport{transport.NewFake(transport.WithLatency(350 * time.Millisecond))}
	traces := diagnostics.NewTraceRing(256)
	engine, err := NewEngine(EngineOptions{Transport: owner, Traces: traces})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "review cleanup") })
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	target, err := FlowTarget(initial, settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Start(t.Context(), target, settings); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Second)
	before := engine.Snapshot()
	target, err = FlowTarget(next, settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.ApplyTarget(t.Context(), target, "speed review retarget"); err != nil {
		t.Fatal(err)
	}
	// Let the buffered transition and new pattern both enter the playback
	// window; an immediate Stop could otherwise cancel the whole new phrase.
	time.Sleep(6 * time.Second)
	after := engine.Snapshot()
	stopped, err := engine.Stop(t.Context(), "speed review completed")
	if err != nil || stopped.Running || before.LastError != "" || after.LastError != "" {
		t.Fatal("runtime failed", before.LastError, after.LastError, err)
	}
	commands := owner.Commands()
	if countCommands(commands, transport.CommandKindPointsPlay) != 1 || countCommands(commands, transport.CommandKindPointsAdd) < 4 {
		t.Fatal("healthy retarget restarted playback or starved its buffer")
	}
	time.Sleep(100 * time.Millisecond)
	if len(owner.Commands()) != len(commands) {
		t.Fatal("command escaped Stop")
	}
	payloads := speedReviewCloudMappings(t, commands)
	scenario := fmt.Sprintf("%s at %d%% → %s at %d%% → Stop", before.Target.PatternName, before.Target.SpeedPercent, after.Target.PatternName, after.Target.SpeedPercent)
	writeSpeedReviewReport(t, path, map[string]any{"scenario": scenario, "simulated_append_latency_ms": 350, "before": before, "after": after, "stopped": stopped,
		"commands": commands, "point_mapping_examples": payloads, "trace": traces.Export()})
}

func speedReviewCloudMappings(t *testing.T, commands []transport.Command) []transport.CloudRequest {
	t.Helper()
	builder, err := transport.NewCloudRESTBuilder(transport.CloudPrerequisites{ApplicationID: "inert-speed-review", ConnectionKey: "synthetic", FirmwareMajor: 4, APIMajor: 3, HSPAvailable: true}, transport.CloudBuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var payloads []transport.CloudRequest
	for _, command := range commands {
		if command.PointsAdd == nil {
			continue
		}
		request, err := builder.BuildHSPAdd(*command.PointsAdd)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Points []transport.CloudHSPPoint `json:"points"`
		}
		if err := json.Unmarshal(encoded, &body); err != nil {
			t.Fatal(err)
		}
		for i, point := range body.Points {
			original := command.PointsAdd.Points[i]
			if point.T != original.TimeMillis || point.X != int(math.Round(original.PositionPercent)) {
				t.Fatal("Cloud changed sampled timing or position units")
			}
		}
		payloads = append(payloads, request)
	}
	return payloads
}
