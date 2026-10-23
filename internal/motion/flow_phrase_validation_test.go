package motion

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestPhraseValidationIdentifiesBrokenJoinWithoutMutating(t *testing.T) {
	for _, tc := range []struct {
		name   string
		groups []PhraseStroke
		want   []string
	}{
		{"loop", []PhraseStroke{{78, 100, 4, 100}, {0, 35, 2, 35}}, []string{"loop seam", "group 2 to group 1", "(35)", "(78)"}},
		{"internal", []PhraseStroke{{0, 35, 2, 35}, {78, 100, 4, 100}}, []string{"join", "group 1 to group 2", "(35)", "(78)"}},
		{"within_group", []PhraseStroke{{95, 100, 4, 100}, {0, 100, 1, 100}}, []string{"group 1 top_percent (100)", "bottom_percent (95)"}},
		{"count", []PhraseStroke{{78, 100, 9, 100}, {0, 100, 1, 100}}, []string{"group 1 count", "got 9"}},
		{"rate", []PhraseStroke{{78, 100, 4, 0}, {0, 100, 1, 100}}, []string{"group 1 rate_percent", "got 0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := phraseFixture(100, 78, 4, 60)
			s.Phrase.Strokes = tc.groups
			before := CloneFlowSpec(&s)
			err := s.Validate(config.DefaultSettings().Motion)
			if err == nil {
				t.Fatal("invalid phrase accepted")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q lacks %q", err, want)
				}
			}
			if !reflect.DeepEqual(&s, before) {
				t.Fatal("validation silently corrected selected motion")
			}
		})
	}
}
