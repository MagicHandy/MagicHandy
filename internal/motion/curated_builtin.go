package motion

import (
	"embed"
	"encoding/json"
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
		definitions = append(definitions, mustNormalizeCatalog(PatternDefinition{
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
