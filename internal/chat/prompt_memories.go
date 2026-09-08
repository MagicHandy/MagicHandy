package chat

import (
	"sort"
	"strings"
	"unicode"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

const maxPromptMemoryBytes = 6 * 1024

// PromptBudgetSettings lets composition reserve the smallest supported input
// budget before selecting memories. The final provider guard also accounts for
// actual schema bytes and removes old history if necessary.
type PromptBudgetSettings struct {
	ContextSize           int
	MaxOutputTokens       int
	ReasoningBudgetTokens int
}

func memoryBudgetForPrompt(sections []PromptSection, options PromptBudgetSettings) int {
	limit := llm.PromptInputByteLimit(options.ContextSize, options.MaxOutputTokens, options.ReasoningBudgetTokens, 4096)
	// Reserve a maximum-size user turn, framing, and complete code-owned sections.
	available := limit - maxUserMessageBytes - 128 - 1024
	for _, section := range sections {
		if section.ID != "memories" {
			available -= section.Bytes + 2
		}
	}
	return max(0, min(maxPromptMemoryBytes, available))
}

// selectPromptMemories keeps complete memories within a shared byte budget.
// Relevant entries win, then newer entries; retained entries keep their stable
// storage order to preserve common prompt prefixes. Originals are never edited.
func selectPromptMemories(memories []string, context *MotionContext, limits ...int) []string {
	query := ""
	if context != nil {
		query = strings.Join(context.UserRequests, " ")
	}
	words := memoryWords(query)
	type candidate struct {
		text         string
		index, score int
	}
	candidates := make([]candidate, 0, len(memories))
	for index, text := range memories {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		score := 0
		for word := range memoryWords(text) {
			if words[word] {
				score++
			}
		}
		candidates = append(candidates, candidate{text, index, score})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].index > candidates[j].index
	})
	selected := make([]candidate, 0, len(candidates))
	remaining := maxPromptMemoryBytes
	if len(limits) > 0 {
		remaining = min(remaining, limits[0])
	}
	for _, item := range candidates {
		if size := len(item.text) + 3; size <= remaining {
			selected = append(selected, item)
			remaining -= size
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].index < selected[j].index })
	texts := make([]string, 0, len(selected))
	for _, item := range selected {
		texts = append(texts, item.text)
	}
	return texts
}

func memoryWords(text string) map[string]bool {
	words := make(map[string]bool)
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		if len(word) >= 3 {
			words[word] = true
		}
	}
	return words
}
