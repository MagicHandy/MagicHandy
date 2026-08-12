package httpapi

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
)

func choices(ids ...string) []chat.PatternChoice {
	out := make([]chat.PatternChoice, 0, len(ids))
	for _, id := range ids {
		out = append(out, chat.PatternChoice{ID: id})
	}
	return out
}

func ids(choices []chat.PatternChoice) []string {
	out := make([]string, 0, len(choices))
	for _, choice := range choices {
		out = append(out, choice.ID)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Autopilot held one pattern for a whole session with that same id sitting in
// the recent list. Withholding what just played leaves the model choosing among
// what it has not used, which is the property the deterministic planner already
// has through recencyPenalty.
func TestWithoutRecentPatterns(t *testing.T) {
	for _, test := range []struct {
		name   string
		menu   []chat.PatternChoice
		recent []string
		want   []string
	}{
		{
			name:   "recent ones are withheld",
			menu:   choices("a", "b", "c", "d", "e", "f"),
			recent: []string{"b", "d"},
			want:   []string{"a", "c", "e", "f"},
		},
		{
			name:   "matching ignores case and padding",
			menu:   choices("Stroke", "pulse", "tease", "drive", "roll", "climb"),
			recent: []string{" STROKE ", "Pulse"},
			want:   []string{"tease", "drive", "roll", "climb"},
		},
		{
			name:   "no history leaves the menu alone",
			menu:   choices("a", "b", "c", "d", "e"),
			recent: nil,
			want:   []string{"a", "b", "c", "d", "e"},
		},
		{
			// A forced choice is worse than a repeated one, so a small library
			// keeps every option rather than being narrowed toward empty.
			name:   "a small library is never narrowed",
			menu:   choices("a", "b", "c"),
			recent: []string{"a", "b"},
			want:   []string{"a", "b", "c"},
		},
		{
			name:   "withholding stops at the floor",
			menu:   choices("a", "b", "c", "d", "e"),
			recent: []string{"a", "b"},
			want:   []string{"a", "b", "c", "d", "e"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := ids(withoutRecentPatterns(test.menu, test.recent))
			if !equal(got, test.want) {
				t.Errorf("withoutRecentPatterns() = %v, want %v", got, test.want)
			}
		})
	}
}

// The menu is a nudge, not a gate: nothing here disables a pattern, so a model
// that names a withheld id still resolves against the enabled library.
func TestWithoutRecentPatternsDoesNotMutateInput(t *testing.T) {
	menu := choices("a", "b", "c", "d", "e", "f")
	withoutRecentPatterns(menu, []string{"a", "b"})
	if got := ids(menu); !equal(got, []string{"a", "b", "c", "d", "e", "f"}) {
		t.Errorf("input menu was mutated: %v", got)
	}
}
