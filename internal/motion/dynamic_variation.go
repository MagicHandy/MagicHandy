package motion

import "math/rand"

// FreshDynamicPhrase changes only the realization of an explicitly authored
// Creative phrase. Normalization and saved-target replay stay deterministic;
// speed-only edits retain the active phrase. No sample-time randomness is used.
func FreshDynamicPhrase(definition DynamicDefinition, previous uint32) DynamicDefinition {
	definition = NormalizeDynamicDefinition(definition)
	if !dynamicUsesSeededTexture(definition) {
		return definition
	}
	seed := previous
	for seed == 0 || seed == previous {
		// #nosec G404 -- Non-security motion variation; semantic bounds remain authoritative.
		seed = rand.Uint32()
	}
	definition.PhraseSeed = seed
	return definition
}
