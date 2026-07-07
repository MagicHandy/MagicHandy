package library

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/funscript"
)

func TestFunscriptFileAndMotionBlockCRUD(t *testing.T) { //nolint:gocyclo,funlen // integration test exercises full CRUD surface
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "library.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	file, err := store.CreateFunscriptFile(ctx, FunscriptFile{
		Filename:     "sample.funscript",
		Path:         "/tmp/sample.funscript",
		DurationMS:   1500,
		ActionCount:  4,
		Hash:         "abc123",
		SourceFormat: string(funscript.SourceFormatFunscript),
		Metadata:     map[string]any{"name": "sample"},
		Actions: []funscript.StoredAction{
			{At: 0, Pos: 10},
			{At: 500, Pos: 90},
			{At: 1000, Pos: 20},
			{At: 1500, Pos: 80},
		},
	})
	if err != nil {
		t.Fatalf("CreateFunscriptFile: %v", err)
	}
	if file.ID == "" {
		t.Fatal("expected generated file id")
	}

	gotFile, err := store.GetFunscriptFile(ctx, file.ID)
	if err != nil {
		t.Fatalf("GetFunscriptFile: %v", err)
	}
	if gotFile.Filename != file.Filename || len(gotFile.Actions) != 4 {
		t.Fatalf("got file = %+v", gotFile)
	}

	byHash, err := store.GetFunscriptFileByHash(ctx, "abc123")
	if err != nil {
		t.Fatalf("GetFunscriptFileByHash: %v", err)
	}
	if byHash.ID != file.ID {
		t.Fatalf("by hash id = %q, want %q", byHash.ID, file.ID)
	}

	block, err := store.CreateMotionBlock(ctx, MotionBlock{
		ID:           "sample-top-slow-01",
		SourceFileID: file.ID,
		StartMS:      0,
		EndMS:        1500,
		DurationMS:   1500,
		MinPos:       10,
		MaxPos:       90,
		AvgPos:       50,
		Amplitude:    80,
		Zone:         "mixed",
		StrokeLength: "medium",
		Speed:        "medium",
		Rhythm:       "steady",
		Intensity:    42,
		Tags:         []string{"mixed", "medium"},
		Actions:      gotFile.Actions,
	})
	if err != nil {
		t.Fatalf("CreateMotionBlock: %v", err)
	}
	if block.ContentHash == "" || block.SemanticSummary == "" {
		t.Fatalf("block = %+v, want hash and summary", block)
	}

	gotBlock, err := store.GetMotionBlock(ctx, block.ID)
	if err != nil {
		t.Fatalf("GetMotionBlock: %v", err)
	}
	if gotBlock.SourceFileID != file.ID {
		t.Fatalf("source file id = %q, want %q", gotBlock.SourceFileID, file.ID)
	}

	blocks, err := store.ListMotionBlocksByFileID(ctx, file.ID)
	if err != nil {
		t.Fatalf("ListMotionBlocksByFileID: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("blocks = %d, want 1", len(blocks))
	}

	files, err := store.ListFunscriptFiles(ctx, 10)
	if err != nil {
		t.Fatalf("ListFunscriptFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files = %d, want 1", len(files))
	}

	gotFile.DurationMS = 2000
	if err := store.UpdateFunscriptFile(ctx, gotFile); err != nil {
		t.Fatalf("UpdateFunscriptFile: %v", err)
	}
	updated, err := store.GetFunscriptFile(ctx, file.ID)
	if err != nil {
		t.Fatalf("GetFunscriptFile after update: %v", err)
	}
	if updated.DurationMS != 2000 {
		t.Fatalf("duration = %d, want 2000", updated.DurationMS)
	}

	if err := store.DeleteMotionBlock(ctx, block.ID); err != nil {
		t.Fatalf("DeleteMotionBlock: %v", err)
	}
	if _, err := store.DeleteMotionBlocksByFileID(ctx, file.ID); err != nil {
		t.Fatalf("DeleteMotionBlocksByFileID: %v", err)
	}
	if err := store.DeleteFunscriptFile(ctx, file.ID); err != nil {
		t.Fatalf("DeleteFunscriptFile: %v", err)
	}
	if _, err := store.GetFunscriptFile(ctx, file.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestIngestPersistRoundTrip(t *testing.T) {
	ctx := context.Background()
	result, err := funscript.IngestFunscriptFile(filepath.Join("..", "funscript", "testdata", "sample.funscript"), "")
	if err != nil {
		t.Fatalf("IngestFunscriptFile: %v", err)
	}

	store, err := Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	file, err := store.CreateFunscriptFile(ctx, FunscriptFile{
		Filename:     result.Source.Filename,
		Path:         result.Source.Path,
		DurationMS:   result.Summary.DurationMS,
		ActionCount:  result.Summary.ActionCount,
		ImportedAt:   result.Source.ImportedAt,
		Hash:         result.Source.Hash,
		SourceFormat: string(result.Source.SourceFormat),
		Metadata:     result.Metadata,
		Actions:      result.NormalizedActions,
	})
	if err != nil {
		t.Fatalf("CreateFunscriptFile: %v", err)
	}

	for _, record := range result.Blocks {
		_, err := store.CreateMotionBlock(ctx, MotionBlock{
			ID:           record.ID,
			SourceFileID: file.ID,
			StartMS:      record.StartMS,
			EndMS:        record.EndMS,
			DurationMS:   record.DurationMS,
			MinPos:       record.MinPos,
			MaxPos:       record.MaxPos,
			AvgPos:       record.AvgPos,
			Amplitude:    record.Amplitude,
			Zone:         record.Zone,
			StrokeLength: record.StrokeLength,
			Speed:        record.Speed,
			Rhythm:       record.Rhythm,
			Intensity:    record.Intensity,
			Tags:         record.Tags,
			Actions:      record.Actions,
			ContentHash:  funscript.HashBlockActions(record.Actions),
		})
		if err != nil {
			t.Fatalf("CreateMotionBlock(%s): %v", record.ID, err)
		}
	}

	blocks, err := store.ListMotionBlocksByFileID(ctx, file.ID)
	if err != nil {
		t.Fatalf("ListMotionBlocksByFileID: %v", err)
	}
	if len(blocks) != len(result.Blocks) {
		t.Fatalf("persisted blocks = %d, want %d", len(blocks), len(result.Blocks))
	}
}
