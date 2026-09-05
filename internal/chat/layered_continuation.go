package chat

// LayeredExactHoldRequested recognizes explicit continuing preservation, not a
// one-turn question or a scoped instruction to keep only speed unchanged.
func LayeredExactHoldRequested(requests []string) bool {
	for i := len(requests) - 1; i >= 0; i-- {
		message := normalizeMotionIntent(requests[i])
		if motionIntentIsConversation(message) {
			continue
		}
		if hasIntentPhrase(message, "exact pattern", "exact score", "exact repetition", "exactly this", "exactly the same", "no changes from now on", "keep everything unchanged", "keep it unchanged", "stop changing") {
			return true
		}
		if hasIntentPhrase(message, "vary", "varying", "variation", "alternate", "alternating", "stroke", "strokes", "jerk", "tip", "base", "layer", "layers", "speed", "slower", "faster", "gentler", "evolve", "start", "motion") {
			return false
		}
	}
	return false
}

// LayeredContinuationMessage makes explicit user preservation authoritative in
// the autonomous request, rather than competing with an evolution instruction.
func LayeredContinuationMessage(requests []string) string {
	if LayeredExactHoldRequested(requests) {
		return `HOLD EXACT: the user has requested this exact score to keep repeating without changes. Return {"edits":{},"reply":"Keeping the exact score."}. Do not evolve or edit any controls or layers.`
	}
	return layeredAutopilotMessage
}
