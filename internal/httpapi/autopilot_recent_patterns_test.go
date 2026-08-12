package httpapi

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
)

func patternChoices(ids ...string) []chat.PatternChoice {
	out := make([]chat.PatternChoice, 0, len(ids))
	for _, id := range ids {
		out = append(out, chat.PatternChoice{ID: id})
	}
	return out
}

func patternChoiceIDs(choices []chat.PatternChoice) []string {
	out := make([]string, 0, len(choices))
	for _, choice := range choices {
		out = append(out, choice.ID)
	}
	return out
}

func equalStrings(a, b []string) bool {
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
			menu:   patternChoices("a", "b", "c", "d", "e", "f"),
			recent: []string{"b", "d"},
			want:   []string{"a", "c", "e", "f"},
		},
		{
			name:   "matching ignores case and padding",
			menu:   patternChoices("Stroke", "pulse", "tease", "drive", "roll", "climb"),
			recent: []string{" STROKE ", "Pulse"},
			want:   []string{"tease", "drive", "roll", "climb"},
		},
		{
			name:   "no history leaves the menu alone",
			menu:   patternChoices("a", "b", "c", "d", "e"),
			recent: nil,
			want:   []string{"a", "b", "c", "d", "e"},
		},
		{
			// A forced choice is worse than a repeated one, so a small library
			// keeps every option rather than being narrowed toward empty.
			name:   "a small library is never narrowed",
			menu:   patternChoices("a", "b", "c"),
			recent: []string{"a", "b"},
			want:   []string{"a", "b", "c"},
		},
		{
			name:   "withholding stops at the floor",
			menu:   patternChoices("a", "b", "c", "d", "e"),
			recent: []string{"a", "b"},
			want:   []string{"a", "b", "c", "d", "e"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := patternChoiceIDs(withoutRecentPatterns(test.menu, test.recent))
			if !equalStrings(got, test.want) {
				t.Errorf("withoutRecentPatterns() = %v, want %v", got, test.want)
			}
		})
	}
}

// Building the turn-specific allow-list must not mutate the complete enabled
// catalog retained by the server for later turns and interactive chat.
func TestWithoutRecentPatternsDoesNotMutateInput(t *testing.T) {
	menu := patternChoices("a", "b", "c", "d", "e", "f")
	withoutRecentPatterns(menu, []string{"a", "b"})
	if got := patternChoiceIDs(menu); !equalStrings(got, []string{"a", "b", "c", "d", "e", "f"}) {
		t.Errorf("input menu was mutated: %v", got)
	}
}
