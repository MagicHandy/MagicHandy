package chat

import "strings"

// appendContractSections composes the response contract and, when the device
// can move, the depth frame beside it and the enabled pattern catalog.
func appendContractSections(sections []PromptSection, capabilities Capabilities, context *MotionContext, patterns []PatternChoice) []PromptSection {
	sections = appendPromptSection(sections, "response_contract", "Response contract",
		contractForMotionState(capabilities, context))
	if !capabilities.Motion {
		return sections
	}
	sections = appendPromptSection(sections, "depth_frame", "Depth frame", depthFrame)
	if capabilities.Patterns {
		sections = appendPromptSection(sections, "pattern_catalog", "Pattern catalog",
			curationInstructions(patterns))
	}
	return sections
}

// contractForMotionState removes examples that are invalid for the supplied
// lifecycle state. This narrows the grammar without choosing whether to act,
// prescribing geometry, or overriding the model's none/Stop choices.
func contractForMotionState(capabilities Capabilities, context *MotionContext) string {
	contract := contractInstructions(capabilities)
	if context == nil || !capabilities.Motion || capabilities.MotionMode != MotionModeDynamic {
		return contract
	}
	lines := strings.Split(contract, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		starts := strings.Contains(line, `"action":"start"`)
		updates := strings.Contains(line, `"action":"update"`)
		if context.Paused && (starts || updates) {
			continue
		}
		if !context.Running && updates {
			continue
		}
		if context.Running && !context.Paused && starts {
			line = strings.ReplaceAll(line, `"action":"start"`, `"action":"update"`)
			line = strings.Replace(line, "- Start ", "- Replace active motion with ", 1)
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}
