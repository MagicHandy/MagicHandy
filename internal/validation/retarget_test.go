package validation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestRunRetargetValidationExportsRequiredTraces(t *testing.T) {
	fake := transport.NewFake()
	traces := diagnostics.NewTraceRing(512)
	engine, err := motion.NewEngine(motion.EngineOptions{
		Transport:        fake,
		Traces:           traces,
		ChunkSize:        4,
		SampleInterval:   25 * time.Millisecond,
		DispatchInterval: time.Hour,
		StreamIDPrefix:   "validation-test",
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	result, err := RunRetargetValidation(context.Background(), engine, traces, RetargetOptions{
		ExportDir:       t.TempDir(),
		MaxSpeedPercent: 35,
		SettlingDelay:   time.Millisecond,
		Sleep: func(context.Context, time.Duration) error {
			return nil
		},
	})
	if err != nil {
		t.Fatalf("RunRetargetValidation: %v", err)
	}
	if result.CombinedPath == "" {
		t.Fatalf("result = %+v, want combined trace path", result)
	}
	if len(result.TraceFiles) < 9 {
		t.Fatalf("trace files = %+v, want per-step plus combined exports", result.TraceFiles)
	}

	data, err := os.ReadFile(result.CombinedPath)
	if err != nil {
		t.Fatalf("read combined trace: %v", err)
	}
	var export diagnostics.TraceExport
	if err := json.Unmarshal(data, &export); err != nil {
		t.Fatalf("decode combined trace: %v", err)
	}
	if err := validateRequiredRetargetReasons(export); err != nil {
		t.Fatalf("validate retarget reasons: %v", err)
	}
	assertNoValidationSpeedAboveCap(t, export)
	assertTraceFileExists(t, result.CombinedPath)
	assertTraceFileExists(t, filepath.Join(result.ExportDir, "01-area-change.json"))
	assertTraceFileExists(t, filepath.Join(result.ExportDir, "07-emergency-stop.json"))
}

func TestRunRetargetValidationRejectsUnsafeSpeed(t *testing.T) {
	engine, err := motion.NewEngine(motion.EngineOptions{
		Transport: transport.NewFake(),
		Traces:    diagnostics.NewTraceRing(8),
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	_, err = RunRetargetValidation(context.Background(), engine, diagnostics.NewTraceRing(8), RetargetOptions{
		ExportDir:       t.TempDir(),
		MaxSpeedPercent: SafeValidationSpeedCapPercent + 1,
	})
	if err == nil {
		t.Fatal("RunRetargetValidation accepted an unsafe speed cap")
	}
}

func assertNoValidationSpeedAboveCap(t *testing.T, export diagnostics.TraceExport) {
	t.Helper()
	for _, row := range export.Rows {
		if row.Target != nil && row.Target.SpeedPercent > SafeValidationSpeedCapPercent {
			t.Fatalf("row %+v exceeded validation speed cap", row)
		}
		if row.Retarget != nil && row.Retarget.NextTarget != nil && row.Retarget.NextTarget.SpeedPercent > SafeValidationSpeedCapPercent {
			t.Fatalf("retarget %+v exceeded validation speed cap", row.Retarget)
		}
	}
}

func assertTraceFileExists(t *testing.T, path string) {
	t.Helper()
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Fatalf("trace file %q stat = %+v, err = %v", path, info, err)
	}
}
