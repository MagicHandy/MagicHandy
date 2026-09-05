package chat

import (
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestEveryContinuousRecipeHasDistinctRoundTripHandles(t *testing.T) {
	for _, method := range []string{"library", "library_descriptive", "library_actions"} {
		seen := map[string]bool{}
		for _, recipe := range motion.ContinuousRecipes(25) {
			handle := libraryLabHandle(method, recipe.ID)
			if strings.TrimSpace(handle) == "" || seen[handle] {
				t.Fatalf("%s duplicate/empty handle for %s", method, recipe.ID)
			}
			seen[handle] = true
			_, spec, _, err := ParseLLMLab(`{"reply":"Apply this shape.","recipe_id":"`+handle+`"}`, method, motion.DefaultFlowSpec(), config.DefaultSettings().Motion)
			if err != nil {
				t.Fatal(err)
			}
			_, name := labCurrentRecipe(method, spec)
			if name != recipe.Name {
				t.Fatalf("%s selected %s instead of %s", handle, name, recipe.Name)
			}
		}
	}
}
