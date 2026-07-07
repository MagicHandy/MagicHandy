package funscript

const (
	minSegmentDurationMS   = 1500
	maxSegmentDurationMS   = 45000
	minAmplitude           = 3
	maxHoldRatioWithoutTag = 0.6
)

// ImportQAResult reports whether a segmented block should be persisted.
type ImportQAResult struct {
	Approved     bool
	Warnings     []string
	RejectReason string
}

// EvaluateSegmentRecord returns whether a segmented block should be persisted.
func EvaluateSegmentRecord(record BlockRecord) ImportQAResult {
	warnings := make([]string, 0)
	durationMS := record.DurationMS
	amplitude := record.Amplitude
	actionCount := len(record.Actions)
	rhythm := record.Rhythm

	if durationMS < minSegmentDurationMS {
		return ImportQAResult{
			Approved:     false,
			RejectReason: sprintf("segment too short (%dms < %dms)", durationMS, minSegmentDurationMS),
		}
	}
	if durationMS > maxSegmentDurationMS {
		return ImportQAResult{
			Approved:     false,
			RejectReason: sprintf("segment too long (%dms > %dms)", durationMS, maxSegmentDurationMS),
		}
	}
	if actionCount < 2 {
		return ImportQAResult{Approved: false, RejectReason: "fewer than 2 actions"}
	}
	if amplitude < minAmplitude {
		return ImportQAResult{
			Approved:     false,
			RejectReason: sprintf("amplitude too small (%d < %d)", amplitude, minAmplitude),
		}
	}

	holdRatio := 0.0
	hasHoldRatio := false
	if record.Features != nil {
		holdRatio = record.Features.HoldRatio
		hasHoldRatio = true
	}
	if hasHoldRatio && holdRatio >= maxHoldRatioWithoutTag && rhythm != "pause_hold" {
		warnings = append(warnings, sprintf("high hold ratio (%.2f) without pause_hold rhythm tag", holdRatio))
	}
	return ImportQAResult{Approved: true, Warnings: warnings}
}

// FilterSegmentRecords splits segment records into accepted and rejected QA lists.
func FilterSegmentRecords(records []BlockRecord) (accepted []BlockRecord, rejected []map[string]any) {
	accepted = make([]BlockRecord, 0, len(records))
	for _, record := range records {
		qa := EvaluateSegmentRecord(record)
		if !qa.Approved {
			rejected = append(rejected, map[string]any{
				"id":          record.ID,
				"reason":      qa.RejectReason,
				"duration_ms": record.DurationMS,
			})
			continue
		}
		if len(qa.Warnings) > 0 {
			tags := append([]string(nil), record.Tags...)
			if !containsString(tags, "qa_warning") {
				tags = append(tags, "qa_warning")
			}
			record.Tags = tags
			record.QAWarnings = append([]string(nil), qa.Warnings...)
		}
		accepted = append(accepted, record)
	}
	return accepted, rejected
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
