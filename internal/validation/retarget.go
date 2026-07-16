// Package validation drives repeatable hardware validation workflows.
package validation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const (
	// SafeValidationSpeedCapPercent is the highest speed allowed by the hardware validator.
	SafeValidationSpeedCapPercent = 40
	// DefaultValidationMaxSpeedPercent keeps automated validation below the hard safety cap.
	DefaultValidationMaxSpeedPercent = 35
)

// RetargetOptions controls a Phase 7 real-device retarget validation run.
type RetargetOptions struct {
	ExportDir       string
	MaxSpeedPercent int
	SettlingDelay   time.Duration
	Sleep           func(context.Context, time.Duration) error
}

// RetargetResult summarizes trace files produced by a validation run.
type RetargetResult struct {
	ExportDir    string              `json:"export_dir"`
	StartedAt    string              `json:"started_at"`
	CompletedAt  string              `json:"completed_at,omitempty"`
	TraceFiles   []RetargetTraceFile `json:"trace_files"`
	CombinedPath string              `json:"combined_path,omitempty"`
}

// RetargetTraceFile records one exported trace artifact.
type RetargetTraceFile struct {
	Name   string `json:"name"`
	Reason string `json:"reason,omitempty"`
	Path   string `json:"path"`
	Rows   int    `json:"rows"`
}

type retargetStep struct {
	name   string
	reason string
	run    func(context.Context, *motion.Engine, *config.MotionSettings, int) error
}

// RunRetargetValidation drives the standard Phase 7 retarget checklist and exports trace files.
func RunRetargetValidation(
	ctx context.Context,
	engine *motion.Engine,
	traces *diagnostics.TraceRing,
	options RetargetOptions,
) (RetargetResult, error) {
	if engine == nil {
		return RetargetResult{}, errors.New("validation requires a motion engine")
	}
	if traces == nil {
		return RetargetResult{}, errors.New("validation requires a trace ring")
	}
	options = normalizeRetargetOptions(options)
	if err := validateRetargetOptions(options); err != nil {
		return RetargetResult{}, err
	}
	if err := os.MkdirAll(options.ExportDir, 0o700); err != nil {
		return RetargetResult{}, fmt.Errorf("create validation export directory: %w", err)
	}

	result := RetargetResult{
		ExportDir: options.ExportDir,
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	startSequence := latestTraceSequence(traces.Rows())
	currentExport := func() diagnostics.TraceExport {
		return traceExportAfter(traces.Export(), startSequence)
	}
	settings := validationSettings(options.MaxSpeedPercent)
	stopped := false
	defer func() {
		if !stopped {
			_, _ = engine.Stop(context.Background(), "validation_cleanup_stop")
			_, _ = exportTraceFile(options.ExportDir, "cleanup", "validation_cleanup_stop", currentExport())
		}
	}()

	if _, err := engine.Start(ctx, validationTarget("baseline", motion.PatternStroke, boundedSpeed(30, options.MaxSpeedPercent)), settings); err != nil {
		return result, exportFailure(options.ExportDir, &result, "00-start-failed", "validation_start", currentExport(), err)
	}
	if err := recordTrace(options.ExportDir, &result, "00-start", "validation_start", currentExport()); err != nil {
		return result, err
	}
	if err := options.Sleep(ctx, options.SettlingDelay); err != nil {
		return result, exportFailure(options.ExportDir, &result, "00-start-interrupted", "validation_start", currentExport(), err)
	}

	for index, step := range retargetSteps() {
		if err := step.run(ctx, engine, &settings, options.MaxSpeedPercent); err != nil {
			_, stopErr := engine.Stop(ctx, "validation_error_stop")
			stopped = stopErr == nil
			if stopErr != nil {
				err = errors.Join(err, fmt.Errorf("validation recovery Stop failed: %w", stopErr))
			}
			return result, exportFailure(options.ExportDir, &result, fmt.Sprintf("%02d-%s-failed", index+1, step.name), step.reason, currentExport(), err)
		}
		if step.reason == "validation_emergency_stop" {
			stopped = true
		}
		if err := recordTrace(options.ExportDir, &result, fmt.Sprintf("%02d-%s", index+1, step.name), step.reason, currentExport()); err != nil {
			return result, err
		}
		if !stopped {
			if err := options.Sleep(ctx, options.SettlingDelay); err != nil {
				_, stopErr := engine.Stop(ctx, "validation_interrupted_stop")
				stopped = stopErr == nil
				if stopErr != nil {
					err = errors.Join(err, fmt.Errorf("validation interruption Stop failed: %w", stopErr))
				}
				return result, exportFailure(options.ExportDir, &result, fmt.Sprintf("%02d-%s-interrupted", index+1, step.name), step.reason, currentExport(), err)
			}
		}
	}

	export := currentExport()
	if err := validateRequiredRetargetReasons(export); err != nil {
		return result, exportFailure(options.ExportDir, &result, "required-retargets-missing", "validation_trace_audit", export, err)
	}
	combined, err := exportTraceFile(options.ExportDir, "combined", "validation_combined", export)
	if err != nil {
		return result, err
	}
	result.CombinedPath = combined.Path
	result.TraceFiles = append(result.TraceFiles, combined)
	result.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return result, nil
}

func normalizeRetargetOptions(options RetargetOptions) RetargetOptions {
	if options.MaxSpeedPercent == 0 {
		options.MaxSpeedPercent = DefaultValidationMaxSpeedPercent
	}
	if options.SettlingDelay <= 0 {
		options.SettlingDelay = 1500 * time.Millisecond
	}
	if options.Sleep == nil {
		options.Sleep = sleepContext
	}
	return options
}

func validateRetargetOptions(options RetargetOptions) error {
	if options.ExportDir == "" {
		return errors.New("validation export directory is required")
	}
	if options.MaxSpeedPercent < 10 {
		return errors.New("validation max speed must be at least 10 percent")
	}
	if options.MaxSpeedPercent > SafeValidationSpeedCapPercent {
		return fmt.Errorf("validation max speed %d exceeds safe cap %d", options.MaxSpeedPercent, SafeValidationSpeedCapPercent)
	}
	return nil
}

func validationSettings(maxSpeed int) config.MotionSettings {
	return config.MotionSettings{
		SpeedMinPercent:  10,
		SpeedMaxPercent:  maxSpeed,
		StrokeMinPercent: 20,
		StrokeMaxPercent: 80,
	}
}

func retargetSteps() []retargetStep {
	return []retargetStep{
		{
			name:   "area-change",
			reason: "validation_area_change",
			run: func(ctx context.Context, engine *motion.Engine, _ *config.MotionSettings, maxSpeed int) error {
				_, err := engine.ApplyTarget(ctx, validationTarget("area focus", motion.PatternStroke, boundedSpeed(30, maxSpeed), motion.AreaFocus{MinPercent: 0, MaxPercent: 30}), "validation_area_change")
				return err
			},
		},
		{
			name:   "speed-change",
			reason: "validation_speed_change",
			run: func(ctx context.Context, engine *motion.Engine, _ *config.MotionSettings, maxSpeed int) error {
				_, err := engine.ApplyTarget(ctx, validationTarget("speed change", motion.PatternStroke, maxSpeed), "validation_speed_change")
				return err
			},
		},
		{
			name:   "stroke-change",
			reason: "validation_stroke_change",
			run: func(ctx context.Context, engine *motion.Engine, settings *config.MotionSettings, _ int) error {
				settings.StrokeMinPercent = 25
				settings.StrokeMaxPercent = 70
				_, err := engine.RefreshSettings(ctx, *settings, "validation_stroke_change")
				return err
			},
		},
		{
			name:   "direction-change",
			reason: "validation_direction_change",
			run: func(ctx context.Context, engine *motion.Engine, settings *config.MotionSettings, _ int) error {
				settings.ReverseDirection = true
				_, err := engine.RefreshSettings(ctx, *settings, "validation_direction_change")
				return err
			},
		},
		{
			name:   "same-pattern-change",
			reason: "validation_same_pattern_change",
			run: func(ctx context.Context, engine *motion.Engine, _ *config.MotionSettings, maxSpeed int) error {
				target := validationTarget("same pattern", motion.PatternStroke, boundedSpeed(25, maxSpeed))
				target.SoftAnchor = &motion.SoftAnchor{PositionPercent: 45, WeightPercent: 30}
				_, err := engine.ApplyTarget(ctx, target, "validation_same_pattern_change")
				return err
			},
		},
		{
			name:   "cross-pattern-change",
			reason: "validation_cross_pattern_change",
			run: func(ctx context.Context, engine *motion.Engine, _ *config.MotionSettings, maxSpeed int) error {
				_, err := engine.ApplyTarget(ctx, validationTarget("cross pattern", motion.PatternPulse, boundedSpeed(30, maxSpeed)), "validation_cross_pattern_change")
				return err
			},
		},
		{
			name:   "emergency-stop",
			reason: "validation_emergency_stop",
			run: func(ctx context.Context, engine *motion.Engine, _ *config.MotionSettings, _ int) error {
				_, err := engine.Stop(ctx, "validation_emergency_stop")
				return err
			},
		},
	}
}

func validationTarget(label string, pattern motion.PatternID, speed int, focus ...motion.AreaFocus) motion.MotionTarget {
	target := motion.MotionTarget{
		Label:        label,
		Source:       "phase7_validation",
		PatternID:    pattern,
		SpeedPercent: speed,
	}
	if len(focus) > 0 {
		target.AreaFocus = &focus[0]
	}
	return target
}

func boundedSpeed(speed int, maxSpeed int) int {
	if speed > maxSpeed {
		return maxSpeed
	}
	if speed < 10 {
		return 10
	}
	return speed
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func recordTrace(exportDir string, result *RetargetResult, name string, reason string, export diagnostics.TraceExport) error {
	file, err := exportTraceFile(exportDir, name, reason, export)
	if err != nil {
		return err
	}
	result.TraceFiles = append(result.TraceFiles, file)
	return nil
}

func exportFailure(exportDir string, result *RetargetResult, name string, reason string, export diagnostics.TraceExport, cause error) error {
	if err := recordTrace(exportDir, result, name, reason, export); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func exportTraceFile(exportDir string, name string, reason string, export diagnostics.TraceExport) (RetargetTraceFile, error) {
	path := filepath.Join(exportDir, name+".json")
	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return RetargetTraceFile{}, fmt.Errorf("encode trace export %s: %w", name, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return RetargetTraceFile{}, fmt.Errorf("write trace export %s: %w", name, err)
	}
	return RetargetTraceFile{
		Name:   name,
		Reason: reason,
		Path:   path,
		Rows:   len(export.Rows),
	}, nil
}

func validateRequiredRetargetReasons(export diagnostics.TraceExport) error {
	required := map[string]bool{
		"validation_area_change":          false,
		"validation_speed_change":         false,
		"validation_stroke_change":        false,
		"validation_direction_change":     false,
		"validation_same_pattern_change":  false,
		"validation_cross_pattern_change": false,
	}
	for _, row := range export.Rows {
		if row.Retarget == nil {
			continue
		}
		if _, ok := required[row.Reason]; ok {
			required[row.Reason] = true
		}
	}
	var missing []string
	for reason, seen := range required {
		if !seen {
			missing = append(missing, reason)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return fmt.Errorf("validation trace missing retarget rows for %v", missing)
	}
	return nil
}

func latestTraceSequence(rows []diagnostics.MotionTraceRow) uint64 {
	var latest uint64
	for _, row := range rows {
		if row.Sequence > latest {
			latest = row.Sequence
		}
	}
	return latest
}

func traceExportAfter(export diagnostics.TraceExport, sequence uint64) diagnostics.TraceExport {
	rows := make([]diagnostics.MotionTraceRow, 0, len(export.Rows))
	for _, row := range export.Rows {
		if row.Sequence > sequence {
			rows = append(rows, row)
		}
	}
	export.Rows = rows
	return export
}
