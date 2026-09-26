//go:build liveeval

package chat

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// TestLabModesDepthLanguageLive puts the chat depth cases to each main LLM Lab
// mode through the Lab's own request and parser, from a starting score
// confined to the case's region, and measures the compiled result as
// TestContinuousDepthLanguageLive does. Every mode sees the same message,
// limits and model, so the tallies compare vocabularies. A move of at least
// 5 points in the lower-turn median counts as a direction.
func TestLabModesDepthLanguageLive(t *testing.T) {
	eval := newDepthEval(t)
	repeats, _ := strconv.Atoi(os.Getenv("MAGICHANDY_EVAL_REPEATS"))
	methods := []string{"creative_v2", "layered", LabMethodStrokeEnds, LabMethodGroove, LabMethodPlainWords}
	if filter := os.Getenv("MAGICHANDY_LAB_METHODS"); filter != "" {
		methods = strings.Split(filter, ",")
	}
	rows := []map[string]any{}
	for _, method := range methods {
		tally := labDepthTally{}
		for _, tc := range depthCases() {
			if filter := os.Getenv("MAGICHANDY_EVAL_CASES"); tc.autopilot || (filter != "" && !strings.Contains(","+filter+",", ","+tc.name+",")) {
				continue
			}
			for repeat := range max(1, repeats) {
				row := eval.labTurn(t.Context(), method, tc)
				row["repeat"] = repeat
				rows = append(rows, row)
				tally.add(row, tc.name)
				b, a := row["reach_before"].(depthMetrics), row["reach_after"].(depthMetrics)
				t.Logf("%s/%s/%d pass=%t deepest %.0f->%.0f turn-median %.0f->%.0f err=%v",
					method, tc.name, repeat, row["intent_pass"], b.Deepest, a.Deepest, b.LowerTurnMedian, a.LowerTurnMedian, row["error"])
				writeDepthReport(t, rows)
			}
		}
		t.Logf("%s: depth intent %d/%d, right way %d, wrong way %d, rejected %d",
			method, tally.passes, tally.turns, tally.rightWay, tally.wrongWay, tally.rejected)
	}
}

type labDepthTally struct{ turns, passes, rightWay, wrongWay, rejected int }

// shallowDepthCases ask for less depth; every other chat case asks for more.
var shallowDepthCases = map[string]bool{"just-the-tip": true, "head-only": true, "not-so-deep": true}

func (t *labDepthTally) add(row map[string]any, name string) {
	t.turns++
	if row["intent_pass"] == true {
		t.passes++
	}
	if row["valid"] != true {
		t.rejected++
		return
	}
	move := row["reach_after"].(depthMetrics).LowerTurnMedian - row["reach_before"].(depthMetrics).LowerTurnMedian
	if shallowDepthCases[name] {
		move = -move
	}
	switch {
	case move <= -5:
		t.rightWay++
	case move >= 5:
		t.wrongWay++
	}
}

// labTurn runs one case through the Lab from its own starting score.
func (e depthEval) labTurn(ctx context.Context, method string, tc depthCase) map[string]any {
	before := labDepthStart(method, tc.start, e.limits)
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	trial := RunLLMLab(ctx, e.provider, e.model, method, LLMLabPrompts()[method], tc.message, before, e.limits, nil, true)
	b, a := depthReach(before, e.limits), depthReach(trial.After, e.limits)
	pass := trial.Valid && tc.check(b, a)
	row := map[string]any{"model": e.model, "method": method, "recipe_name": tc.name, "expected_recipe": tc.name,
		"message": tc.message, "valid": trial.Valid, "intent_pass": pass, "reply": trial.Reply, "raw": trial.Raw,
		"limits": e.limits, "before": before, "after": trial.After, "reach_before": b, "reach_after": a,
		"elapsed_ms": trial.ElapsedMillis}
	if trial.Error != "" {
		row["error"] = trial.Error
	}
	return row
}

// labDepthStart confines each mode's starting score to one region. Stroke
// scores turn inside the region's ends and drift up to 10 points inward.
func labDepthStart(method, region string, limits config.MotionSettings) motion.FlowSpec {
	switch method {
	case "creative_v2":
		return depthStart(MotionModeCreativeV2, region)
	case "layered":
		return depthStart(MotionModeLayered, region)
	}
	low, high := 0, 100
	switch region {
	case "upper":
		low, high = 45, 100
	case "middle":
		low, high = 25, 90
	case "deep":
		low, high = 0, 55
	}
	spec := FreshStrokeLabScore(limits)
	spec.Strokes.Bottom = motion.StrokeTurn{AtPercent: low, VaryToPercent: low + 10, Character: "drift"}
	spec.Strokes.Top = motion.StrokeTurn{AtPercent: high, VaryToPercent: high - 10, Character: "drift"}
	spec.Seed = 123456 // Replayable evaluation fixture, never a production seed.
	return spec
}
