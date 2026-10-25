package chat

import (
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// Keep the complete motion guide. E4B did worse when it was shortened; a
// targeted patch reminder improves omission/preservation without hiding tools.
const simplePatchGuide = "SPARSE PATCH: Write compact JSON without indentation or trailing whitespace. Include only fields you are changing; the saved score is state to preserve, not an object to copy. A speed-only request needs only one speed edit. When geometry supplies a shape, do not recreate its layers. Complete every requested change before the reply, and leave every unrequested setting out of the patch."
const simpleReachGuide = "FINAL EDIT CHECK: Full-device sweeps after a released regional restriction need BOTH outer endpoints 0 and 100. A full-and-tip/base geometry alone keeps the previous band. Broader SHORT strokes change the shortest/local width while preserving the broad strokes and outer band; re-emitting the same geometry does not broaden anything. Exact repetition means empty edits, with no evolve. Only a request for fresh details authorizes evolve. Compare the actual fields with the latest request before describing success."

func simpleInteractiveGuide(caps Capabilities, state MotionContext) string {
	if !caps.SimpleMotion {
		return ""
	}
	if state.Running && !state.Paused {
		return simplePatchGuide + "\n" + simpleReachGuide
	}
	// Reach/repetition refinement is useful during an ongoing pattern. Keep
	// the stopped contract's existing start/intent guidance within 16K instead
	// of adding another reminder beside its larger lifecycle examples.
	return simplePatchGuide
}

func simpleInteractiveGuideForLocale(caps Capabilities, state MotionContext, locale promptLocale) string {
	// Two native-language comparison seeds showed no quality gain for these
	// sets, including a Japanese adaptation. Preserve their established guide.
	// Selection uses the saved prompt set, never a classifier of user language.
	if !simpleContinuous(caps) || locale == promptLocaleJapanese || locale == promptLocalePortugueseBrazil {
		return ""
	}
	return simpleInteractiveGuide(caps, state)
}

// This is a representation decision, not an interpretation of user wording.
func simpleContinuous(caps Capabilities) bool {
	return caps.Motion && caps.SimpleMotion && !caps.AdvancedMotion && !caps.IntermediateMotion &&
		(caps.MotionMode == MotionModeLayered || caps.MotionMode == MotionModeCreativeV2)
}

// The correction already contains precise validator feedback. Omit only our
// optional first-attempt reminder to leave room for that feedback at 16K;
// preserve the full contract, human request, schema and authoritative state.
func simpleRepairInput(input llm.ChatRequest) llm.ChatRequest {
	if len(input.Messages) == 0 || input.Messages[0].Role != "system" {
		return input
	}
	input.Messages = append([]llm.Message(nil), input.Messages...)
	for _, reminder := range []string{simplePatchGuide + "\n" + simpleReachGuide, simplePatchGuide} {
		input.Messages[0].Content = strings.TrimSuffix(input.Messages[0].Content, "\n\n"+reminder)
	}
	return input
}
