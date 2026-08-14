package motion

// generateHardAndRegularPattern keeps the accepted 0 -> 100 -> 74 -> 0 beat,
// but makes the partial return brief enough to remain motion after normal pace
// retiming. The imported source gave the 26-point accent almost as much time as
// the preceding 100-point stroke. That worked only at its unusually fast source
// cadence; at a normal 40% pace it became a one-second crawl after every apex.
func generateHardAndRegularPattern() PatternDefinition {
	const (
		beats          = 16
		beatMillis     = int64(450)
		upMillis       = int64(210)
		accentMillis   = int64(70)
		accentPosition = 74.24242424242425
	)
	points := make([]CurvePoint, 0, beats*3+1)
	points = append(points, CurvePoint{PositionPercent: 0})
	for beat := range beats {
		start := int64(beat) * beatMillis
		points = append(points,
			CurvePoint{TimeMillis: start + upMillis, PositionPercent: 100},
			CurvePoint{TimeMillis: start + upMillis + accentMillis, PositionPercent: accentPosition},
			CurvePoint{TimeMillis: start + beatMillis, PositionPercent: 0},
		)
	}
	return mustNormalizeCatalog(PatternDefinition{
		ID: PatternHardAndRegular, Name: "Hard and Regular",
		Description: "Full-range strokes with a brief partial return accent on each beat.",
		Kind:        PatternKindRoutine, CycleMillis: beats * beatMillis, Points: points,
		Tags: []string{"full-span", "regular", "return-accent", TagCurated},
	})
}

// promotedBuiltinPatterns preserve user-selected geometry in the built-in
// catalog. Normal runtime speed retiming and the global motion envelope still
// apply in the shared engine. Only playful jerk retains exact imported holds;
// Hard and Regular uses the velocity-balanced timing above.
var promotedBuiltinPatterns = []PatternDefinition{
	generateHardAndRegularPattern(),
	mustNormalizeCatalog(PatternDefinition{
		ID: PatternPlayfulJerk, Name: "playful jerk",
		Description: "Staggered full-range accents shift from short midpoint holds into longer sweeps.",
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
		Tags: []string{"syncopated", "full-span", "midpoint-holds", "tempo-ramp", TagCurated},
	}),
}

var retiredBuiltinPatternIDs = []PatternID{
	PatternCradle,
	PatternTopAnchoredDepths,
	PatternDeepBookends,
	PatternOneDeepThreeShallow,
	PatternLowerMidrangeMix,
	PatternMidTopSwitch,
	PatternMidrangeFullFinish,

	// Retired after the user disabled all fifteen by hand. Measuring them found
	// two failure modes, both traceable to the same authoring gap: positions and
	// travel times were independent lists, so stroke velocity was never a
	// designed quantity.
	//
	// Five stalled outright -- the rendered curve spends a contiguous span under
	// 30%/s, up to 2.46s of a 6.6s loop in Cascade, because a shrinking stroke
	// kept a fixed half-period. Ten more never settled into a pace: their slowest
	// stroke averaged 33%/s against 62%/s across the patterns that were kept, and
	// they used 5.5 distinct stroke lengths against 3.0.
	//
	// Replacements are authored from velocity and live in catalogPatternSpecs.
	PatternCascade,
	PatternSurge,
	PatternDescendingLadder,
	PatternPendulum,
	PatternDeepMediumShortPairs,
	PatternClimb,
	PatternWaves,
	PatternWanderingSwell,
	PatternSway,
	PatternRolling,
	PatternFallingCrest,
	PatternDoubleTap,
	PatternShortMediumSteps,
	PatternSyncopate,
	PatternThreeDeepOneShort,
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
