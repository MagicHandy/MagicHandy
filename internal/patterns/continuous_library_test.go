package patterns

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestDeprecatedBuiltinsAreDisabledOnUpgrade(t *testing.T) {
	dir := t.TempDir()
	library, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := library.Pattern(string(motion.PatternStroke))
	if err != nil {
		t.Fatal(err)
	}
	name, weight := "My previous motion", 1.7
	if _, err := library.UpdatePattern(legacy.ID, PatternPatch{Name: &name, Weight: &weight}); err != nil {
		t.Fatal(err)
	}
	disabled := false
	if _, err := library.UpdatePattern(string(motion.PatternFullSweeps), PatternPatch{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	// Simulate a database written by the previous release, including an
	// explicitly enabled legacy row. Startup must migrate existing installs.
	if _, err := library.db.SQL().Exec(`UPDATE patterns SET enabled = 1 WHERE origin = 'builtin' AND id NOT LIKE 'flow-%'`); err != nil {
		t.Fatal(err)
	}
	if err := library.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	legacy, err = reopened.Pattern(legacy.ID)
	if err != nil || legacy.Name != name || legacy.Weight != weight || legacy.Enabled || !legacy.Deprecated {
		t.Fatal("deprecation reset saved preferences")
	}
	current, err := reopened.Pattern(string(motion.PatternFullSweeps))
	if err != nil || current.Enabled {
		t.Fatal("upgrade reset a new recipe's saved disablement")
	}
	for _, definition := range motion.BuiltinPatternDefinitions() {
		row, err := reopened.Pattern(string(definition.ID))
		if err != nil || (row.Deprecated && row.Enabled) {
			t.Fatal("upgrade left a deprecated builtin enabled", definition.ID)
		}
	}
}

func TestContinuousCatalogExportsImportableSampledBakes(t *testing.T) {
	library, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = library.Close() })
	for _, recipe := range motion.ContinuousRecipes(25) {
		data, filename, err := library.ExportPattern(string(recipe.ID))
		if err != nil {
			t.Fatal(err)
		}
		imported, err := library.Import(filename, data, importAsPattern)
		if err != nil || imported.Pattern == nil {
			t.Fatalf("%s export could not be imported: %v", recipe.ID, err)
		}
		if imported.Pattern.Continuous || imported.Pattern.Origin == OriginBuiltin || len(imported.Pattern.Points) < 3 {
			t.Fatal("imported sampled bake claimed the private continuous recipe")
		}
	}
}

func TestDeprecatedBuiltinsCannotBeEnabledPlayedOrCurated(t *testing.T) {
	library, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = library.Close() })
	legacy, err := library.Pattern(string(motion.PatternStroke))
	if err != nil {
		t.Fatal(err)
	}
	if !legacy.Deprecated || legacy.Enabled {
		t.Fatal("legacy builtin must be disabled")
	}
	_, found, err := library.ResolveEnabled(legacy.ID)
	if err != nil || found {
		t.Fatal("legacy manual playback must be unavailable")
	}
	enabled := true
	if _, err := library.UpdatePattern(legacy.ID, PatternPatch{Enabled: &enabled}); err == nil {
		t.Fatal("legacy builtin was re-enabled")
	}
	choices, err := library.EnabledChoices()
	if err != nil {
		t.Fatal(err)
	}
	if len(choices) != len(motion.ContinuousRecipes(25)) {
		t.Fatalf("model catalog has %d choices", len(choices))
	}
	for _, choice := range choices {
		if _, ok := motion.ContinuousRecipeByID(motion.PatternID(choice.ID), 25); !ok {
			t.Fatalf("legacy choice leaked: %s", choice.ID)
		}
	}
}
