package chat

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"encoding/json"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// realCatalog loads the operator's actual enabled pattern list, exported from a
// running app with `curl /api/library`. defaultPatternChoices offers three
// patterns, which understates both the choice the model really has and the
// prompt size it has to read past: the live app has 87 enabled.
func realCatalog(t *testing.T) []PatternChoice {
	path := os.Getenv("CATALOG")
	if path == "" {
		return defaultPatternChoices()
	}
	raw, err := os.ReadFile(path) //nolint:gosec // operator-supplied export path
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	var payload struct {
		Library struct {
			Patterns []struct {
				ID          string   `json:"id"`
				Name        string   `json:"name"`
				Description string   `json:"description"`
				Tags        []string `json:"tags"`
				Weight      float64  `json:"weight"`
				Enabled     bool     `json:"enabled"`
			} `json:"patterns"`
		} `json:"library"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	choices := make([]PatternChoice, 0, len(payload.Library.Patterns))
	for _, p := range payload.Library.Patterns {
		if !p.Enabled {
			continue
		}
		choices = append(choices, PatternChoice{
			ID: p.ID, Name: p.Name, Description: p.Description,
			Tags: p.Tags, Weight: p.Weight,
		})
	}
	if len(choices) == 0 {
		t.Fatal("catalog export contained no enabled patterns")
	}
	return choices
}

// Measures autopilot motion-decision variability against a live model, driving
// the real AutopilotService so the prompt, contract, parse, and repair are the
// ones the app uses.
//
//	LLAMACPP=http://127.0.0.1:8797 TURNS=24 go test ./internal/chat -run TestAutopilotVariability -v
//
// The reported fault is that the device speed barely moves during a session. A
// real ten-minute trace showed 19 holds against 1 model segment, one pattern
// throughout, speed confined to 45-62 of an allowed 11-85, and "soon" chosen
// for every timing. So the measure is spread across each decision axis, not
// whether any single decision looks reasonable.

type varTally struct {
	turns      int
	holds      int
	errors     int
	speeds     []int
	patterns   map[string]int
	timings    map[string]int
	variabits  map[string]int
	areas      map[string]int
	arcs       map[string]int
	latencySum time.Duration
}

func newVarTally() *varTally {
	return &varTally{
		patterns:  map[string]int{},
		timings:   map[string]int{},
		variabits: map[string]int{},
		areas:     map[string]int{},
		arcs:      map[string]int{},
	}
}

func spread(values []int) (lo, hi, mean int) {
	if len(values) == 0 {
		return 0, 0, 0
	}
	lo, hi = values[0], values[0]
	sum := 0
	for _, v := range values {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
		sum += v
	}
	return lo, hi, sum / len(values)
}

func distinct(values []int) int {
	seen := map[int]bool{}
	for _, v := range values {
		seen[v] = true
	}
	return len(seen)
}

func keyed(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, counts[k]))
	}
	if len(parts) == 0 {
		return "(none)"
	}
	return strings.Join(parts, " ")
}

func (v *varTally) report(label string, bandLow, bandHigh int) {
	lo, hi, mean := spread(v.speeds)
	band := bandHigh - bandLow
	used := 0.0
	if band > 0 && len(v.speeds) > 0 {
		used = 100 * float64(hi-lo) / float64(band)
	}
	holdPct := 0.0
	if v.turns > 0 {
		holdPct = 100 * float64(v.holds) / float64(v.turns)
	}
	fmt.Printf("\n===== %s (turns=%d, band %d-%d) =====\n", label, v.turns, bandLow, bandHigh)
	fmt.Printf("  hold rate     : %3.0f%%  (%d/%d)   errors=%d  avg latency=%.1fs\n",
		holdPct, v.holds, v.turns, v.errors, v.latencySum.Seconds()/float64(max(1, v.turns)))
	fmt.Printf("  speed         : %d-%d (mean %d), %d distinct, spans %.0f%% of the allowed band\n",
		lo, hi, mean, distinct(v.speeds), used)
	fmt.Printf("  patterns      : %s\n", keyed(v.patterns))
	fmt.Printf("  next          : %s\n", keyed(v.timings))
	fmt.Printf("  variability   : %s\n", keyed(v.variabits))
	fmt.Printf("  area          : %s\n", keyed(v.areas))
	fmt.Printf("  arc           : %s\n", keyed(v.arcs))
}

// menuWithoutRecent mirrors httpapi.withoutRecentPatterns, which cannot be
// imported here without a dependency cycle.
func menuWithoutRecent(choices []PatternChoice, recent []string) []PatternChoice {
	const floor = 4
	if len(choices) <= floor || len(recent) == 0 {
		return choices
	}
	withheld := map[string]bool{}
	for _, id := range recent {
		withheld[strings.ToLower(strings.TrimSpace(id))] = true
	}
	kept := make([]PatternChoice, 0, len(choices))
	for _, choice := range choices {
		if withheld[strings.ToLower(strings.TrimSpace(choice.ID))] {
			continue
		}
		kept = append(kept, choice)
	}
	if len(kept) < floor {
		return choices
	}
	return kept
}

func TestAutopilotVariability(t *testing.T) {
	base := os.Getenv("LLAMACPP")
	if base == "" {
		t.Skip("set LLAMACPP")
	}
	_ = loopbackEndpoint(t, base)
	turns := 20
	if v := os.Getenv("TURNS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			turns = parsed
		}
	}

	provider, err := llm.NewLlamaCPPProvider(llm.HTTPProviderOptions{
		BaseURL: base, Model: "local-model", Timeout: 180 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	caps := FullCapabilities()
	caps.Voice = VoiceExplicit
	caps.MoodTracking = true

	catalog := realCatalog(t)
	const bandLow, bandHigh = 11, 85
	service := AutopilotService{
		Provider:      provider,
		Prompt:        set,
		Model:         "local-model",
		MaxTokens:     256,
		ReasoningMode: "off",
		Patterns:      catalog,
		Capabilities:  caps,
		ConversationContext: &ConversationContext{
			PersonaName: "Mara", PersonaDescription: evalPersonas[0].desc,
			UserAnatomy: "penis", CurrentMood: "Intimate",
		},
	}

	tally := newVarTally()
	fmt.Printf("catalog: %d enabled patterns\n", len(catalog))
	// Walk a session the way the manager does: current motion and the session
	// facts advance with each accepted decision.
	current := AutopilotContext{
		Style: "intense", SpeedMinPercent: bandLow, SpeedMaxPercent: bandHigh,
		CurrentPatternID: "stroke", CurrentSpeed: 45, CurrentArea: AreaZoneFull,
		AreaFocusEnabled: true, SessionTracking: true, ArcEnabled: true,
	}
	secondsAtSpeed := 0
	for turn := 0; turn < turns; turn++ {
		current.SegmentIndex = turn
		current.SessionSeconds = turn * 30
		current.SecondsAtCurrentSpeed = secondsAtSpeed
		// The arc fills over the configured session length, not over the number of
		// decisions. Ramping it straight to 100% made the prompt say "aim higher"
		// every turn, which pinned speed at the ceiling and hid everything else.
		const arcMinutes, secondsPerTurn = 20, 30
		current.ArcPercent = min(100, turn*secondsPerTurn*100/(arcMinutes*60))

		// Mirrors withoutRecentPatterns in the httpapi motion path.
		service.Patterns = menuWithoutRecent(catalog, current.RecentPatternIDs)
		began := time.Now()
		response, err := service.Complete(context.Background(), AutopilotKindMotion,
			Request{Message: AutopilotMotionMessage(current)})
		took := time.Since(began)
		tally.turns++
		tally.latencySum += took
		if err != nil {
			tally.errors++
			t.Logf("  turn %2d ERROR %v", turn, err)
			continue
		}
		tally.timings[string(response.Next)]++
		tally.variabits[strings.TrimSpace(response.Variability)]++
		if a := strings.TrimSpace(response.Arc); a != "" {
			tally.arcs[a]++
		}
		tally.applyDecision(t, turn, response, &current, &secondsAtSpeed)
	}
	tally.report(os.Getenv("LABEL")+" autopilot motion", bandLow, bandHigh)
}

// applyDecision folds one response into the running session, so the test body
// stays a loop over turns.
func (v *varTally) applyDecision(
	t *testing.T,
	turn int,
	response AutopilotResponse,
	current *AutopilotContext,
	secondsAtSpeed *int,
) {
	t.Helper()
	m := response.Motion
	if m == nil || m.Action == MotionActionNone {
		v.holds++
		*secondsAtSpeed += 30
		t.Logf("  turn %2d HOLD    speed=%d next=%s var=%s", turn, current.CurrentSpeed, response.Next, response.Variability)
		return
	}
	if m.PatternID != "" {
		current.CurrentPatternID = m.PatternID
	}
	switch {
	case m.SpeedPercent != nil:
		if *m.SpeedPercent != current.CurrentSpeed {
			*secondsAtSpeed = 0
		}
		current.CurrentSpeed = *m.SpeedPercent
	case m.Intensity != nil:
		if *m.Intensity != current.CurrentSpeed {
			*secondsAtSpeed = 0
		}
		current.CurrentSpeed = *m.Intensity
	}
	if m.Area != "" {
		current.CurrentArea = m.Area
	}
	current.RecentPatternIDs = append(current.RecentPatternIDs, current.CurrentPatternID)
	if len(current.RecentPatternIDs) > 4 {
		current.RecentPatternIDs = current.RecentPatternIDs[len(current.RecentPatternIDs)-4:]
	}
	v.speeds = append(v.speeds, current.CurrentSpeed)
	v.patterns[current.CurrentPatternID]++
	v.areas[current.CurrentArea]++
	t.Logf("  turn %2d CHANGE  speed=%d pattern=%s area=%s next=%s var=%s arc=%s",
		turn, current.CurrentSpeed, current.CurrentPatternID, current.CurrentArea, response.Next, response.Variability, response.Arc)
}
