package chat

// HasMotionDirection distinguishes a human-authored character from an empty
// Autopilot session. Questions and social conversation do not lock a default
// score forever, and they cannot erase earlier motion directions.
func HasMotionDirection(requests []string) bool {
	for _, request := range requests {
		message := normalizeMotionIntent(request)
		// Refusals and non-English text must not accidentally unlock broader
		// autonomy because this compact English intent vocabulary missed them.
		// Uncertain human directions conservatively preserve the current score.
		if motionIntentIsNegated(message) || userAuthorizesMotion(message, MotionActionUpdate) {
			return true
		}
		for _, r := range message {
			if r > 127 {
				return true
			}
		}
		if motionIntentIsConversation(message) {
			continue
		}
		if hasIntentPhrase(message, "motion", "move", "moving", "stroke", "strokes", "pace", "speed", "range", "reach", "tip", "base", "layer", "layers", "pattern", "score", "repeating", "unchanged", "vary", "variation", "evolve", "bounce", "rebounds", "sweep", "slower", "faster", "gentler", "gentle", "stop changing", "velocidad", "velocidade", "ritmo", "movimiento", "movimento", "punta", "ponta", "manten", "mantem", "manter") {
			return true
		}
	}
	return false
}

const continuousAutopilotExploration = `AUTOPILOT EXPLORATION: no human motion character is locked. Active Autopilot authorizes you to compose the next stretch within saved_limits. Use current_score and session observations to choose a coherent direction, then encode the edits needed to reach it. You may vary pace, location, reach and the mode's independent features; the current score is a starting point, not a set of user constraints. Select features for their effect, not to tick off a list. A hold or seed refresh can fit continuity, but seed refresh alone never changes the character. Explore meaningful contrasts over time without a fixed rotation or mandatory change each turn. Choose a few compatible edits, and keep the brief reply faithful to those edits. This is a motion-planning turn, not ordinary conversation.`

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
	if !HasMotionDirection(requests) {
		return continuousAutopilotExploration
	}
	return layeredAutopilotMessage
}
