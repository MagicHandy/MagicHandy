package semantic

import "testing"

func TestBoundsFromRegiaoLegacyMatrix(t *testing.T) {
	prefs := DefaultMotionPreferences()
	cases := map[string]struct {
		min float64
		max float64
	}{
		"base":        {0.0, 0.3},
		"meio_base":   {0.0, 0.3},
		"meio":        {0.3, 0.7},
		"meio_cabeca": {0.3, 0.7},
		"cabeca":      {0.7, 1.0},
		"full":        {0.0, 1.0},
		"completo":    {0.0, 1.0},
		"aleatoria":   {0.0, 1.0},
	}
	for regiao, want := range cases {
		min, max, ok := BoundsFromRegiao(regiao, prefs)
		if !ok {
			t.Fatalf("BoundsFromRegiao(%q) not ok", regiao)
		}
		if min != want.min || max != want.max {
			t.Fatalf("BoundsFromRegiao(%q) = %.2f..%.2f, want %.2f..%.2f", regiao, min, max, want.min, want.max)
		}
	}
}

func TestBoundsFromRegiaoHonorsCustomPrefs(t *testing.T) {
	prefs := DefaultMotionPreferences()
	prefs.Zones[ZoneTip] = ZoneRange{Min: 0.75, Max: 0.95}
	min, max, ok := BoundsFromRegiao("cabeca", prefs)
	if !ok || min != 0.75 || max != 0.95 {
		t.Fatalf("bounds = %.2f..%.2f ok=%v", min, max, ok)
	}
}

func TestLocationRegiaoRoundTrip(t *testing.T) {
	cases := []struct {
		regiao   string
		location LocationName
	}{
		{"base", LocationBase},
		{"meio_cabeca", LocationShaft},
		{"cabeca", LocationTip},
		{"full", LocationFull},
	}
	for _, tc := range cases {
		if got := RegiaoToLocation(tc.regiao); got != tc.location {
			t.Fatalf("RegiaoToLocation(%q) = %q, want %q", tc.regiao, got, tc.location)
		}
		if got := LocationToRegiao(tc.location); got == "" {
			t.Fatalf("LocationToRegiao(%q) empty", tc.location)
		}
	}
}
