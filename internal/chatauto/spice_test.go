package chatauto

import "testing"

func TestResolveSpiceLevel(t *testing.T) {
	cases := []struct {
		humor    Humor
		progress float64
		allowDom bool
		want     SpiceLevel
	}{
		{HumorDesejando, 10, true, SpiceBiquinho},
		{HumorTesao, 40, true, SpiceDedoDeMoca},
		{HumorIntensa, 60, true, SpiceJalapeno},
		{HumorIntensa, 70, true, SpiceMalagueta},
		{HumorIntensa, 80, false, SpiceMalagueta},
		{HumorIntensa, 70, false, SpiceJalapeno},
		{HumorDominatrix, 95, true, SpiceCarolinaReaper},
	}
	for _, tc := range cases {
		got := ResolveSpiceLevel(tc.humor, tc.progress, tc.allowDom)
		if got != tc.want {
			t.Fatalf("ResolveSpiceLevel(%q, %v, %v) = %q, want %q", tc.humor, tc.progress, tc.allowDom, got, tc.want)
		}
	}
}

func TestFallbackReplyMatchesLevel(t *testing.T) {
	if FallbackReply(SpiceBiquinho) == FallbackReply(SpiceCarolinaReaper) {
		t.Fatal("fallback replies should differ by spice level")
	}
}
