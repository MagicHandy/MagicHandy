package chat

import (
	"errors"
	"fmt"
	"strings"
)

const hostedStartGuide = `When stopped, use "start" for a fresh request to perform motion, even without saying "start" or after an earlier Stop. Questions or explicitly prospective/settings-only requests use "none". While paused, only the app can resume.`

// These replacements affect code-owned instructions only. The hosted model
// interprets user intent; no words in a user message grant start authority.
func hostedStartContract(text string, capabilities Capabilities) string {
	if !capabilities.HostedMotion {
		return text
	}
	text = strings.Replace(text, `Use "start" only when the user asks to begin motion.`, hostedStartGuide, 1)
	return strings.Replace(text, `Use "start" only for an explicit request to begin motion.`, hostedStartGuide, 1)
}

func hostedStoppedGuidance(text string, capabilities Capabilities) string {
	if !capabilities.HostedMotion {
		return text
	}
	text = strings.Replace(text, `use action "start" only for an explicit motion request`, `use action "start" for a fresh request to perform motion`, 1)
	return strings.Replace(text, `use action "start" for an explicit motion request`, `use action "start" for a fresh request to perform motion`, 1)
}

func (s Service) validateHostedStartState(response AssistantResponse, capabilities Capabilities) error {
	if !capabilities.HostedMotion || s.TrustedMotionInput || response.Motion == nil || response.Motion.Action != MotionActionStart {
		return nil
	}
	if s.MotionContext == nil || capabilities.MotionHolder != MotionHolderSettings {
		return errors.New("a hosted start requires an authoritative chat-owned motion state")
	}
	if s.MotionContext.Paused {
		return errors.New("motion is paused; only the app can resume it")
	}
	if s.MotionContext.Running {
		return errors.New("motion is already running; a new start is invalid")
	}
	speed := response.Motion.SpeedPercent
	if speed == nil {
		speed = response.Motion.Intensity
	}
	limits := normalizedPromptMotionContext(*s.MotionContext).SpeedLimits
	if speed != nil && (*speed < limits.Min || *speed > limits.Max) {
		return fmt.Errorf("start speed must be within saved limits %d-%d", limits.Min, limits.Max)
	}
	return nil
}

// This grants no inferred start: the model must emit a complete valid start
// command for an interactive hosted turn that actually owns stopped motion.
func (s Service) authorizesHostedStart(command MotionCommand, capabilities Capabilities) bool {
	return capabilities.HostedMotion && capabilities.Motion && capabilities.MotionHolder == MotionHolderSettings &&
		!s.TrustedMotionInput && command.Action == MotionActionStart && s.MotionContext != nil &&
		!s.MotionContext.Running && !s.MotionContext.Paused
}
