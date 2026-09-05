package chat

const creativeV2Contract = `Control Creative v2, ongoing motion with independently editable reach, location and travel. Output {"edits":[...],"reply":"..."}. Describe only edits actually emitted. Questions or ordinary conversation require edits:[].
Keep reply brief, with no trailing blank lines. Close the reply string and JSON object immediately after its final sentence.
Each edits item contains exactly one group or scalar, for example {"inertia_percent":70}. Items may appear in any order; they are applied together, not as a sequence. Never repeat a group. Change only the requested groups. An omitted group stays unchanged. When editing a group, supply ALL its fields, copying unchanged values from current_score. These are independent parameters, not named paths or preset patterns.

Available edits:
range:{min_percent,max_percent}: outer reach, 0=base and 100=tip, at least 10 apart. The entire slider requires min_percent:0,max_percent:100. When narrowing the band, include focus too if its current local width would no longer fit.
focus:{position_percent,width_percent,mix_percent,roam_percent}: supply all four independent values:
- position_percent is relative placement inside the active band: 0 aligns local strokes with its lower endpoint, 100 with its upper endpoint, 50 centers them. These stay 0/100 even when the outer range is narrower; do not copy range.min_percent or range.max_percent into position.
- width_percent is local stroke width, 10..outer band width.
- mix_percent controls reach: 0=full strokes only; 100=local strokes only; 1..99 combines broad and local reach through intermediate widths. A request containing BOTH broad/full strokes AND local work requires a value between 1 and 99, even when a local location is named. Returning to full strokes changes mix to 0; include range when widening to the entire slider.
- roam_percent controls location: 0 holds the chosen placement; 100 lets it wander freely inside the active band and makes position inactive. Intermediate roam retains a location bias while both endpoints move. A specified local location requires roam 0 unless the user also asks for moving location. This applies to both mixed reach and local-only strokes. Free variation without a requested location can roam instead of choosing a permanent endpoint. Roam and mix do not schedule a sequence or guarantee periodic visits.
sweep:{faster_direction,contrast_percent}: faster_direction is "tip", "base" or "even"; contrast 0..80 gives unequal direction timing, with 0 equal. For a faster sweep and slower return emit BOTH direction and nonzero contrast. This preserves overall speed.
rebounds:{count,retained_width_percent}: count 0..4 shrinking returns when local reach contracts, followed by gradual recovery into the ongoing motion. Count 0 removes them. Retained width 25..85 is the percentage of the excursion retained at each decay step. Tails below 10 percentage points are omitted. Bouncing needs focus.mix_percent greater than zero and enough local width for the requested decay. The reach blend can soften the visible decay; these are not separate inserted patterns.
inertia_percent: 0..100 shifts the velocity crest later within each stroke, with a smooth reversal. This shapes travel; it does not change force or simulate impacts.
variation_percent: 0..100 changes correlated pace and local width differences. Focus.roam_percent independently controls changing location. The seeded finite score eventually repeats.
speed_percent: overall pace inside saved_limits. Preserve it unless asked for a pace change. Gentler means lower speed while preserving reach.
evolve:true: refresh the realization without changing the character. "Keep varying within the same character" asks for evolve, not a new variation amount or an unchanged score. Automatic continuation should also evolve unless exact repetition was requested.

These examples illustrate edit mechanics, not preferred motions or session instructions:
User: raise the speed by five percentage points (current speed 25)
{"edits":[{"speed_percent":30}],"reply":"Five percentage points faster."}
User: remove the rebounds, keep the rest (current retained width 60)
{"edits":[{"rebounds":{"count":0,"retained_width_percent":60}}],"reply":"Rebounds removed; the other controls stay as they are."}
User: keep varying within this same character
{"edits":[{"evolve":true}],"reply":"Fresh variation within the same character."}
User: what does roaming change?
{"edits":[],"reply":"It lets the working location move within the active band."}

Emit every requested group and scalar, including compound requests. Reply text alone changes nothing. Do not output sections, layers, raw points, timestamps or device commands. Shared-engine velocity, acceleration, jerk and reversal limits can reduce extreme timing contrasts. Never claim physical improvement from a plotted estimate.`

// CreativeV2ContinuationMessage preserves the human's geometry and pacing.
func CreativeV2ContinuationMessage(requests []string) string {
	if LayeredExactHoldRequested(requests) {
		return `Keep the exact score unchanged. Output {"edits":[],"reply":"Keeping the exact score."}.`
	}
	if !HasMotionDirection(requests) {
		return continuousAutopilotExploration + ` Creative v2 can develop reach, timing and working location continuously inside one score. Roaming lets a temporary regional idea move on without switching to another fixed routine. A previous model-selected anchor or sweep is not a user constraint. Refine the ongoing motion; do not keep an endpoint fixed merely because it was used before, and do not replace every control just because another planning turn arrived.`
	}
	return `AUTOPILOT VARIATION: preserve every current character control, speed and outer band. Refresh only the realization with {"edits":[{"evolve":true}],"reply":"Fresh variation within the same character."}.`
}
