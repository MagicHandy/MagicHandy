package funscript

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DetectFormat chooses the loader from a file extension.
func DetectFormat(path string) (SourceFormat, error) {
	suffix := strings.ToLower(filepath.Ext(path))
	switch suffix {
	case ".csv":
		return SourceFormatCSV, nil
	case ".funscript":
		return SourceFormatFunscript, nil
	case ".json":
		return SourceFormatJSON, nil
	default:
		return "", parseErrorf("unsupported extension %q. Use .csv, .json, or .funscript", suffix)
	}
}

// LoadCSVActions parses StrokeGPT-style CSV: time_ms,pos per line.
func LoadCSVActions(text, source string) ([]Action, error) {
	if source == "" {
		source = "<csv>"
	}
	actions := make([]Action, 0, 64)
	lineNumber := 0
	for _, rawLine := range strings.Split(text, "\n") {
		lineNumber++
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		header := strings.ReplaceAll(strings.ToLower(line), " ", "")
		if lineNumber == 1 && (header == "time_ms,pos" || header == "at,pos" || header == "time,pos") {
			continue
		}
		at, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return nil, parseErrorf("%s:%d: invalid row %q", source, lineNumber, rawLine)
		}
		pos, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return nil, parseErrorf("%s:%d: invalid row %q", source, lineNumber, rawLine)
		}
		actions = append(actions, Action{At: int(at + 0.5), Pos: pos})
	}
	if len(actions) < 2 {
		return nil, parseErrorf("%s: CSV needs at least two actions.", source)
	}
	return actions, nil
}

// LoadJSONPayload loads actions from a JSON / .funscript object.
func LoadJSONPayload(payload map[string]any, source string) (LoadedFunscript, error) {
	actionsRaw, ok := payload["actions"]
	if !ok {
		return LoadedFunscript{}, parseErrorf("%s: missing 'actions' array.", source)
	}
	items, ok := actionsRaw.([]any)
	if !ok {
		return LoadedFunscript{}, parseErrorf("%s: missing 'actions' array.", source)
	}

	actions := make([]Action, 0, len(items))
	for index, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			return LoadedFunscript{}, parseErrorf("%s: actions[%d] must be an object.", source, index)
		}
		if _, ok := row["at"]; !ok {
			return LoadedFunscript{}, parseErrorf("%s: actions[%d] needs 'at' and 'pos'.", source, index)
		}
		if _, ok := row["pos"]; !ok {
			return LoadedFunscript{}, parseErrorf("%s: actions[%d] needs 'at' and 'pos'.", source, index)
		}
		at, err := toFloat(row["at"])
		if err != nil {
			return LoadedFunscript{}, parseErrorf("%s: actions[%d] needs 'at' and 'pos'.", source, index)
		}
		pos, err := toFloat(row["pos"])
		if err != nil {
			return LoadedFunscript{}, parseErrorf("%s: actions[%d] needs 'at' and 'pos'.", source, index)
		}
		actions = append(actions, Action{At: int(at + 0.5), Pos: pos})
	}
	if len(actions) < 2 {
		return LoadedFunscript{}, parseErrorf("%s: need at least two actions.", source)
	}

	metadata := map[string]any{}
	if raw, ok := payload["metadata"].(map[string]any); ok {
		for k, v := range raw {
			metadata[k] = v
		}
	}

	extra := map[string]any{}
	for _, key := range []string{"version", "inverted", "range"} {
		if v, ok := payload[key]; ok {
			extra[key] = v
		}
	}
	if len(metadata) > 0 {
		extra["metadata"] = metadata
	}

	sourceFormat := SourceFormatJSON
	if strings.HasSuffix(strings.ToLower(source), ".funscript") {
		sourceFormat = SourceFormatFunscript
	}

	return LoadedFunscript{
		Actions:      actions,
		SourceFormat: sourceFormat,
		Metadata:     metadata,
		ExtraFields:  extra,
		SourcePath:   source,
	}, nil
}

// LoadActionsFromPath detects format by extension and loads actions.
func LoadActionsFromPath(path string) (LoadedFunscript, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return LoadedFunscript{}, parseErrorf("file not found: %s", path)
	}

	sourceFormat, err := DetectFormat(path)
	if err != nil {
		return LoadedFunscript{}, err
	}

	data, err := os.ReadFile(path) // #nosec G304 -- caller supplies import path.
	if err != nil {
		return LoadedFunscript{}, parseErrorf("cannot read %s: %v", path, err)
	}
	text := strings.TrimPrefix(string(data), "\ufeff")

	if sourceFormat == SourceFormatCSV {
		actions, err := LoadCSVActions(text, path)
		if err != nil {
			return LoadedFunscript{}, err
		}
		stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		return LoadedFunscript{
			Actions:      actions,
			SourceFormat: SourceFormatCSV,
			Metadata: map[string]any{
				"name":        stem,
				"creator":     "csv-import",
				"description": fmt.Sprintf("Imported from CSV: %s", filepath.Base(path)),
			},
			ExtraFields: map[string]any{},
			SourcePath:  path,
		}, nil
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return LoadedFunscript{}, parseErrorf("%s: invalid JSON — %v", path, err)
	}
	loaded, err := LoadJSONPayload(payload, path)
	if err != nil {
		return LoadedFunscript{}, err
	}
	loaded.SourcePath = path
	return loaded, nil
}

// LoadActionsFromText loads from upload bytes using the filename extension.
func LoadActionsFromText(text, filename string) (LoadedFunscript, error) {
	if filename == "" {
		filename = "upload.funscript"
	}
	suffix := strings.ToLower(filepath.Ext(filename))
	text = strings.TrimPrefix(text, "\ufeff")

	if suffix == ".csv" {
		actions, err := LoadCSVActions(text, filename)
		if err != nil {
			return LoadedFunscript{}, err
		}
		stem := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
		return LoadedFunscript{
			Actions:      actions,
			SourceFormat: SourceFormatCSV,
			Metadata: map[string]any{
				"name":    stem,
				"creator": "csv-import",
			},
			SourcePath: filename,
		}, nil
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return LoadedFunscript{}, parseErrorf("%s: invalid JSON — %v", filename, err)
	}
	loaded, err := LoadJSONPayload(payload, filename)
	if err != nil {
		return LoadedFunscript{}, err
	}
	switch suffix {
	case ".funscript":
		loaded.SourceFormat = SourceFormatFunscript
	default:
		loaded.SourceFormat = SourceFormatJSON
	}
	return loaded, nil
}

func toFloat(v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case json.Number:
		return n.Float64()
	default:
		return 0, fmt.Errorf("not a number")
	}
}
