package llm

import (
	"encoding/json"
	"slices"
)

var routerEfforts = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}

// OpenRouterOutputTokens reserves room for hidden reasoning, which shares the
// completion cap with the answer. Apply this before input admission, for every
// call (including short decisions, repairs and readiness), not just in the UI
// connection check. It is a ceiling, not a request to generate more text.
func OpenRouterOutputTokens(answerTokens int, effort string, enabled *bool) int {
	if effort == "none" || (enabled != nil && !*enabled) {
		return answerTokens
	}
	return max(answerTokens, 2048)
}

type compatibleCatalogModel struct {
	HostedModel
	Reasoning *struct {
		// Omitted means no effort selector; null accepts all gateway efforts.
		SupportedEfforts json.RawMessage `json:"supported_efforts"`
		DefaultEffort    string          `json:"default_effort"`
		Mandatory        bool            `json:"mandatory"`
	} `json:"reasoning"`
}

func (m compatibleCatalogModel) withRouterReasoning() HostedModel {
	model := m.HostedModel
	// Only the documented OpenRouter metadata authorizes reasoning controls.
	model.ReasoningEfforts = nil
	model.DefaultReasoningEffort = ""
	model.ReasoningSupported = m.Reasoning != nil
	model.ReasoningMandatory = m.Reasoning != nil && m.Reasoning.Mandatory
	if m.Reasoning == nil {
		return model
	}
	var accepted []string
	if string(m.Reasoning.SupportedEfforts) == "null" {
		accepted = routerEfforts
	} else {
		_ = json.Unmarshal(m.Reasoning.SupportedEfforts, &accepted)
	}
	for _, effort := range routerEfforts {
		if slices.Contains(accepted, effort) && (effort != "none" || !model.ReasoningMandatory) {
			model.ReasoningEfforts = append(model.ReasoningEfforts, effort)
		}
	}
	if slices.Contains(model.ReasoningEfforts, m.Reasoning.DefaultEffort) {
		model.DefaultReasoningEffort = m.Reasoning.DefaultEffort
	}
	return model
}

func routerReasoning(model HostedModel, effort string, enabled *bool) (map[string]any, error) {
	if effort == "" && enabled == nil {
		return nil, nil
	}
	if !model.ReasoningSupported || (effort != "" && enabled != nil) ||
		(effort != "" && !slices.Contains(model.ReasoningEfforts, effort)) ||
		(model.ReasoningMandatory && (effort == "none" || (enabled != nil && !*enabled))) {
		return nil, &CloudError{Kind: "reasoning"}
	}
	if enabled != nil {
		return map[string]any{"enabled": *enabled}, nil
	}
	return map[string]any{"effort": effort}, nil
}
