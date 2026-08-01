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
	Schema       string `json:"schema"`
	Quarantined  bool   `json:"quarantined"`
	PatternCount int    `json:"pattern_count"`
	Patterns     []struct {
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
	if catalog.Schema != "magichandy.quarantined-pattern-catalog.v2" || !catalog.Quarantined {
		t.Fatalf("curated catalog must be explicitly quarantined: %+v", catalog)
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
		filename := strings.TrimPrefix(string(definition.ID), "curated-") + ".mhpattern.json"
		expectedName, exists := expectedNames[filename]
		if !exists {
			t.Fatalf("curated file %q is absent from the quarantine catalog", filename)
		}
		if definition.Name != expectedName {
			t.Fatalf("curated file %q name = %q, catalog has %q", filename, definition.Name, expectedName)
		}
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

func TestBuiltinCatalogQuarantinesBulkImportedPatterns(t *testing.T) {
	for _, definition := range BuiltinPatternDefinitions() {
		if strings.HasPrefix(string(definition.ID), "curated-") {
			t.Fatalf("unsafe bulk-imported pattern %q remained enabled", definition.ID)
		}
	}
}
