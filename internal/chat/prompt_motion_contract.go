package chat

import "strings"

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
