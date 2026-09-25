package chat

import "strings"

// Autopilot continuity in the continuous modes is judged by the model from the
// recent human messages it is shown. It is not decided by matching those
// messages against a word list: that gate froze whole sessions to seed-only
// refreshes after an ordinary "harder" or "don't stop" while missing "slow down
// a bit", and it could not tell a standing wish from a passing remark. Saved
// limits, validation and Stop remain the hard boundaries.
const continuousAutopilotJudgment = `AUTOPILOT: you compose the next stretch between chat turns, within saved_limits. First read recent_user_requests_oldest_first, the human's latest words. If they asked to keep the motion exactly as it is, or to stop changing it, and nothing they said afterwards asks for change, return "edits":%s with a brief reply and change nothing, not even speed: that wish outranks every other instruction in this turn, including the pace and variety guidance. Otherwise honor whatever in them still applies, such as a pace, a region or a feel, and develop everything they leave open. Use current_score and session observations to choose a coherent direction, then encode the edits needed to reach it; the current score is a starting point, not a set of constraints. Select features for their effect, not to tick off a list. A hold or seed refresh can fit continuity, but a seed refresh alone never changes the character. Explore meaningful contrasts over time without a fixed rotation or mandatory change each turn. Choose a few compatible edits, and keep the brief reply faithful to those edits. This is a motion-planning turn, not ordinary conversation.`

func continuousAutopilotMessage(emptyEdits string) string {
	return strings.Replace(continuousAutopilotJudgment, "%s", emptyEdits, 1)
}

// LayeredContinuationMessage is the Layered Autopilot planning policy shared by
// production and the Lab.
func LayeredContinuationMessage() string {
	return continuousAutopilotMessage("{}") + ` Layered can develop reach, location and pace through geometry, controls and layers. A previous model-selected anchor, alternation or layer is not a user constraint; do not keep it merely because it was used before.`
}
