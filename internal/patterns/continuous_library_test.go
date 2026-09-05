package patterns

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestDeprecatedBuiltinsRemainPlayableButCannotBeCurated(t *testing.T) {
	dir := t.TempDir()
	library, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := library.Pattern(string(motion.PatternStroke))
	if err != nil {
		t.Fatal(err)
	}
	if !legacy.Deprecated || !legacy.Enabled {
		t.Fatal("legacy compatibility lost")
	}
	definition, found, err := library.ResolveEnabled(legacy.ID)
	if err != nil || !found || definition.ID != motion.PatternStroke {
		t.Fatal("legacy manual playback unavailable")
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
	name, weight := "My previous motion", 1.7
	if _, err := library.UpdatePattern(legacy.ID, PatternPatch{Name: &name, Weight: &weight}); err != nil {
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
	if err != nil || legacy.Name != name || legacy.Weight != weight || !legacy.Enabled || !legacy.Deprecated {
		t.Fatal("deprecation reset saved preferences")
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
