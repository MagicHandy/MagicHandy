package funscript

import (
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Ingest runs the full funscript pipeline on a loaded script.
func Ingest(loaded LoadedFunscript, originalFilename string) (IngestResult, error) {
	return IngestLoadedFunscript(loaded, originalFilename)
}

// IngestLoadedFunscript runs the pipeline on a LoadedFunscript.
func IngestLoadedFunscript(loaded LoadedFunscript, originalFilename string) (IngestResult, error) {
	displayFilename := originalFilename
	if displayFilename == "" {
		displayFilename = filepath.Base(loaded.SourcePath)
	}
	if displayFilename == "" {
		displayFilename = "upload"
	}
	sourceStem := stringsTrimSuffix(filepath.Base(displayFilename), filepath.Ext(displayFilename))

	raw := make([]map[string]any, len(loaded.Actions))
	for i, action := range loaded.Actions {
		raw[i] = map[string]any{"at": action.At, "pos": action.Pos}
	}
	validated, err := ValidateActions(raw)
	if err != nil {
		return IngestResult{}, err
	}
	prepared := NormalizeActions(validated)
	if len(prepared) < 2 {
		return IngestResult{}, ErrNotEnoughActions
	}

	importedActions := StoreImportActions(prepared)
	blocks := SegmentActions(prepared)
	if len(blocks) == 0 {
		blocks = [][]Action{prepared}
	}

	featuresList, err := ExtractFeaturesForBlocks(blocks)
	if err != nil {
		return IngestResult{}, err
	}
	classifications, err := ClassifyBlocks(blocks, featuresList)
	if err != nil {
		return IngestResult{}, err
	}

	blockRecords := buildBlockRecords(blocks, featuresList, classifications, sourceStem)
	acceptedSegments, rejectedSegments := FilterSegmentRecords(blockRecords)

	fullBlock, err := BuildFullScriptBlockRecord(prepared, sourceStem, "", loaded.Metadata)
	if err != nil {
		return IngestResult{}, err
	}
	blockRecords = append([]BlockRecord{fullBlock}, acceptedSegments...)

	durationMS := EffectiveScriptDurationMS(prepared, loaded.Metadata)
	importedAt := time.Now().UTC().Format(time.RFC3339)

	sourcePath := loaded.SourcePath
	resolvedPath := sourcePath
	if sourcePath != "" {
		if abs, err := filepath.Abs(sourcePath); err == nil {
			resolvedPath = abs
		}
	}

	hash, err := fileHash(sourcePath, importedActions)
	if err != nil {
		return IngestResult{}, err
	}

	return IngestResult{
		Source: IngestSource{
			Filename:     displayFilename,
			Path:         resolvedPath,
			Hash:         hash,
			ImportedAt:   importedAt,
			SourceFormat: loaded.SourceFormat,
		},
		Metadata:          cloneMap(loaded.Metadata),
		ExtraFields:       cloneMap(loaded.ExtraFields),
		NormalizedActions: importedActions,
		ImportedActions:   importedActions,
		Summary: IngestSummary{
			ActionCount:        len(prepared),
			DurationMS:         durationMS,
			BlockCount:         len(acceptedSegments),
			FullScriptBlockID:  fullBlock.ID,
			QARejectedSegments: rejectedSegments,
		},
		Blocks: blockRecords,
	}, nil
}

// IngestFunscriptFile runs the full funscript pipeline on a file path.
func IngestFunscriptFile(filePath string, originalFilename string) (IngestResult, error) {
	loaded, err := LoadActionsFromPath(filePath)
	if err != nil {
		return IngestResult{}, err
	}
	return IngestLoadedFunscript(loaded, originalFilename)
}

func buildBlockRecords(blocks [][]Action, featuresList []BlockFeatures, classifications []Classification, sourceStem string) []BlockRecord {
	records := make([]BlockRecord, 0, len(blocks))
	for index, block := range blocks {
		features := featuresList[index]
		classification := classifications[index]
		working, bounds := ResolveBlockBounds(block, nil, false)

		tags := append([]string(nil), classification.Tags...)
		tags = append(tags, SourceRangeTag(bounds.SourceStartMS, bounds.MotionEndMS))
		tags = append(tags, "src:"+bounds.SourceRangeSlug)

		stored := StoreImportActions(working)
		featuresCopy := features
		record := BlockRecord{
			ID: BlockRecordID(
				sourceStem,
				index+1,
				classification.Zone,
				classification.Speed,
				bounds.SourceRangeSlug,
			),
			Features:        &featuresCopy,
			StartMS:           bounds.SourceStartMS,
			EndMS:             bounds.MotionEndMS,
			SourceEndMS:       bounds.SourceEndMS,
			MotionEndMS:       bounds.MotionEndMS,
			SourceTimeRange:   bounds.SourceTimeRange,
			MotionTimeRange:   bounds.MotionTimeRange,
			SourceRangeSlug:   bounds.SourceRangeSlug,
			DurationMS:        bounds.EffectiveDurationMS,
			MinPos:            features.MinPos,
			MaxPos:            features.MaxPos,
			AvgPos:            features.AvgPos,
			Amplitude:         features.Amplitude,
			ActionCount:       len(stored),
			Zone:              classification.Zone,
			StrokeLength:      classification.StrokeLength,
			Speed:             classification.Speed,
			Rhythm:            classification.Rhythm,
			Intensity:         classification.Intensity,
			Tags:              tags,
			Actions:           stored,
		}
		record.SemanticSummary = SemanticSummaryFromRecord(record)
		records = append(records, record)
	}
	return records
}

func fileHash(path string, importedActions []StoredAction) (string, error) {
	if path != "" {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			file, err := os.Open(path) // #nosec G304 -- ingest source path.
			if err != nil {
				return "", err
			}
			defer file.Close()
			hasher := sha256.New()
			if _, err := io.Copy(hasher, file); err != nil {
				return "", err
			}
			return fmtHex(hasher.Sum(nil)), nil
		}
	}
	payload, err := json.Marshal(importedActions)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return fmtHex(sum[:]), nil
}

func cloneMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func stringsTrimSuffix(text, suffix string) string {
	if len(suffix) == 0 {
		return text
	}
	if len(text) >= len(suffix) && text[len(text)-len(suffix):] == suffix {
		return text[:len(text)-len(suffix)]
	}
	return text
}
