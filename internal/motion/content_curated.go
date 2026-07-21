package motion

// promotedBuiltinPatterns preserve the timing of two user-tested imported
// curves. Unlike generated catalog content, these are not time-fitted because
// changing their cadence would change the behavior that was accepted in live
// use. They still run through the shared engine and global motion envelope.
var promotedBuiltinPatterns = []PatternDefinition{
	mustNormalizeCatalog(PatternDefinition{
		ID: PatternHardAndRegular, Name: "Hard and Regular",
		Description: "Fast full-range strokes with a brief eased return on each beat.",
		Kind:        PatternKindRoutine, CycleMillis: 7333,
		Points: []CurvePoint{
			{TimeMillis: 0, PositionPercent: 0},
			{TimeMillis: 166, PositionPercent: 100},
			{TimeMillis: 333, PositionPercent: 74.24242424242425},
			{TimeMillis: 458, PositionPercent: 0},
			{TimeMillis: 625, PositionPercent: 100},
			{TimeMillis: 791, PositionPercent: 74.24242424242425},
			{TimeMillis: 916, PositionPercent: 0},
			{TimeMillis: 1083, PositionPercent: 100},
			{TimeMillis: 1250, PositionPercent: 74.24242424242425},
			{TimeMillis: 1375, PositionPercent: 0},
			{TimeMillis: 1541, PositionPercent: 100},
			{TimeMillis: 1708, PositionPercent: 74.24242424242425},
			{TimeMillis: 1833, PositionPercent: 0},
			{TimeMillis: 2000, PositionPercent: 100},
			{TimeMillis: 2166, PositionPercent: 74.24242424242425},
			{TimeMillis: 2291, PositionPercent: 0},
			{TimeMillis: 2458, PositionPercent: 100},
			{TimeMillis: 2625, PositionPercent: 74.24242424242425},
			{TimeMillis: 2750, PositionPercent: 0},
			{TimeMillis: 2916, PositionPercent: 100},
			{TimeMillis: 3083, PositionPercent: 74.24242424242425},
			{TimeMillis: 3208, PositionPercent: 0},
			{TimeMillis: 3375, PositionPercent: 100},
			{TimeMillis: 3541, PositionPercent: 74.24242424242425},
			{TimeMillis: 3666, PositionPercent: 0},
			{TimeMillis: 3833, PositionPercent: 100},
			{TimeMillis: 4000, PositionPercent: 74.24242424242425},
			{TimeMillis: 4125, PositionPercent: 0},
			{TimeMillis: 4291, PositionPercent: 100},
			{TimeMillis: 4458, PositionPercent: 74.24242424242425},
			{TimeMillis: 4583, PositionPercent: 0},
			{TimeMillis: 4750, PositionPercent: 100},
			{TimeMillis: 4916, PositionPercent: 74.24242424242425},
			{TimeMillis: 5041, PositionPercent: 0},
			{TimeMillis: 5208, PositionPercent: 100},
			{TimeMillis: 5375, PositionPercent: 74.24242424242425},
			{TimeMillis: 5500, PositionPercent: 0},
			{TimeMillis: 5666, PositionPercent: 100},
			{TimeMillis: 5833, PositionPercent: 74.24242424242425},
			{TimeMillis: 5958, PositionPercent: 0},
			{TimeMillis: 6125, PositionPercent: 100},
			{TimeMillis: 6291, PositionPercent: 74.24242424242425},
			{TimeMillis: 6416, PositionPercent: 0},
			{TimeMillis: 6583, PositionPercent: 100},
			{TimeMillis: 6750, PositionPercent: 74.24242424242425},
			{TimeMillis: 6875, PositionPercent: 0},
			{TimeMillis: 7041, PositionPercent: 100},
			{TimeMillis: 7208, PositionPercent: 74.24242424242425},
			{TimeMillis: 7333, PositionPercent: 0},
		},
		Tags: []string{TagCurated, "fast", "full", "regular", "accented"},
	}),
	mustNormalizeCatalog(PatternDefinition{
		ID: PatternPlayfulJerk, Name: "playful jerk",
		Description: "Staggered full-range accents shift from quick pauses into longer sweeps.",
		Kind:        PatternKindRoutine, CycleMillis: 11704,
		Points: []CurvePoint{
			{TimeMillis: 0, PositionPercent: 0},
			{TimeMillis: 434, PositionPercent: 50},
			{TimeMillis: 534, PositionPercent: 50},
			{TimeMillis: 867, PositionPercent: 100},
			{TimeMillis: 1067, PositionPercent: 50},
			{TimeMillis: 1167, PositionPercent: 50},
			{TimeMillis: 1534, PositionPercent: 0},
			{TimeMillis: 2067, PositionPercent: 50},
			{TimeMillis: 2168, PositionPercent: 50},
			{TimeMillis: 2434, PositionPercent: 100},
			{TimeMillis: 2634, PositionPercent: 50},
			{TimeMillis: 2734, PositionPercent: 50},
			{TimeMillis: 3234, PositionPercent: 0},
			{TimeMillis: 3701, PositionPercent: 50},
			{TimeMillis: 3801, PositionPercent: 50},
			{TimeMillis: 3968, PositionPercent: 100},
			{TimeMillis: 4135, PositionPercent: 50},
			{TimeMillis: 4368, PositionPercent: 0},
			{TimeMillis: 4635, PositionPercent: 50},
			{TimeMillis: 4702, PositionPercent: 50},
			{TimeMillis: 4868, PositionPercent: 100},
			{TimeMillis: 5068, PositionPercent: 50},
			{TimeMillis: 5302, PositionPercent: 0},
			{TimeMillis: 5502, PositionPercent: 50},
			{TimeMillis: 5568, PositionPercent: 50},
			{TimeMillis: 5735, PositionPercent: 100},
			{TimeMillis: 6069, PositionPercent: 0},
			{TimeMillis: 6735, PositionPercent: 100},
			{TimeMillis: 7769, PositionPercent: 0},
			{TimeMillis: 8836, PositionPercent: 100},
			{TimeMillis: 9736, PositionPercent: 0},
			{TimeMillis: 10837, PositionPercent: 100},
			{TimeMillis: 11704, PositionPercent: 0},
		},
		Tags: []string{TagCurated, "syncopated", "full", "pauses", "tempo-change"},
	}),
}

var retiredBuiltinPatternIDs = []PatternID{
	PatternTopAnchoredDepths,
	PatternDeepBookends,
	PatternOneDeepThreeShallow,
	PatternLowerMidrangeMix,
	PatternMidTopSwitch,
	PatternMidrangeFullFinish,
}

// PromotedBuiltinPatternDefinitions returns defensive copies of user-tested
// curves promoted into the catalog.
func PromotedBuiltinPatternDefinitions() []PatternDefinition {
	definitions := make([]PatternDefinition, len(promotedBuiltinPatterns))
	for index, definition := range promotedBuiltinPatterns {
		definitions[index] = clonePatternDefinition(definition)
	}
	return definitions
}

// RetiredBuiltinPatternIDs returns catalog IDs removed after physical feedback.
func RetiredBuiltinPatternIDs() []PatternID {
	return append([]PatternID(nil), retiredBuiltinPatternIDs...)
}

func mustNormalizeCatalog(definition PatternDefinition) PatternDefinition {
	normalized, err := NormalizePatternDefinition(definition)
	if err != nil {
		panic(err)
	}
	return normalized
}
