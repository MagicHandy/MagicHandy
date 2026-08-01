package motion

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

type curatedCatalogFixture struct {
	Schema              string `json:"schema"`
	StatusPolicy        string `json:"status_policy"`
	NormalSpeedControls bool   `json:"normal_speed_controls"`
	PatternCount        int    `json:"pattern_count"`
	Patterns            []struct {
		File string `json:"file"`
		Name string `json:"name"`
	} `json:"patterns"`
}

func expectedCuratedCatalog(t *testing.T) curatedCatalogFixture {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	catalogPath := filepath.Join(filepath.Dir(file), "builtinpatterns", "curated", "_catalog.json")
	data, err := os.ReadFile(catalogPath) // #nosec G304 -- test fixture path derived from this test file location.
	if err != nil {
		t.Fatalf("read curated catalog: %v", err)
	}
	var catalog curatedCatalogFixture
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("decode curated catalog: %v", err)
	}
	if catalog.PatternCount <= 0 {
		t.Fatalf("curated catalog pattern_count = %d, want > 0", catalog.PatternCount)
	}
	if catalog.Schema != "magichandy.generated-pattern-catalog.v3" ||
		catalog.StatusPolicy != "runtime-budget-audit" || !catalog.NormalSpeedControls {
		t.Fatalf("generated catalog must use normal speed controls: %+v", catalog)
	}
	if len(catalog.Patterns) != catalog.PatternCount {
		t.Fatalf("curated catalog entries = %d, want pattern_count %d", len(catalog.Patterns), catalog.PatternCount)
	}
	return catalog
}

func TestCuratedBuiltinPatternsLoad(t *testing.T) {
	catalog := expectedCuratedCatalog(t)
	definitions := loadCuratedBuiltinPatterns()
	if len(definitions) != catalog.PatternCount {
		t.Fatalf("curated catalog size = %d, want %d", len(definitions), catalog.PatternCount)
	}
	expectedNames := curatedCatalogNames(t, catalog)
	seen := make(map[PatternID]bool, len(definitions))
	seenNames := make(map[string]bool, len(definitions))
	experimental := 0
	stable := make([]PatternID, 0)
	for _, definition := range definitions {
		if assertCuratedBuiltinDefinition(t, definition, expectedNames, seen, seenNames) {
			experimental++
		} else {
			stable = append(stable, definition.ID)
		}
	}
	// A snapshot of the import's own status tags, not a quality measure: it moves
	// whenever the curated set is re-curated by scripts/curated-pattern-labeller.js.
	// It is worth pinning because a silent change in the mix means clips were
	// added or dropped without the catalog manifest being regenerated with them.
	//
	// Motion quality is asserted separately and does not live here: every curated
	// clip is measured against the catalog budgets by
	// TestBuiltinCatalogIncludesGeneratedPatternsWithoutExactTimingExemption, and
	// against the speed envelope by TestCatalogPatternsHoldTheMeasuredSpeedEnvelope.
	wantStable := []PatternID{
		"curated-easy-drive-1", "curated-easy-drive-2", "curated-easy-drive-3",
		"curated-easy-drive-4", "curated-easy-drive-5", "curated-gentle-drive-1",
		"curated-gentle-drive-2", "curated-gentle-drive-3", "curated-steady-drive-2",
	}
	if experimental != catalog.PatternCount-len(wantStable) || !slices.Equal(stable, wantStable) {
		t.Fatalf("generated status audit = %d experimental, stable %v", experimental, stable)
	}
}

func curatedCatalogNames(t *testing.T, catalog curatedCatalogFixture) map[string]string {
	t.Helper()
	expectedNames := make(map[string]string, len(catalog.Patterns))
	for _, entry := range catalog.Patterns {
		if entry.File == "" || entry.Name == "" {
			t.Fatalf("curated catalog contains an incomplete entry: %+v", entry)
		}
		if _, exists := expectedNames[entry.File]; exists {
			t.Fatalf("curated catalog contains duplicate file %q", entry.File)
		}
		expectedNames[entry.File] = entry.Name
	}
	return expectedNames
}

func assertCuratedBuiltinDefinition(
	t *testing.T,
	definition PatternDefinition,
	expectedNames map[string]string,
	seen map[PatternID]bool,
	seenNames map[string]bool,
) bool {
	t.Helper()
	if !strings.HasPrefix(string(definition.ID), "curated-") || seen[definition.ID] {
		t.Fatalf("invalid or duplicate generated pattern id %q", definition.ID)
	}
	seen[definition.ID] = true
	filename := strings.TrimPrefix(string(definition.ID), "curated-") + ".mhpattern.json"
	expectedName, exists := expectedNames[filename]
	if !exists || definition.Name != expectedName || seenNames[definition.Name] {
		t.Fatalf("generated pattern metadata %q / %q does not match manifest", filename, definition.Name)
	}
	seenNames[definition.Name] = true
	if slices.Contains(definition.Tags, TagCurated) || slices.Contains(definition.Tags, "imported") {
		t.Fatalf("pattern %q tags = %#v, want catalog motion tags only", definition.ID, definition.Tags)
	}
	isExperimental := slices.Contains(definition.Tags, TagExperimental)
	if isExperimental != strings.HasPrefix(definition.Description, "Experimental: ") {
		t.Fatalf("pattern %q experimental tag/description mismatch", definition.ID)
	}
	minimumTags := 1
	if isExperimental {
		minimumTags = 2
	}
	if len(definition.Tags) < minimumTags || len(definition.Tags) > 5 {
		t.Fatalf("pattern %q tags = %#v, want status plus 1-4 motion tags", definition.ID, definition.Tags)
	}
	if definition.CycleMillis < RoutineCycleFloorMillis || definition.CycleMillis > 12_000 {
		t.Fatalf("pattern %q normalized cycle = %d, want %d-12000", definition.ID, definition.CycleMillis, RoutineCycleFloorMillis)
	}
	return isExperimental
}

func TestBuiltinCatalogIncludesGeneratedPatternsWithoutExactTimingExemption(t *testing.T) {
	want := expectedCuratedCatalog(t).PatternCount
	got := 0
	for _, definition := range BuiltinPatternDefinitions() {
		if !strings.HasPrefix(string(definition.ID), "curated-") {
			continue
		}
		got++
		if UsesExactImportedCurve(definition) {
			t.Fatalf("generated pattern %q bypasses normal timing controls", definition.ID)
		}
		metrics, err := MeasureCurve(definition.Points, definition.CycleMillis, true)
		if err != nil {
			t.Fatalf("measure %q: %v", definition.ID, err)
		}
		if exceedsCatalogSafetyBudgets(metrics) {
			t.Fatalf("generated pattern %q remains outside catalog budgets: %+v", definition.ID, metrics)
		}
	}
	if got != want {
		t.Fatalf("active generated patterns = %d, want %d", got, want)
	}
}
