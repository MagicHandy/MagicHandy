package patterns

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const (
	importAsPattern           = "pattern"
	importAsProgram           = "program"
	inactiveGapMillis   int64 = 5000
	inactiveReplacement int64 = 500
)

type funscriptFile struct {
	Version  string            `json:"version"`
	Inverted bool              `json:"inverted"`
	Actions  []funscriptAction `json:"actions"`
}

type funscriptAction struct {
	At  json.Number `json:"at"`
	Pos float64     `json:"pos"`
}

// Import parses a MagicHandy share file or standard funscript.
func (l *Library) Import(filename string, data []byte, asKind string) (ImportResult, error) {
	if len(data) == 0 {
		return ImportResult{}, invalidContentError(errors.New("import file is empty"))
	}
	if len(data) > MaxImportBytes {
		return ImportResult{}, invalidContentError(fmt.Errorf("import file exceeds %d MiB", MaxImportBytes>>20))
	}
	var header struct {
		Schema  string            `json:"schema"`
		Actions []json.RawMessage `json:"actions"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return ImportResult{}, invalidContentError(fmt.Errorf("import file is not valid JSON: %w", err))
	}
	switch header.Schema {
	case PatternFileSchema:
		return l.importPatternFile(data)
	case ProgramFileSchema:
		return l.importProgramFile(data)
	default:
		if header.Actions != nil {
			return l.importFunscript(filename, data, asKind)
		}
		return ImportResult{}, invalidContentError(errors.New("unknown motion content schema"))
	}
}

// ExportPattern returns one individual shareable pattern file.
func (l *Library) ExportPattern(id string) ([]byte, string, error) {
	pattern, err := l.Pattern(id)
	if err != nil {
		return nil, "", err
	}
	file := patternFile{
		Schema: PatternFileSchema, Name: pattern.Name, Description: pattern.Description,
		Kind: pattern.Kind, CycleMillis: pattern.CycleMillis,
		Points: pattern.Points, Tags: pattern.Tags,
	}
	data, err := json.MarshalIndent(file, "", "  ")
	return append(data, '\n'), safeFilename(pattern.Name) + ".mhpattern.json", err
}

// ExportProgram returns one individual finite program file.
func (l *Library) ExportProgram(id string) ([]byte, string, error) {
	program, err := l.Program(id)
	if err != nil {
		return nil, "", err
	}
	file := programFile{
		Schema: ProgramFileSchema, Name: program.Name,
		DurationMillis: program.DurationMillis, Points: program.Points,
	}
	data, err := json.MarshalIndent(file, "", "  ")
	return append(data, '\n'), safeFilename(program.Name) + ".mhprogram.json", err
}

func (l *Library) importPatternFile(data []byte) (ImportResult, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var file patternFile
	if err := decoder.Decode(&file); err != nil {
		return ImportResult{}, invalidContentError(fmt.Errorf("decode pattern file: %w", err))
	}
	pattern, err := l.CreatePattern(PatternInput{
		Name: file.Name, Description: file.Description, Kind: file.Kind,
		CycleMillis: file.CycleMillis, Points: file.Points, Tags: file.Tags,
	})
	if err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Kind: importAsPattern, Pattern: &pattern}, nil
}

func (l *Library) importProgramFile(data []byte) (ImportResult, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var file programFile
	if err := decoder.Decode(&file); err != nil {
		return ImportResult{}, invalidContentError(fmt.Errorf("decode program file: %w", err))
	}
	program, err := l.CreateProgram(file.Name, OriginImported, file.Points, file.DurationMillis)
	if err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Kind: importAsProgram, Program: &program}, nil
}

func (l *Library) importFunscript(filename string, data []byte, asKind string) (ImportResult, error) {
	asKind = strings.ToLower(strings.TrimSpace(asKind))
	if asKind == "" {
		asKind = importAsProgram
	}
	if asKind != importAsPattern && asKind != importAsProgram {
		return ImportResult{}, invalidContentError(errors.New("funscript import target must be pattern or program"))
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var file funscriptFile
	if err := decoder.Decode(&file); err != nil {
		return ImportResult{}, invalidContentError(fmt.Errorf("decode funscript: %w", err))
	}
	points, err := funscriptPoints(file)
	if err != nil {
		return ImportResult{}, invalidContentError(err)
	}
	name := strings.TrimSpace(strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)))
	if name == "" {
		name = "Imported funscript"
	}
	if asKind == importAsPattern {
		points, gaps := stripInactiveGaps(points)
		points, err = normalizeRelativeSpan(points)
		if err != nil {
			return ImportResult{}, invalidContentError(err)
		}
		pattern, err := l.CreatePattern(PatternInput{
			Name: name, Description: "Imported funscript example.",
			Kind: motion.PatternKindRoutine, CycleMillis: points[len(points)-1].TimeMillis,
			Points: points, Tags: []string{"imported", "funscript"}, SimplifyError: 1,
		})
		if err != nil {
			return ImportResult{}, err
		}
		return ImportResult{Kind: importAsPattern, Pattern: &pattern, GapsStripped: gaps}, nil
	}
	program, err := l.CreateProgram(name, OriginImported, points, points[len(points)-1].TimeMillis)
	if err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Kind: importAsProgram, Program: &program}, nil
}

func invalidContentError(err error) error {
	if errors.Is(err, ErrInvalidContent) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrInvalidContent, err)
}

func funscriptPoints(file funscriptFile) ([]motion.CurvePoint, error) {
	if len(file.Actions) < 2 || len(file.Actions) > maxProgramPoints*5 {
		return nil, fmt.Errorf("funscript requires 2..%d actions", maxProgramPoints*5)
	}
	points := make([]motion.CurvePoint, 0, len(file.Actions))
	for index, action := range file.Actions {
		at, err := numberMillis(action.At)
		if err != nil || at < 0 || at > maxContentDuration {
			return nil, fmt.Errorf("funscript action %d has invalid time", index)
		}
		if math.IsNaN(action.Pos) || math.IsInf(action.Pos, 0) || action.Pos < 0 || action.Pos > 100 {
			return nil, fmt.Errorf("funscript action %d position must be between 0 and 100", index)
		}
		position := action.Pos
		if file.Inverted {
			position = 100 - position
		}
		points = append(points, motion.CurvePoint{TimeMillis: at, PositionPercent: position})
	}
	slices.SortStableFunc(points, comparePointTime)
	points = deduplicatePointTimes(points)
	if len(points) < 2 || points[len(points)-1].TimeMillis == points[0].TimeMillis {
		return nil, errors.New("funscript needs at least two distinct action times")
	}
	start := points[0].TimeMillis
	for index := range points {
		points[index].TimeMillis -= start
	}
	return points, nil
}

func numberMillis(number json.Number) (int64, error) {
	if value, err := strconv.ParseInt(string(number), 10, 64); err == nil {
		return value, nil
	}
	value, err := strconv.ParseFloat(string(number), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, errors.New("invalid number")
	}
	return int64(math.Round(value)), nil
}

func stripInactiveGaps(points []motion.CurvePoint) ([]motion.CurvePoint, int) {
	result := append([]motion.CurvePoint(nil), points...)
	removed := int64(0)
	count := 0
	for index := 1; index < len(result); index++ {
		originalGap := points[index].TimeMillis - points[index-1].TimeMillis
		if originalGap > inactiveGapMillis && math.Abs(points[index].PositionPercent-points[index-1].PositionPercent) <= 2 {
			removed += originalGap - inactiveReplacement
			count++
		}
		result[index].TimeMillis -= removed
	}
	return result, count
}

func normalizeRelativeSpan(points []motion.CurvePoint) ([]motion.CurvePoint, error) {
	minimum, maximum := points[0].PositionPercent, points[0].PositionPercent
	for _, point := range points[1:] {
		minimum = math.Min(minimum, point.PositionPercent)
		maximum = math.Max(maximum, point.PositionPercent)
	}
	if maximum-minimum < 1 {
		return nil, errors.New("funscript example has no usable motion span")
	}
	result := append([]motion.CurvePoint(nil), points...)
	for index := range result {
		result[index].PositionPercent = (result[index].PositionPercent - minimum) * 100 / (maximum - minimum)
	}
	return result, nil
}

func safeFilename(name string) string {
	name = strings.Map(func(character rune) rune {
		switch character {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*', '\r', '\n':
			return '-'
		default:
			return character
		}
	}, strings.TrimSpace(name))
	name = strings.Trim(name, " .-")
	if name == "" {
		return "motion-content"
	}
	return name
}
