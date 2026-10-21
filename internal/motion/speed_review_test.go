//go:build magichandy_labs

package motion

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// TestSpeedReview exports commanded curves without constructing a transport.
// Set MAGICHANDY_SPEED_REPORT to an ignored local JSON path for visual review.
func TestSpeedReview(t *testing.T) {
	path := os.Getenv("MAGICHANDY_SPEED_REPORT")
	if path == "" {
		t.Skip("set MAGICHANDY_SPEED_REPORT for an inert speed atlas")
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	var entries []Review
	for _, model := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		settings.HandyModel = model
		for _, speed := range []int{25, 50, 75, 100} {
			for _, name := range []string{"v2-full", "v2-narrow", "v2-varied", "v2-directional", "layered", "creative"} {
				s := DefaultFlowSpec()
				s.MinPercent, s.MaxPercent, s.SpeedPercent = 0, 100, speed
				g := DefaultGestureSpec()
				g.FocusMixPercent, g.VariationPercent = 0, 0
				s.Gesture = &g
				switch name {
				case "v2-narrow":
					s.MinPercent, s.MaxPercent, s.RangeFloorPercent, g.FocusWidthPercent = 40, 60, 10, 10
				case "v2-varied":
					g = DefaultGestureSpec()
				case "v2-directional":
					g.FasterDirection, g.ContrastPercent, g.InertiaPercent = "tip", 65, 70
				case "layered":
					s.Gesture, s.PaceVariationPercent = nil, 0
				}
				target, err := FlowTarget(s, settings)
				if name == "creative" {
					d := NormalizeDynamicDefinition(DynamicDefinition{CenterPercent: 50, SpanPercent: 100, SpanMinPercent: 100, SpanProfile: "steady"})
					target = MotionTarget{SpeedPercent: speed, Dynamic: &d}
				}
				if err != nil {
					t.Fatal(err)
				}
				entry := ReviewMotionOutput(target, settings)
				entry.ID, entry.Name, entry.Group = fmt.Sprintf("%s-%s-%d", name, model, speed), name, model
				entries = append(entries, entry)
				t.Logf("%s %s %d: mean=%.2f effective=%.1f peak=%.2f accel=%.0f jerk=%.0f %v error=%s", model, name, speed,
					entry.Summary.CommandedMeanTravelPerSecond, entry.Summary.Pace.EffectivePercent, entry.Summary.CommandedPeakVelocityPerSecond,
					entry.PeakAcceleration, entry.PeakJerk, entry.Summary.Pace.Limiters, entry.Error)
				if entry.Error != "" {
					t.Error(entry.Error)
				}
			}
		}
	}
	writeSpeedReviewReport(t, path, map[string]any{"schema": "magichandy.motion-atlas.v1", "entries": entries})
}

func writeSpeedReviewReport(t *testing.T, path string, report any) {
	t.Helper()
	scratch, err := filepath.Abs(filepath.Join("..", "..", ".scratch"))
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(scratch, path)
	if err != nil || !filepath.IsLocal(relative) {
		t.Fatal("review reports must stay inside the repository's .scratch directory")
	}
	root, err := os.OpenRoot(scratch)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile(relative, data, 0600); err != nil {
		t.Fatal(err)
	}
}
