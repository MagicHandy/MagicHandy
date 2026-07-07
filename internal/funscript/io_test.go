package funscript

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateAndNormalize(t *testing.T) {
	payload := []map[string]any{
		{"at": 1000, "pos": 50},
		{"at": 0, "pos": 0},
		{"at": 1000, "pos": 50},
		{"at": 2000, "pos": 100},
	}
	if _, err := ValidateActions(payload); err != nil {
		t.Fatalf("ValidateActions: %v", err)
	}
	validated, err := ValidateActions(payload)
	if err != nil {
		t.Fatalf("ValidateActions: %v", err)
	}
	actions := NormalizeActions(validated)
	if actions[0].At != 0 {
		t.Fatalf("first action at = %d, want 0", actions[0].At)
	}
	if len(actions) != 3 {
		t.Fatalf("normalized actions = %d, want 3", len(actions))
	}
}

func TestLoadAndIngestFixtures(t *testing.T) {
	root := filepath.Join("testdata")
	cases := []struct {
		name   string
		file   string
		format SourceFormat
	}{
		{"funscript", "sample.funscript", SourceFormatFunscript},
		{"csv", "sample.csv", SourceFormatCSV},
		{"json", "sample.json", SourceFormatJSON},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			loaded, err := LoadActionsFromPath(path)
			if err != nil {
				t.Fatalf("LoadActionsFromPath: %v", err)
			}
			if loaded.SourceFormat != tc.format {
				t.Fatalf("source format = %q, want %q", loaded.SourceFormat, tc.format)
			}
			result, err := IngestFunscriptFile(path, "")
			if err != nil {
				t.Fatalf("IngestFunscriptFile: %v", err)
			}
			if result.Summary.ActionCount < 2 {
				t.Fatalf("action count = %d, want >= 2", result.Summary.ActionCount)
			}
			if len(result.Blocks) == 0 {
				t.Fatal("expected at least one block")
			}
			if result.Source.SourceFormat != tc.format {
				t.Fatalf("ingest source format = %q, want %q", result.Source.SourceFormat, tc.format)
			}
		})
	}
}

func TestInvalidFunscriptRaises(t *testing.T) {
	_, err := ValidateActions([]map[string]any{
		{"at": "bad", "pos": 1},
		{"at": 1000, "pos": 50},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestHashBlockActionsStable(t *testing.T) {
	actions := []StoredAction{{At: 0, Pos: 10}, {At: 500, Pos: 90}}
	first := HashBlockActions(actions)
	second := HashBlockActions(actions)
	if first != second || len(first) != 64 {
		t.Fatalf("hash = %q, want stable 64-char sha256", first)
	}
}

func TestLoadActionsFromTextUpload(t *testing.T) {
	text, err := os.ReadFile(filepath.Join("testdata", "sample.funscript"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	loaded, err := LoadActionsFromText(string(text), "upload.funscript")
	if err != nil {
		t.Fatalf("LoadActionsFromText: %v", err)
	}
	if len(loaded.Actions) < 2 {
		t.Fatalf("actions = %d, want >= 2", len(loaded.Actions))
	}
}
