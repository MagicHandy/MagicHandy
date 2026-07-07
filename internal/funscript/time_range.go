package funscript

import (
	"fmt"
	"strconv"
	"strings"
)

// BlockBounds holds original-script bounds for a block.
type BlockBounds struct {
	SourceStartMS      int
	SourceEndMS        int
	MotionEndMS        int
	EffectiveDurationMS int
	SourceTimeRange    string
	MotionTimeRange    string
	SourceRangeSlug    string
}

func formatSourceTimestampMS(ms int) string {
	totalSec := maxInt(ms, 0) / 1000
	minutes := totalSec / 60
	seconds := totalSec % 60
	if minutes > 0 {
		return fmt.Sprintf("%d:%02d", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

func formatSourceRangeDisplay(startMS, endMS int) string {
	start := maxInt(startMS, 0)
	end := maxInt(endMS, start)
	return fmt.Sprintf("%s–%s", formatSourceTimestampMS(start), formatSourceTimestampMS(end))
}

func slugMS(ms int) string {
	totalSec := maxInt(ms, 0) / 1000
	if totalSec >= 3600 {
		hours := totalSec / 3600
		rem := totalSec % 3600
		minutes := rem / 60
		seconds := rem % 60
		return fmt.Sprintf("%dh%02dm%02ds", hours, minutes, seconds)
	}
	if totalSec >= 60 {
		minutes := totalSec / 60
		seconds := totalSec % 60
		return fmt.Sprintf("%dm%02ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", totalSec)
}

func formatSourceRangeSlug(startMS, endMS int) string {
	return fmt.Sprintf("%s-%s", slugMS(startMS), slugMS(endMS))
}

// SourceRangeTag returns a tag for the source time range.
func SourceRangeTag(startMS, endMS int) string {
	return "source:" + formatSourceRangeSlug(startMS, endMS)
}

func blockSourceBounds(actions []Action, metadata map[string]any, isFullScript bool) BlockBounds {
	if len(actions) == 0 {
		return BlockBounds{
			SourceStartMS: 0,
			SourceEndMS:   0,
			MotionEndMS:   0,
			SourceTimeRange: "0s–0s",
		}
	}

	ordered := append([]Action(nil), actions...)
	for i := 0; i < len(ordered)-1; i++ {
		for j := i + 1; j < len(ordered); j++ {
			if ordered[j].At < ordered[i].At {
				ordered[i], ordered[j] = ordered[j], ordered[i]
			}
		}
	}

	sourceStartMS := ordered[0].At
	sourceEndMS := ordered[len(ordered)-1].At
	motionEndMS := MotionPlaybackEndMS(ordered)

	rangeEndMS := sourceEndMS
	if isFullScript {
		if metaMS := MetadataDurationMS(metadata); metaMS > sourceEndMS {
			rangeEndMS = metaMS
		}
	}

	effectiveMS := maxInt(1, motionEndMS-sourceStartMS)
	if isFullScript {
		effectiveMS = maxInt(1, EffectiveScriptDurationMS(ordered, metadata))
	}

	return BlockBounds{
		SourceStartMS:       sourceStartMS,
		SourceEndMS:         sourceEndMS,
		MotionEndMS:         motionEndMS,
		EffectiveDurationMS: effectiveMS,
		SourceTimeRange:     formatSourceRangeDisplay(sourceStartMS, rangeEndMS),
		MotionTimeRange:     formatSourceRangeDisplay(sourceStartMS, motionEndMS),
		SourceRangeSlug:     formatSourceRangeSlug(sourceStartMS, rangeEndMS),
	}
}

// ResolveBlockBounds trims dead time for playback while keeping source span labels.
func ResolveBlockBounds(actions []Action, metadata map[string]any, isFullScript bool) ([]Action, BlockBounds) {
	rawBounds := blockSourceBounds(actions, metadata, isFullScript)
	trimmed := TrimActionsForPlayback(actions, metadata)
	working := trimmed
	if len(trimmed) < 2 {
		working = append([]Action(nil), actions...)
	}

	motionBounds := rawBounds
	if len(trimmed) >= 2 && len(trimmed) < len(actions) {
		motionBounds = blockSourceBounds(trimmed, metadata, false)
	}

	effectiveMS := maxInt(1, motionBounds.MotionEndMS-rawBounds.SourceStartMS)
	if isFullScript {
		effectiveMS = maxInt(1, EffectiveScriptDurationMS(working, metadata))
	}

	bounds := BlockBounds{
		SourceStartMS:       rawBounds.SourceStartMS,
		SourceEndMS:         rawBounds.SourceEndMS,
		MotionEndMS:         motionBounds.MotionEndMS,
		EffectiveDurationMS: effectiveMS,
		SourceTimeRange:     rawBounds.SourceTimeRange,
		MotionTimeRange:     formatSourceRangeDisplay(rawBounds.SourceStartMS, motionBounds.MotionEndMS),
		SourceRangeSlug:     rawBounds.SourceRangeSlug,
	}
	return working, bounds
}

// ParseSlugMS parses compact slug token (12s, 1m23s) back to milliseconds.
func ParseSlugMS(token string) int {
	raw := strings.ToLower(strings.TrimSpace(token))
	if raw == "" {
		return 0
	}
	if strings.HasSuffix(raw, "s") && !strings.Contains(raw, "m") && !strings.Contains(raw, "h") {
		sec, _ := strconv.Atoi(strings.TrimSuffix(raw, "s"))
		return maxInt(sec, 0) * 1000
	}
	hours, minutes, seconds := 0, 0, 0
	if strings.Contains(raw, "h") {
		parts := strings.SplitN(raw, "h", 2)
		hours, _ = strconv.Atoi(parts[0])
		raw = parts[1]
	}
	if strings.Contains(raw, "m") {
		parts := strings.SplitN(raw, "m", 2)
		minutes, _ = strconv.Atoi(parts[0])
		raw = parts[1]
	}
	if strings.HasSuffix(raw, "s") {
		seconds, _ = strconv.Atoi(strings.TrimSuffix(raw, "s"))
	}
	totalSec := hours*3600 + minutes*60 + seconds
	return maxInt(totalSec, 0) * 1000
}
