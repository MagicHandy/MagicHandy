package library

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/funscript"
)

// PersistResult reports what was written during funscript ingest persistence.
type PersistResult struct {
	FileID                   string   `json:"file_id"`
	FullBlockID              string   `json:"full_block_id"`
	BlocksInserted           int      `json:"blocks_inserted"`
	BlocksSkippedDuplicate   int      `json:"blocks_skipped_duplicate"`
	BlocksSkippedContentHash int      `json:"blocks_skipped_content_hash"`
	InsertedBlockIDs         []string `json:"inserted_block_ids"`
	SourceFormat             string   `json:"source_format,omitempty"`
}

// Service wraps Store with higher-level library operations.
type Service struct {
	store *Store
}

// NewService returns a library service over an open store.
func NewService(store *Store) *Service {
	return &Service{store: store}
}

// Store returns the underlying SQLite store.
func (s *Service) Store() *Store {
	return s.store
}

// PersistIngestResult upserts the imported file and inserts motion blocks.
func (s *Service) PersistIngestResult(ctx context.Context, result funscript.IngestResult) (PersistResult, error) {
	if s == nil || s.store == nil {
		return PersistResult{}, fmt.Errorf("library store is unavailable")
	}

	sourceStem := funscript.ImportSourceStem(result.Source.Filename)
	segmentBlocks := make([]funscript.BlockRecord, 0, len(result.Blocks))
	for _, block := range result.Blocks {
		if !block.IsFullScript && !funscript.IsFullScriptBlock(block.ID, block.Zone, block.Tags) {
			segmentBlocks = append(segmentBlocks, block)
		}
	}

	existing, existingErr := s.store.GetFunscriptFileByHash(ctx, result.Source.Hash)
	reimport := existingErr == nil

	fileMeta := cloneMetadata(result.Metadata)
	if scriptNumber, ok := scriptNumberFromMetadata(fileMeta); ok {
		_ = scriptNumber
	} else if reimport && len(existing.Metadata) > 0 {
		if n, ok := scriptNumberFromMetadata(existing.Metadata); ok {
			fileMeta["script_number"] = n
		}
	}
	if _, ok := fileMeta["script_number"]; !ok {
		fileMeta["script_number"] = s.store.nextScriptNumber(ctx)
	}

	file := FunscriptFile{
		Filename:     result.Source.Filename,
		Path:         result.Source.Path,
		DurationMS:   result.Summary.DurationMS,
		ActionCount:  result.Summary.ActionCount,
		ImportedAt:   result.Source.ImportedAt,
		Hash:         result.Source.Hash,
		SourceFormat: string(result.Source.SourceFormat),
		Metadata:     fileMeta,
		Actions:      result.NormalizedActions,
	}
	if reimport {
		file.ID = existing.ID
		if err := s.store.UpdateFunscriptFile(ctx, file); err != nil {
			return PersistResult{}, err
		}
		if _, err := s.store.DeleteMotionBlocksByFileID(ctx, file.ID); err != nil {
			return PersistResult{}, err
		}
	} else {
		created, err := s.store.CreateFunscriptFile(ctx, file)
		if err != nil {
			return PersistResult{}, err
		}
		file = created
	}

	fullRecord, err := funscript.BuildFullScriptBlockRecord(
		actionsFromStored(result.NormalizedActions),
		sourceStem,
		file.ID,
		fileMeta,
	)
	if err != nil {
		return PersistResult{}, err
	}
	fullBlockID := fullRecord.ID
	result.Summary.FullScriptBlockID = fullBlockID

	knownHashes, err := s.store.ListMotionBlockContentHashes(ctx)
	if err != nil {
		return PersistResult{}, err
	}
	seenHashes := make(map[string]struct{}, len(knownHashes))
	for _, hash := range knownHashes {
		if hash != "" {
			seenHashes[hash] = struct{}{}
		}
	}

	out := PersistResult{
		FileID:       file.ID,
		FullBlockID:  fullBlockID,
		SourceFormat: string(result.Source.SourceFormat),
	}
	insertedIDs := make([]string, 0, len(segmentBlocks)+1)

	if inserted, err := s.insertBlockRecord(ctx, file.ID, fullRecord, seenHashes); err != nil {
		return PersistResult{}, err
	} else if inserted {
		out.BlocksInserted++
		insertedIDs = append(insertedIDs, fullRecord.ID)
	} else {
		out.BlocksSkippedContentHash++
	}

	for _, record := range segmentBlocks {
		inserted, err := s.insertBlockRecord(ctx, file.ID, record, seenHashes)
		if err != nil {
			return PersistResult{}, err
		}
		if inserted {
			out.BlocksInserted++
			insertedIDs = append(insertedIDs, record.ID)
			continue
		}
		existingBlock, err := s.store.GetMotionBlock(ctx, record.ID)
		if err == nil && existingBlock.SourceFileID == file.ID {
			out.BlocksSkippedDuplicate++
			continue
		}
		out.BlocksSkippedContentHash++
	}

	out.InsertedBlockIDs = insertedIDs
	return out, nil
}

func (s *Service) insertBlockRecord(
	ctx context.Context,
	fileID string,
	record funscript.BlockRecord,
	seenHashes map[string]struct{},
) (bool, error) {
	contentHash := record.ContentHash
	if contentHash == "" {
		contentHash = funscript.HashBlockActions(record.Actions)
	}
	if record.IsFullScript || funscript.IsFullScriptBlock(record.ID, record.Zone, record.Tags) {
		contentHash = funscript.FullScriptContentHash(record.Actions)
	}
	if _, exists := seenHashes[contentHash]; exists {
		return false, nil
	}
	seenHashes[contentHash] = struct{}{}

	block := MotionBlock{
		ID:              record.ID,
		SourceFileID:    fileID,
		StartMS:         record.StartMS,
		EndMS:           record.EndMS,
		DurationMS:      record.DurationMS,
		MinPos:          record.MinPos,
		MaxPos:          record.MaxPos,
		AvgPos:          record.AvgPos,
		Amplitude:       record.Amplitude,
		Zone:            record.Zone,
		StrokeLength:    record.StrokeLength,
		Speed:           record.Speed,
		Rhythm:          record.Rhythm,
		Intensity:       record.Intensity,
		Tags:            append([]string(nil), record.Tags...),
		Actions:         record.Actions,
		ContentHash:     contentHash,
		SemanticSummary: record.SemanticSummary,
	}
	if _, err := s.store.CreateMotionBlock(ctx, block); err != nil {
		return false, err
	}
	return true, nil
}

func actionsFromStored(stored []funscript.StoredAction) []funscript.Action {
	out := make([]funscript.Action, len(stored))
	for i, action := range stored {
		out[i] = funscript.Action{At: action.At, Pos: action.Pos}
	}
	return out
}

func cloneMetadata(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func scriptNumberFromMetadata(metadata map[string]any) (int, bool) {
	if metadata == nil {
		return 0, false
	}
	raw, ok := metadata["script_number"]
	if !ok {
		return 0, false
	}
	switch value := raw.(type) {
	case int:
		return value, value > 0
	case int64:
		return int(value), value > 0
	case float64:
		return int(value), value > 0
	default:
		return 0, false
	}
}

// ApplyBlockFeedback adjusts success score and favorite/blocked flags.
func (s *Service) ApplyBlockFeedback(ctx context.Context, blockID, feedback string) (MotionBlock, error) {
	if s == nil || s.store == nil {
		return MotionBlock{}, fmt.Errorf("library store is unavailable")
	}
	return s.store.applyBlockFeedback(ctx, blockID, feedback)
}

// ImportDisplayFilename strips temp upload stems for UI labels.
func ImportDisplayFilename(filename string) string {
	stem := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	if funscript.ImportSourceStem(filename) == "import" {
		return "Imported script"
	}
	return stem
}
