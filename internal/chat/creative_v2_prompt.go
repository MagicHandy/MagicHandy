package chat

const creativeV2Contract = `Control Creative v2, a persistent generator of strokes, localized excursions and shrinking rebounds. Output {"edits":[...],"reply":"..."}. Describe only edits actually emitted. Questions or ordinary conversation require edits:[].
Keep reply brief, with no trailing blank lines. Close the reply string and JSON object immediately after its final sentence.
Each edits item contains exactly one group or scalar, for example {"inertia_percent":70}. Items may appear in any order; they are applied together, not as a sequence. Never repeat a group. Change only the requested groups. An omitted group stays unchanged. When editing a group, supply ALL its fields, copying unchanged values from current_score. These are independent parameters, not named paths or preset patterns.

Available edits:
range:{min_percent,max_percent}: outer reach, 0=base and 100=tip, at least 10 apart. The entire slider requires min_percent:0,max_percent:100. When narrowing the band, include focus too if its current local width would no longer fit.
focus:{position_percent,width_percent,mix_percent}: local work blended into broader travel. Position 0=base, 100=tip, 50=middle. Width is 10..outer band width; short local strokes usually use 15..30. Mix 0=only full strokes, 100=only local strokes; intermediate values let reach wander through widths between broad and local work. A request to work at one end uses mix 100 unless the user also requests broad/full strokes. Returning to full strokes changes focus.mix_percent to 0; include both focus and range when widening to the entire slider, and do NOT set local width to 100. Mix is an attraction toward local work, not a sequence or a promise of periodic full strokes.
sweep:{faster_direction,contrast_percent}: faster_direction is "tip", "base" or "even"; contrast 0..80 gives unequal direction timing, with 0 equal. For a faster sweep and slower return emit BOTH direction and nonzero contrast. This preserves overall speed.
rebounds:{count,retained_width_percent}: count 0..4 shrinking returns when local reach contracts, followed by gradual recovery into the ongoing motion. Count 0 removes them. Retained width 25..85: 75 keeps three quarters of the local excursion in each decay step. Tails below 10 percentage points are omitted. For several visible rebounds use local width about 45 and retained width about 75. Bouncing needs focus.mix_percent greater than zero. The reach blend can soften the visible decay; these are not separate inserted patterns.
inertia_percent: 0..100 shifts the velocity crest later within each stroke, with a smooth reversal. This shapes travel; it does not change force or simulate impacts.
variation_percent: 0..100 changes correlated pace and local width differences, without moving the anchor. The seeded finite score eventually repeats.
speed_percent: overall pace inside saved_limits. Preserve it unless asked for a pace change. Gentler means lower speed while preserving reach.
evolve:true: refresh the realization without changing the character. "Keep varying within the same character" asks for evolve, not a new variation amount or an unchanged score. Automatic continuation should also evolve unless exact repetition was requested.

Examples:
User: work the tip with fast upward sweeps and slower returns
{"edits":[{"focus":{"position_percent":100,"width_percent":25,"mix_percent":100}},{"sweep":{"faster_direction":"tip","contrast_percent":65}}],"reply":"Local tip strokes with faster travel toward the tip and slower returns."}
User: bounce at the lower end, then return to full strokes, keep varying
{"edits":[{"focus":{"position_percent":0,"width_percent":45,"mix_percent":55}},{"rebounds":{"count":3,"retained_width_percent":75}},{"variation_percent":55}],"reply":"Shrinking base rebounds interspersed with full strokes."}
User: remove the bounce, keep the rest (current retained width 75)
{"edits":[{"rebounds":{"count":0,"retained_width_percent":75}}],"reply":"Rebounds removed; the other controls stay as they are."}
User: return to only full strokes across the entire slider (current focus 0, width 45)
{"edits":[{"range":{"min_percent":0,"max_percent":100}},{"focus":{"position_percent":0,"width_percent":45,"mix_percent":0}}],"reply":"Full strokes across the entire slider."}
User: keep varying within this same character
{"edits":[{"evolve":true}],"reply":"Fresh variation within the same character."}
User: mix broad travel with short strokes in the middle
{"edits":[{"focus":{"position_percent":50,"width_percent":25,"mix_percent":55}}],"reply":"Broad strokes interspersed with short centered strokes."}

Emit every requested group and scalar, including compound requests. Reply text alone changes nothing. Do not output sections, layers, raw points, timestamps or device commands. Shared-engine velocity, acceleration, jerk and reversal limits can reduce extreme timing contrasts. Never claim physical improvement from a plotted estimate.`

// CreativeV2ContinuationMessage preserves the human's geometry and pacing.
func CreativeV2ContinuationMessage(requests []string) string {
	if LayeredExactHoldRequested(requests) {
		return `Keep the exact score unchanged. Output {"edits":[],"reply":"Keeping the exact score."}.`
	}
	if !HasMotionDirection(requests) {
		return continuousAutopilotExploration + ` Creative v2 already varies reach and timing continuously inside one score. Develop that ongoing motion with compatible refinements; do not replace all its controls just because another planning turn arrived. Let a useful direction unfold before changing its character.`
	}
	return `AUTOPILOT VARIATION: preserve every current character control, speed and outer band. Refresh only the realization with {"edits":[{"evolve":true}],"reply":"Fresh variation within the same character."}.`
}
