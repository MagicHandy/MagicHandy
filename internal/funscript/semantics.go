package funscript

import (
	"strings"
	"unicode"
)

var (
	zonePT = map[string]string{
		"top":    "ponta/cabeça",
		"middle": "meio do eixo",
		"bottom": "base/fundo",
		"full":   "curso completo",
		"mixed":  "zonas mistas",
	}
	speedPT = map[string]string{
		"slow":        "lento",
		"very_slow":   "muito lento",
		"medium":      "médio",
		"medium_slow": "médio-lento",
		"medium_fast": "médio-rápido",
		"fast":        "rápido",
		"very_fast":   "muito rápido",
	}
	strokePT = map[string]string{
		"micro":  "micro-carrinhos",
		"short":  "curtos",
		"medium": "médios",
		"full":   "curso longo",
	}
	rhythmPT = map[string]string{
		"steady":       "ritmo constante",
		"accelerating": "acelerando",
		"decelerating": "desacelerando",
		"pulsed":       "pulsado",
		"chaotic":      "caótico/irregular",
		"pause_hold":   "pausas e holds",
	}
)

func lookupLabel(table map[string]string, key string) string {
	if label, ok := table[key]; ok {
		return label
	}
	return key
}

// BuildSemanticSummary returns a compact pt-BR description for library UI.
func BuildSemanticSummary(zone, strokeLength, speed, rhythm string, intensity float64, durationMS int, bpm float64, actionCount int, holdRatio float64, includeHold bool) string {
	parts := make([]string, 0, 8)
	if zone != "" {
		parts = append(parts, lookupLabel(zonePT, zone))
	}
	if strokeLength != "" {
		parts = append(parts, "amplitude "+lookupLabel(strokePT, strokeLength))
	}
	if speed != "" {
		parts = append(parts, lookupLabel(speedPT, speed))
	}
	if rhythm != "" {
		parts = append(parts, lookupLabel(rhythmPT, rhythm))
	}
	if durationMS > 0 {
		parts = append(parts, sprintf("~%.0fs", float64(durationMS)/1000.0))
	}
	if bpm > 0 {
		parts = append(parts, sprintf("~%.0f bat/min", bpm))
	}
	if intensity != 0 || len(parts) > 0 {
		parts = append(parts, sprintf("intensidade %.0f", intensity))
	}
	if actionCount > 0 {
		parts = append(parts, sprintf("%d pontos", actionCount))
	}
	if includeHold && holdRatio >= 0.35 {
		parts = append(parts, "com holds")
	}
	if len(parts) == 0 {
		return "movimento importado"
	}
	text := strings.Join(parts, ", ")
	return upperFirst(text)
}

// SemanticSummaryFromRecord builds summary from an ingest block record.
func SemanticSummaryFromRecord(record BlockRecord) string {
	if strings.TrimSpace(record.SemanticSummary) != "" {
		return strings.TrimSpace(record.SemanticSummary)
	}

	actions := record.Actions
	durationMS := record.DurationMS
	bpm := 0.0
	if durationMS > 0 && len(actions) >= 2 {
		actionList := storedToActions(actions)
		if result := ComputeStrokeBPM(actionList, durationMS); result != nil {
			if value, ok := result["bpm"].(float64); ok {
				bpm = value
			}
		}
	}

	holdRatio := 0.0
	includeHold := false
	if record.Features != nil {
		holdRatio = record.Features.HoldRatio
		includeHold = true
	}

	actionCount := len(actions)
	if actionCount == 0 {
		actionCount = record.ActionCount
	}

	return BuildSemanticSummary(
		record.Zone,
		record.StrokeLength,
		record.Speed,
		record.Rhythm,
		record.Intensity,
		durationMS,
		bpm,
		actionCount,
		holdRatio,
		includeHold,
	)
}

func storedToActions(stored []StoredAction) []Action {
	out := make([]Action, len(stored))
	for i, item := range stored {
		out[i] = Action(item)
	}
	return out
}

func upperFirst(text string) string {
	if text == "" {
		return text
	}
	runes := []rune(text)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
