package httpapi

import (
	"encoding/json"
	"math"
	"net/http"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestMediaSyncReportsCompiledFiltersIncludingMeasuredZero(t *testing.T) {
	for _, rounding := range []int{0, 100} {
		t.Run(map[int]string{0: "measured-zero", 100: "after-speed-cap"}[rounding], func(t *testing.T) {
			server := newTestServer(t)
			root := t.TempDir()
			writeMediaPair(t, root, "Filters", `{"actions":[{"at":0,"pos":0},{"at":500,"pos":100},{"at":1000,"pos":0}]}`)
			saveSettings(t, server.store, func(settings config.Settings) config.Settings {
				settings.Media.LibraryPaths = []string{root}
				settings.Media.ScriptSmoothingPercent = 3
				settings.Media.PeakRoundingMillis = rounding
				settings.Motion.ApplyVideoSpeedLimit = true
				settings.Motion.SpeedMaxPercent = 25
				return settings
			})
			if _, err := server.media.StartScan([]string{root}); err != nil {
				t.Fatal(err)
			}
			waitForMediaScan(t, server)
			video := mustSingleMediaVideo(t, server)
			response := postMediaSync(t, server, server.stopSequence.Load(), video.ID, "playing", "play", 0, 1)
			if response.Code != http.StatusOK {
				t.Fatal(response.Body.String())
			}
			var body struct {
				Sync mediaSyncStatus `json:"sync"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			effect := body.Sync.FilterEffect
			if effect == nil || effect.SmoothingPercent != 3 || effect.RoundingMillis != rounding || !effect.SpeedLimitEnabled || effect.ActionsRemoved != 0 {
				t.Fatalf("wrong applied policy or missing zero report: %+v", effect)
			}
			compiled := server.currentMotionEngine().Snapshot().Target.MediaRoundingEffect
			if effect.RoundedCorners != compiled.RoundedCorners || effect.PeakReductionPercent != math.Round(compiled.PeakReductionPercent*10)/10 {
				t.Fatalf("effect differs from compiled motion: %+v vs %+v", effect, compiled)
			}
			if rounding > 0 && (effect.RoundedCorners != 1 || effect.PeakReductionPercent <= 0 || effect.PeakReductionPercent >= 7.5) {
				t.Fatalf("expected post-cap reduction below uncapped 7.5%%: %+v", effect)
			}
		})
	}
}
