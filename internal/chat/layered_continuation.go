package chat

import "strings"

// Autopilot continuity in the continuous modes is judged by the model from the
// recent human messages it is shown. It is not decided by matching those
// messages against a word list: that gate froze whole sessions to seed-only
// refreshes after an ordinary "harder" or "don't stop" while missing "slow down
// a bit", and it could not tell a standing wish from a passing remark. Saved
// limits, validation and Stop remain the hard boundaries.
// continuousPaceGuide lets the model choose pace from how the person responds,
// across the whole saved range. Without it, models eased a few points after
// "that's too much" and climbed back within a couple of stretches.
const continuousPaceGuide = `Pace follows the person and may use all of saved_limits. When they say it is too much, or that they are close and you choose to draw the session out, set speed_percent within a few points of the saved minimum in that same edit, not a few points lower at a time. Hold it there for an appropriate period, judged from how long ago they said it, then rebuild gradually. When you choose to let them finish, hold or build the pace instead. Encode the choice in speed_percent; the reply only describes it.`

const continuousAutopilotJudgment = `AUTOPILOT: you compose the next stretch between chat turns, within saved_limits. First read recent_user_requests_oldest_first, the human's latest words and, when given, how long ago each was said. If they asked to keep the motion exactly as it is, or to stop changing it, and nothing they said afterwards asks for change, return "edits":%s with a brief reply and change nothing, not even speed: that wish outranks every other instruction in this turn, including the pace and variety guidance. Otherwise honor whatever in them still applies, such as a pace, a region or a feel, and develop everything they leave open. Whether words still apply depends on time as well as on later words: a passing state, such as it being too much or asking you to hold off, fades after an appropriate period, often a minute or two from when it was said, while a lasting preference, such as keeping it slow from now on, stays. Use current_score and session observations to choose a coherent direction, then encode the edits needed to reach it; the current score is a starting point, not a set of constraints. Select features for their effect, not to tick off a list. A hold or seed refresh can fit continuity, but a seed refresh alone never changes the character and never answers a request for variety or a surprise. Explore meaningful contrasts over time without a fixed rotation or mandatory change each turn. Choose a few compatible edits, and keep the brief reply faithful to those edits. This is a motion-planning turn, not ordinary conversation.`

func continuousAutopilotMessage(emptyEdits string) string {
	return strings.Replace(continuousAutopilotJudgment, "%s", emptyEdits, 1)
}

// LayeredContinuationMessage is the Layered Autopilot planning policy shared by
// production and the Lab.
func LayeredContinuationMessage() string {
	return continuousAutopilotMessage("{}") + ` Layered can develop reach, location and pace through geometry, controls and layers. A previous model-selected anchor, alternation or layer is not a user constraint; do not keep it merely because it was used before.`
}
