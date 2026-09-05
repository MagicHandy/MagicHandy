package chat

import (
	"strconv"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// A reviewed catalog name can identify what an explicit Start refers to.
// Substitution preserves the entire utterance, including negation/questions,
// and sends it through the existing authorization gate. User-authored names
// never become authorization syntax, and this does not choose a movement.
func authorizesNamedContinuousStart(message string, command MotionCommand, capabilities Capabilities, state *MotionContext) bool {
	if !capabilities.Patterns || command.Action != MotionActionStart {
		return false
	}
	recipe, ok := motion.ContinuousRecipeByID(motion.PatternID(command.PatternID), 25)
	if !ok {
		return false
	}
	normalized, name := normalizeMotionIntent(message), normalizeMotionIntent(recipe.Name)
	if !hasIntentPhrase(normalized, name) {
		return false
	}
	return userAuthorizesMotionCommandForCapabilities(strings.ReplaceAll(normalized, name, "pattern"), command, capabilities, state)
}

// A compound preservation request can begin with "keep" before its explicit
// numeric pace edit. This authorizes only that edit on an already running
// library target, never a different shape, area, or a start.
func authorizesPreservedPatternPace(message string, command MotionCommand, capabilities Capabilities, state *MotionContext) bool {
	if !capabilities.Patterns || capabilities.MotionMode == MotionModeDynamic || state == nil || !state.Running ||
		command.Action != MotionActionTarget || command.SpeedPercent == nil ||
		command.PatternID != "" && command.PatternID != state.PatternID || command.Area != "" && command.Area != state.Area {
		return false
	}
	normalized := normalizeMotionIntent(message)
	if !hasIntentPrefix(normalized, "keep ") || motionIntentIsNegated(normalized) || motionIntentIsConversation(normalized) {
		return false
	}
	pace := strconv.Itoa(*command.SpeedPercent)
	return hasIntentPhrase(normalized, "change only speed to "+pace, "change only pace to "+pace,
		"change only the speed to "+pace, "change only the pace to "+pace)
}
