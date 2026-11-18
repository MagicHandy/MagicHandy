package chat

import "fmt"

const localBandExamplePrefix = "First decide whether motion is requested NOW."

const localCreativeReachGuide = `Confining motion to W points at one end sets the outer range as well as the local focus. At the tip use (100 minus W)..100, relative position 100, width W, mix 100 and roam 0. At the base use 0..W and position 0. When the next request combines full-device strokes and local returns, restore 0..100, switch focus to the newly named end, and use a mixed reach. Keeping the same repeating loop preserves its variation and seed: action none with empty edits. Zeroing variation would change the loop.`

// Use the selected tier's JSON shape and a state-valid action. The values are
// examples; neither the model's response nor the human's words are rewritten.
func localRequestedMotionGuide(caps Capabilities, state MotionContext) string {
	if caps.MotionMode == MotionModeCreativeV2 {
		if caps.SimpleMotion {
			return ""
		}
		return localCreativeReachGuide
	}
	if caps.MotionMode != MotionModeLayered {
		return ""
	}
	action := MotionActionStart
	if state.Running {
		action = MotionActionUpdate
	}
	speed := max(33, state.SpeedMinPercent)
	if state.SpeedMaxPercent > 0 {
		speed = min(speed, state.SpeedMaxPercent)
	}
	return fmt.Sprintf(localBandExamplePrefix+` Saving, correcting or reading a definition is speech only. When asked to play a corrected definition, use its latest explicit numbers. Full strokes between two marks touch both marks on EVERY pass. Send the specified outer endpoints and full_band together, including when narrowing a full-stroke band. full_slider sets 0 and 100. A mixture adds short strokes. Example for full passes from 17 to 69 at overall speed %d: {"action":"%s","edits":[{"range":{"min_percent":17,"max_percent":69}},{"geometry":"full_band"},{"controls":{"speed_percent":%d}}],"reply":"Applying the requested sweep."} A speed-only edit uses only speed. A shift of the whole interval moves both current endpoints by the requested amount.`, speed, action, speed)
}
