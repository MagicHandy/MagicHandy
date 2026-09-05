package httpapi

import (
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func dynamicChatMotionTarget(command *chat.MotionCommand, current motion.ActiveMotionState) motion.MotionTarget {
	var currentDynamic *motion.DynamicDefinition
	if current.Running && current.Target.Dynamic != nil {
		currentDynamic = current.Target.Dynamic
	}
	dynamic := dynamicDefinitionFromCommand(command, currentDynamic)
	speed := 0
	if command.SpeedPercent != nil {
		speed = *command.SpeedPercent
	} else if current.Running {
		speed = current.Target.SpeedPercent
	}
	return motion.MotionTarget{
		Label: "Chat", Source: "chat", SpeedPercent: speed, Dynamic: &dynamic,
	}
}

func dynamicDefinitionFromCommand(command *chat.MotionCommand, current *motion.DynamicDefinition) motion.DynamicDefinition {
	dynamic := motion.NormalizeDynamicDefinition(motion.DynamicDefinition{})
	if current != nil {
		dynamic = motion.NormalizeDynamicDefinition(*current)
	}
	if len(command.Sections) > 0 {
		return dynamicDefinitionFromSections(command, dynamic, current)
	}
	if len(dynamic.Sections) > 0 && commandChangesSingleDynamicPhrase(command) {
		dynamic = collapseDynamicPhrase(dynamic)
	}
	next := applySingleDynamicCommand(command, dynamic)
	if current == nil || commandChangesSingleDynamicPhrase(command) {
		next = motion.FreshDynamicPhrase(next, dynamic.PhraseSeed)
	}
	return next
}

func dynamicDefinitionFromSections(
	command *chat.MotionCommand,
	dynamic motion.DynamicDefinition,
	current *motion.DynamicDefinition,
) motion.DynamicDefinition {
	dynamic.Sections = make([]motion.DynamicSection, 0, len(command.Sections))
	for _, section := range command.Sections {
		dynamic.Sections = append(dynamic.Sections, dynamicSectionFromCommand(section))
	}
	dynamic.PhraseSeed = 0
	if command.SegmentSeconds != nil {
		dynamic.SegmentSeconds = *command.SegmentSeconds
	}
	previous := uint32(0)
	if current != nil {
		previous = current.PhraseSeed
	}
	return motion.FreshDynamicPhrase(dynamic, previous)
}

// collapseDynamicPhrase edits the currently effective first section. Speed-
// only and horizon-only updates never call it and preserve the complete phrase.
func collapseDynamicPhrase(dynamic motion.DynamicDefinition) motion.DynamicDefinition {
	first := dynamic.Sections[0]
	return motion.DynamicDefinition{
		CenterPercent: first.CenterPercent, SpanPercent: first.SpanPercent,
		SpanMinPercent: first.SpanMinPercent, SpanProfile: first.SpanProfile,
		Anchors:          append([]motion.DynamicAnchor(nil), first.Anchors...),
		VariationPercent: first.VariationPercent, SegmentSeconds: dynamic.SegmentSeconds,
	}
}

func applySingleDynamicCommand(
	command *chat.MotionCommand,
	dynamic motion.DynamicDefinition,
) motion.DynamicDefinition {
	envelopeChanged := false
	if len(command.Anchors) > 0 {
		dynamic.Anchors = make([]motion.DynamicAnchor, 0, len(command.Anchors))
		for _, name := range command.Anchors {
			if position, ok := chat.DynamicAnchorPosition(name); ok {
				dynamic.Anchors = append(dynamic.Anchors, motion.DynamicAnchor{Name: name, PositionPercent: position})
			}
		}
		envelopeChanged = true
	} else if command.CenterPercent != nil || command.SpanPercent != nil {
		// A center/span update intentionally leaves an anchor route. The current
		// normalized bounds provide sensible omitted-field preservation.
		dynamic.Anchors = nil
		if command.CenterPercent != nil {
			dynamic.CenterPercent = *command.CenterPercent
		}
		if command.SpanPercent != nil {
			dynamic.SpanPercent = *command.SpanPercent
		}
		envelopeChanged = true
	}
	if command.SpanMinPercent != nil {
		dynamic.SpanMinPercent = *command.SpanMinPercent
		if strings.TrimSpace(command.SpanProfile) == "" &&
			(dynamic.SpanProfile == "" || dynamic.SpanProfile == motion.DynamicSpanProfileSteady) {
			dynamic.SpanProfile = motion.DynamicSpanProfileWander
		}
		envelopeChanged = true
	}
	if strings.TrimSpace(command.SpanProfile) != "" {
		dynamic.SpanProfile = strings.ToLower(strings.TrimSpace(command.SpanProfile))
		envelopeChanged = true
	}
	if command.VariationPercent != nil {
		dynamic.VariationPercent = *command.VariationPercent
	}
	if command.SegmentSeconds != nil {
		dynamic.SegmentSeconds = *command.SegmentSeconds
	}
	if envelopeChanged {
		dynamic.PhraseSeed = 0
	}
	return motion.NormalizeDynamicDefinition(dynamic)
}

func commandChangesSingleDynamicPhrase(command *chat.MotionCommand) bool {
	return command.CenterPercent != nil || command.SpanPercent != nil || len(command.Anchors) > 0 ||
		command.SpanMinPercent != nil || strings.TrimSpace(command.SpanProfile) != "" ||
		command.VariationPercent != nil
}

func dynamicSectionFromCommand(section chat.DynamicSectionCommand) motion.DynamicSection {
	converted := motion.DynamicSection{Cycles: section.Cycles, SpanProfile: section.SpanProfile}
	if section.CenterPercent != nil {
		converted.CenterPercent = *section.CenterPercent
	}
	if section.SpanPercent != nil {
		converted.SpanPercent = *section.SpanPercent
	}
	if section.SpanMinPercent != nil {
		converted.SpanMinPercent = *section.SpanMinPercent
		if strings.TrimSpace(converted.SpanProfile) == "" {
			converted.SpanProfile = motion.DynamicSpanProfileWander
		}
	}
	if section.VariationPercent != nil {
		converted.VariationPercent = *section.VariationPercent
	}
	for _, name := range section.Anchors {
		if position, ok := chat.DynamicAnchorPosition(name); ok {
			converted.Anchors = append(converted.Anchors, motion.DynamicAnchor{
				Name: name, PositionPercent: position,
			})
		}
	}
	return converted
}
