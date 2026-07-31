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

func expectedCuratedPatternCount(t *testing.T) int {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	catalogPath := filepath.Join(filepath.Dir(file), "builtinpatterns", "curated", "_catalog.json")
	data, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("read curated catalog: %v", err)
	}
	var catalog struct {
		PatternCount int `json:"pattern_count"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("decode curated catalog: %v", err)
	}
	if catalog.PatternCount <= 0 {
		t.Fatalf("curated catalog pattern_count = %d, want > 0", catalog.PatternCount)
	}
	return catalog.PatternCount
}

func TestCuratedBuiltinPatternsLoad(t *testing.T) {
	expected := expectedCuratedPatternCount(t)
	definitions := loadCuratedBuiltinPatterns()
	if len(definitions) != expected {
		t.Fatalf("curated catalog size = %d, want %d", len(definitions), expected)
	}
	seen := make(map[PatternID]bool, len(definitions))
	seenNames := make(map[string]bool, len(definitions))
	for _, definition := range definitions {
		if !strings.HasPrefix(string(definition.ID), "curated-") {
			t.Fatalf("pattern %q id must use curated- prefix", definition.ID)
		}
		if seen[definition.ID] {
			t.Fatalf("duplicate curated pattern id %q", definition.ID)
		}
		seen[definition.ID] = true
		if seenNames[definition.Name] {
			t.Fatalf("duplicate curated pattern name %q", definition.Name)
		}
		seenNames[definition.Name] = true
		if slices.Contains(definition.Tags, TagCurated) || slices.Contains(definition.Tags, "imported") {
			t.Fatalf("pattern %q tags = %#v, want catalog motion tags only", definition.ID, definition.Tags)
		}
		if len(definition.Tags) == 0 || len(definition.Tags) > 4 {
			t.Fatalf("pattern %q tags = %#v, want 1-4 catalog tags", definition.ID, definition.Tags)
		}
		if definition.CycleMillis < RoutineCycleFloorMillis {
			t.Fatalf("pattern %q cycle = %d, below routine floor", definition.ID, definition.CycleMillis)
		}
		if definition.CycleMillis > 10_500 {
			t.Fatalf("pattern %q cycle = %d, above 10.5s clip budget", definition.ID, definition.CycleMillis)
		}
	}
}

func TestBuiltinCatalogIncludesCuratedPatterns(t *testing.T) {
	expected := expectedCuratedPatternCount(t)
	curated := 0
	for _, definition := range BuiltinPatternDefinitions() {
		if strings.HasPrefix(string(definition.ID), "curated-") {
			curated++
			if !UsesExactImportedCurve(definition) {
				t.Fatalf("curated pattern %q not marked exact imported", definition.ID)
			}
		}
	}
	if curated != expected {
		t.Fatalf("builtin catalog curated count = %d, want %d", curated, expected)
	}
}
