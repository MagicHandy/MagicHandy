package store

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/personabuiltin"
)

func TestMigrateV8SeedsClarissaPersona(t *testing.T) {
	t.Parallel()
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	rows, err := db.ListPersonas()
	if err != nil {
		t.Fatalf("ListPersonas: %v", err)
	}
	var found bool
	for _, row := range rows {
		if row.ID == personabuiltin.ClarissaSynsualID {
			found = true
			if row.Name != personabuiltin.ClarissaSynsualName {
				t.Fatalf("name = %q, want %q", row.Name, personabuiltin.ClarissaSynsualName)
			}
			if row.SystemPrompt != personabuiltin.ClarissaSynsualSystem {
				t.Fatal("unexpected clarissa system prompt")
			}
		}
	}
	if !found {
		t.Fatalf("expected persona %q after migration v8", personabuiltin.ClarissaSynsualID)
	}

	db2, err := Open(db.DataDir())
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()
	clarissaCount := 0
	rows2, err := db2.ListPersonas()
	if err != nil {
		t.Fatalf("ListPersonas reopen: %v", err)
	}
	for _, row := range rows2 {
		if row.ID == personabuiltin.ClarissaSynsualID {
			clarissaCount++
		}
	}
	if clarissaCount != 1 {
		t.Fatalf("clarissa rows = %d, want 1", clarissaCount)
	}
}
