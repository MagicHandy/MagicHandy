package chat

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

const paceEvidenceGuide = `Read each original user message independently. In its own field, list each explicit request to SET a numeric pace or ADJUST it by a numeric amount, in order. For each request give operation set or adjust, the numeric value, and a short exact quote of the amount with its unit. Number words count. A reference to the current pace, a request to keep it, a rebound count, a travel limit or an unrelated number is not a new numeric pace request: return [] for that field. Never infer a value or perform a command.`

const paceEvidenceField = `{"type":"array","items":{"type":"object","properties":{"operation":{"type":"string","enum":["set","adjust"]},"value":{"type":"number"},"quote":{"type":"string"}},"required":["operation","value","quote"],"additionalProperties":false},"maxItems":8}`

type citedPace struct {
	Operation string   `json:"operation"`
	Value     *float64 `json:"value"`
	Quote     string   `json:"quote"`
}

var citedDigits = regexp.MustCompile(`[+-]?[0-9]+(?:\.[0-9]+)?`)

// Index extraction by the actual source row. Compact models otherwise
// confused the ordinal of a pace with the ID of the message that requested it.
func (s Service) openingPaceAnswer(ctx context.Context, lookup HistoryLookup, sources []HistorySource) (string, error) {
	sources = slices.DeleteFunc(slices.Clone(sources), func(row HistorySource) bool { return !hasCitedNumber(row.Text) })
	var all []citedPace
	// Four rows leave room for actual citations within the compact models'
	// output limit. Stop reading as soon as the requested sequence is settled.
	for rows := range slices.Chunk(sources, 4) {
		events, intact, err := s.readPaceRows(ctx, rows)
		if err != nil {
			return "", err
		}
		all = append(all, events...)
		if _, done := openingPaces(all, lookup.Count, *lookup.FollowingAdjustment); done || !intact {
			break
		}
	}
	return renderOpeningPaces(all, lookup.Count, *lookup.FollowingAdjustment, historyUnavailable(s.Prompt.ID)), ctx.Err()
}

func (s Service) readPaceRows(ctx context.Context, sources []HistorySource) ([]citedPace, bool, error) {
	originalCount := len(sources)
	for len(sources) > 0 {
		properties := make(map[string]json.RawMessage, len(sources))
		keys := make([]string, 0, len(sources))
		var archive strings.Builder
		for i, row := range sources {
			key := "row" + strconv.Itoa(i+1)
			keys = append(keys, key)
			properties[key] = json.RawMessage(paceEvidenceField)
			line, _ := json.Marshal(row.Text)
			archive.WriteString(key + " = " + string(line) + "\n")
		}
		schema, _ := json.Marshal(map[string]any{"type": "object", "properties": properties, "required": keys, "additionalProperties": false})
		input := s.historyAnswerRequest(schema)
		input.Messages = []llm.Message{{Role: "system", Content: paceEvidenceGuide}, {Role: "user", Content: archive.String()}}
		if _, _, err := llm.BudgetChatRequest(input, s.PromptBudget.ContextSize); err != nil {
			sources = sources[:len(sources)-1]
			continue
		}
		raw, err := s.Provider.StreamChat(ctx, input, func(string) error { return ctx.Err() })
		if err != nil {
			return nil, false, err
		}
		var cited map[string][]citedPace
		if json.Unmarshal([]byte(raw), &cited) != nil || len(cited) != len(keys) {
			return nil, false, errors.New("invalid archived pace evidence")
		}
		events, intact, err := verifiedPacePrefix(sources, keys, cited)
		return events, intact && len(sources) == originalCount, err
	}
	return nil, false, ctx.Err()
}

func verifiedPacePrefix(sources []HistorySource, keys []string, cited map[string][]citedPace) ([]citedPace, bool, error) {
	var events []citedPace
	used := make(map[int64][]quoteSpan)
	for i, key := range keys {
		items, ok := cited[key]
		if !ok || items == nil || len(items) > 8 {
			return nil, false, errors.New("missing or invalid archived pace evidence")
		}
		for _, item := range items {
			if (item.Operation != "set" && item.Operation != "adjust") || item.Value == nil || len(item.Quote) > 512 || !hasCitedNumber(item.Quote) {
				return events, false, nil
			}
			item.Quote = claimHistoryQuote(sources[i:i+1], sources[i].ID, strings.TrimSpace(item.Quote), used)
			if item.Quote == "" {
				return events, false, nil
			}
			if digits := citedDigits.FindString(item.Quote); digits != "" {
				number, err := strconv.ParseFloat(digits, 64)
				if err != nil || math.Abs(number) != math.Abs(*item.Value) {
					return events, false, nil
				}
			}
			events = append(events, item)
		}
	}
	return events, true, nil
}

func openingPaces(events []citedPace, count int, following bool) ([]string, bool) {
	var values []float64
	var answers []string
	for _, event := range events {
		if event.Operation == "set" && len(values) == count && *event.Value != values[count-1] {
			return answers, true // a later adjustment now belongs to a different setting
		}
		if event.Operation == "set" && !slices.Contains(values, *event.Value) {
			values = append(values, *event.Value)
			answers = append(answers, event.Quote)
			if len(values) == count && !following {
				return answers, true
			}
		} else if event.Operation == "adjust" && len(values) == count && following {
			answers = append(answers, event.Quote)
			return answers, true
		}
	}
	return answers, false
}

func renderOpeningPaces(events []citedPace, count int, following bool, unavailable string) string {
	answers, _ := openingPaces(events, count, following)
	if len(answers) == 0 {
		return unavailable
	}
	if following {
		count++
	}
	for len(answers) < count {
		answers = append(answers, unavailable)
	}
	return strings.Join(answers, ", ")
}
