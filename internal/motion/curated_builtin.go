package motion

import (
	"embed"
	"encoding/json"
	"math"
	"path"
	"strings"
)

//go:embed builtinpatterns/curated/*.mhpattern.json
var curatedPatternFiles embed.FS

type curatedPatternFile struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Kind        string       `json:"kind"`
	CycleMillis int64        `json:"cycle_ms"`
	Points      []CurvePoint `json:"points"`
	Tags        []string     `json:"tags,omitempty"`
}

func loadCuratedBuiltinPatterns() []PatternDefinition {
	entries, err := curatedPatternFiles.ReadDir("builtinpatterns/curated")
	if err != nil {
		panic("curated builtin patterns: " + err.Error())
	}
	definitions := make([]PatternDefinition, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filename := entry.Name()
		data, readErr := curatedPatternFiles.ReadFile(path.Join("builtinpatterns/curated", filename))
		if readErr != nil {
			panic("curated builtin patterns: read " + filename + ": " + readErr.Error())
		}
		var file curatedPatternFile
		if decodeErr := json.Unmarshal(data, &file); decodeErr != nil {
			panic("curated builtin patterns: decode " + filename + ": " + decodeErr.Error())
		}
		definitions = append(definitions, prepareCuratedBuiltinPattern(PatternDefinition{
			ID:          PatternID(curatedBuiltinPatternID(filename)),
			Name:        file.Name,
			Description: file.Description,
			Kind:        file.Kind,
			CycleMillis: file.CycleMillis,
			Points:      file.Points,
			Tags:        normalizeCuratedCatalogTags(file.Tags),
		}))
	}
	return definitions
}

func prepareCuratedBuiltinPattern(definition PatternDefinition) PatternDefinition {
	normalized := mustNormalizeCatalog(definition)
	metrics, err := MeasureCurve(normalized.Points, normalized.CycleMillis, true)
	if err != nil {
		panic("curated builtin pattern metrics: " + err.Error())
	}
	sourceExceededSafetyBudgets := exceedsCatalogSafetyBudgets(metrics)
	if sourceExceededSafetyBudgets {
		normalized.Points = resampleCuratedPattern(
			normalized.Points,
			normalized.CycleMillis,
			catalogMinReversalGap,
		)
	}
	normalized.Points = removeLowProminenceCuratedReversals(normalized.Points, catalogMinStrokeAmplitude)
	normalized = mustNormalizeCatalog(normalized)
	normalized = mustFitCatalog(normalized)
	feel, feelErr := measureCatalogPatternFeel(normalized)
	if feelErr != nil {
		panic("curated builtin pattern feel metrics: " + feelErr.Error())
	}
	if sourceExceededSafetyBudgets || !feel.acceptable() {
		normalized.Tags = normalizeTags(append(normalized.Tags, TagExperimental))
		if normalized.Description == "" {
			normalized.Description = "Experimental: Generated motion pending physical acceptance."
		} else if !strings.HasPrefix(normalized.Description, "Experimental: ") {
			normalized.Description = "Experimental: " + normalized.Description
		}
	}
	return normalized
}

func removeLowProminenceCuratedReversals(points []CurvePoint, minimumAmplitude float64) []CurvePoint {
	result := append([]CurvePoint(nil), points...)
	for len(result) > 2 {
		anchors := curveReversalAnchors(result)
		removed := false
		for index := 1; index < len(anchors)-1; index++ {
			left := result[anchors[index-1]].PositionPercent
			current := result[anchors[index]].PositionPercent
			right := result[anchors[index+1]].PositionPercent
			prominence := math.Min(math.Abs(current-left), math.Abs(current-right))
			if prominence >= minimumAmplitude {
				continue
			}
			pointIndex := anchors[index]
			result = append(result[:pointIndex], result[pointIndex+1:]...)
			removed = true
			break
		}
		if !removed {
			break
		}
	}
	return result
}

// resampleCuratedPattern preserves the strongest excursion in each complete
// time bucket while removing reversal clusters too fast for the catalog. The
// final fitter handles any remaining acceleration excess.
func resampleCuratedPattern(points []CurvePoint, duration, minimumInterval int64) []CurvePoint {
	curve, err := NewCurve(points, duration, true)
	if err != nil {
		panic("curated builtin pattern resampling: " + err.Error())
	}
	intervalCount := max(2, int(duration/minimumInterval))
	result := make([]CurvePoint, 1, intervalCount+1)
	result[0] = CurvePoint{PositionPercent: points[0].PositionPercent}
	sourceIndex := 1
	for interval := 1; interval < intervalCount; interval++ {
		start := int64(interval-1) * duration / int64(intervalCount)
		end := int64(interval) * duration / int64(intervalCount)
		position := curve.Sample(end)
		largestDelta := math.Abs(position - result[len(result)-1].PositionPercent)
		for sourceIndex < len(points) && points[sourceIndex].TimeMillis <= end {
			if points[sourceIndex].TimeMillis > start {
				delta := math.Abs(points[sourceIndex].PositionPercent - result[len(result)-1].PositionPercent)
				if delta > largestDelta {
					position = points[sourceIndex].PositionPercent
					largestDelta = delta
				}
			}
			sourceIndex++
		}
		result = append(result, CurvePoint{TimeMillis: end, PositionPercent: position})
	}
	return append(result, CurvePoint{TimeMillis: duration, PositionPercent: result[0].PositionPercent})
}

func curatedBuiltinPatternID(filename string) string {
	stem := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(filename)), ".mhpattern.json")
	return "curated-" + stem
}

func normalizeCuratedCatalogTags(tags []string) []string {
	filtered := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" || tag == TagCurated || tag == "imported" {
			continue
		}
		if strings.HasPrefix(tag, "pose-") || strings.HasPrefix(tag, "zone-") {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		filtered = append(filtered, tag)
	}
	return normalizeTags(filtered)
}
