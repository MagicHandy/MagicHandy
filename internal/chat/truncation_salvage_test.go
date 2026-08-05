package chat

import "testing"

// A reply cut off at the output cap used to be sent into a repair that could not
// succeed: the correction is longer than the text that already did not fit, and
// the repair carried the same token budget. That burned a second full generation
// on the way to a second failure. The prose is already in the partial object, so
// it is recovered instead.
func TestSalvageTruncatedReply(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "cut mid string is the common case",
			raw:  `{"reply": "I push you down and start telling you about the night we`,
			want: "I push you down and start telling you about the night we",
		},
		{
			name: "escapes are decoded",
			raw:  `{"reply": "Line one.\nShe said \"stay\" and I did. Then`,
			want: "Line one.\nShe said \"stay\" and I did. Then",
		},
		{
			name: "unicode escape is decoded",
			raw:  `{"reply": "café and then`,
			want: "café and then",
		},
		{
			name: "closed string stops at the quote",
			raw:  `{"reply": "All of it.", "motion": {"action":"start"`,
			want: "All of it.",
		},
		{
			name: "half written escape is dropped rather than mangled",
			raw:  `{"reply": "almost there\`,
			want: "almost there",
		},
		{
			name: "other keys before reply do not confuse it",
			raw:  `{"new_mood": "Passionate", "reply": "Come here and`,
			want: "Come here and",
		},
		{name: "no reply key", raw: `{"motion": {"action":"stop"}}`, want: ""},
		{name: "reply is not a string", raw: `{"reply": 12`, want: ""},
		{name: "empty", raw: "", want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := salvageTruncatedReply(test.raw); got != test.want {
				t.Errorf("salvageTruncatedReply() = %q, want %q", got, test.want)
			}
		})
	}
}

// Motion must never be taken from a partial object: the fields after the reply
// were not necessarily written, and a half-parsed command could move the device.
func TestSalvageNeverCarriesMotion(t *testing.T) {
	raw := `{"reply": "Faster then.", "motion": {"action":"start","speed_percent":90`
	if got := salvageTruncatedReply(raw); got != "Faster then." {
		t.Fatalf("reply = %q", got)
	}
	// The salvage path in Complete constructs AssistantResponse{Reply: ...}
	// only, so there is no route from this string to a motion command.
	response := AssistantResponse{Reply: salvageTruncatedReply(raw)}
	if response.Motion != nil {
		t.Fatal("salvaged response carried a motion command")
	}
}

func TestRepairPromptAsksForBrevityOnlyWhenTruncated(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	const marker = "cut off because it exceeded the output limit"
	if got := repairPromptFor(set, "bad json", true); !contains(got, marker) {
		t.Errorf("truncated repair prompt is missing the length instruction:\n%s", got)
	}
	if got := repairPromptFor(set, "bad json", false); contains(got, marker) {
		t.Errorf("non-truncated repair prompt should not mention the output limit:\n%s", got)
	}
	if got := RepairPrompt(set, "bad json"); contains(got, marker) {
		t.Errorf("RepairPrompt should keep its original non-truncated wording:\n%s", got)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
