package media

import (
	"fmt"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// BenchmarkMediaSeek includes slicing, optional filters, and compilation by the
// shared engine. It excludes decoder and transport latency, which vary by host.
func BenchmarkMediaSeek(b *testing.B) {
	actions := make([]FunscriptAction, MaxMediaFunscriptActions)
	for index := range actions {
		actions[index] = FunscriptAction{AtMillis: int64(index) * 100, Position: 20 + (index%2)*60}
	}
	script := Funscript{VideoID: "benchmark", Name: "Synthetic seek", Actions: actions, DurationMillis: actions[len(actions)-1].AtMillis}
	for _, test := range []struct {
		name    string
		at      int64
		filters Filters
	}{
		{name: "start"},
		{name: "middle", at: script.DurationMillis / 2},
		{name: "near_end", at: script.DurationMillis * 9 / 10},
		{name: "rounded_middle", at: script.DurationMillis / 2, filters: Filters{PeakRoundingMillis: 60}},
	} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				timeline, _, err := script.TimelineFrom(test.at, 1, test.filters)
				if err != nil {
					b.Fatal(err)
				}
				plan := motion.NewMotionPlan("seek", motion.MotionTarget{Source: motion.TargetSourceMedia, Media: &timeline}, config.DefaultSettings().Motion, 0, 0, time.Time{})
				if plan.Target.Media == nil {
					b.Fatal("media plan lost its timeline")
				}
			}
		})
	}
}

func BenchmarkMediaSeekSmoothing(b *testing.B) {
	for _, count := range []int{1000, 10_000, 100_000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			actions := make([]FunscriptAction, count)
			for index := range actions {
				actions[index] = FunscriptAction{AtMillis: int64(index) * 100, Position: 50 + index%2}
			}
			script := Funscript{VideoID: "smoothing", Name: "Synthetic chatter", Actions: actions, DurationMillis: actions[len(actions)-1].AtMillis}
			b.ReportAllocs()
			for b.Loop() {
				if _, _, err := script.TimelineFrom(0, 1, Filters{SmoothingPercent: 3}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
