package funscript

const (
	fullScriptZone     = "full"
	fullScriptTag      = "full_script"
	fullScriptIDMarker = "-full-script-"
)

// FullScriptContentHash is distinct from segment hashes so dedupe keeps the full block.
func FullScriptContentHash(actions []StoredAction) string {
	return HashBlockActions(actions) + ":full"
}

// BuildFullScriptBlockRecord builds a library block holding the entire imported funscript.
func BuildFullScriptBlockRecord(normalizedActions []Action, sourceStem, fileID string, fileMetadata map[string]any) (BlockRecord, error) {
	blocks := [][]Action{normalizedActions}
	featuresList, err := ExtractFeaturesForBlocks(blocks)
	if err != nil {
		return BlockRecord{}, err
	}
	classifications, err := ClassifyBlocks(blocks, featuresList)
	if err != nil {
		return BlockRecord{}, err
	}
	features := featuresList[0]
	classification := classifications[0]

	working, bounds := ResolveBlockBounds(normalizedActions, fileMetadata, true)
	tags := []string{fullScriptTag, "original"}
	for _, tag := range classification.Tags {
		if tag == "" || containsString(tags, tag) {
			continue
		}
		tags = append(tags, tag)
	}
	tags = append(tags, SourceRangeTag(bounds.SourceStartMS, bounds.MotionEndMS))
	tags = append(tags, "src:"+bounds.SourceRangeSlug)

	actions := StoreImportActions(working)
	record := BlockRecord{
		ID:              FullBlockRecordID(sourceStem, fileID, bounds.SourceRangeSlug),
		Features:        &features,
		StartMS:         bounds.SourceStartMS,
		EndMS:           bounds.MotionEndMS,
		SourceEndMS:     bounds.SourceEndMS,
		MotionEndMS:     bounds.MotionEndMS,
		SourceTimeRange: bounds.SourceTimeRange,
		MotionTimeRange: bounds.MotionTimeRange,
		SourceRangeSlug: bounds.SourceRangeSlug,
		DurationMS:      bounds.EffectiveDurationMS,
		MinPos:          features.MinPos,
		MaxPos:          features.MaxPos,
		AvgPos:          features.AvgPos,
		Amplitude:       features.Amplitude,
		ActionCount:     features.ActionCount,
		Zone:            fullScriptZone,
		StrokeLength:    classification.StrokeLength,
		Speed:           classification.Speed,
		Rhythm:          classification.Rhythm,
		Intensity:       classification.Intensity,
		Tags:            tags,
		Actions:         actions,
		IsFullScript:    true,
	}
	if record.StrokeLength == "" {
		record.StrokeLength = "full"
	}
	if record.Speed == "" {
		record.Speed = "medium"
	}
	record.SemanticSummary = SemanticSummaryFromRecord(record)
	return record, nil
}

// IsFullScriptBlock reports whether a block record represents the full script.
func IsFullScriptBlock(blockID, zone string, tags []string) bool {
	if stringsContains(blockID, fullScriptIDMarker) {
		return true
	}
	if zone == fullScriptZone {
		for _, tag := range tags {
			if tag == fullScriptTag {
				return true
			}
		}
	}
	return false
}

func stringsContains(text, needle string) bool {
	return len(text) >= len(needle) && indexOf(text, needle) >= 0
}

func indexOf(text, needle string) int {
	for i := 0; i+len(needle) <= len(text); i++ {
		if text[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
