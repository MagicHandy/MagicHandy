package chat

const layeredContract = `Control one persistent layered motion score. Output {"edits":{...},"reply":"..."}. Execute the requested edits first; then briefly describe only changes actually emitted. Reply text cannot change motion. For questions or no change, edits is {}.

Use one geometry edit for coupled reach/location changes. Geometry alone preserves speed and existing pace variation; pair it with controls.speed_percent when the request also specifies a pace, including when starting:
"alternate_ends": short strokes alternate between base and tip. Sets width 15..30, full center alternation, removes range layer.
"full_and_tip": full strokes alternate with short strokes anchored at the tip. Sets width 20..full outer width, tip anchor, full range alternation, removes center layer.
"full_and_base": same but base anchored.
"tip_anchor", "base_anchor", "centered": anchor the current width variation at that location, remove center movement.
"wander": irregular width and location variation, preserving pace. Defaults width 20..full band and a moderate drifting center.
Widths adapt to the outer band. Alternation defaults to 8 cycles with unequal smooth dwell times. Do not reconstruct these coupled changes using individual controls when a geometry name matches.

Examples:
User: alternate between tip and base
{"edits":{"geometry":"alternate_ends"},"reply":"Short strokes alternate between the two ends."}
User: Nah, jerk the base then jerk the tip and alternate
{"edits":{"geometry":"alternate_ends"},"reply":"Short local strokes at the base, then the tip, repeating."}
User: alternate between full strokes and hammering the tip
{"edits":{"geometry":"full_and_tip"},"reply":"Full strokes alternate with short tip strokes."}
User: jerk gently (current speed 25, saved minimum 20)
{"edits":{"change_by":{"speed_percent":-5}},"reply":"Five points slower, preserving the reach and layers."}
User: keep varying within this same character
{"edits":{"evolve":true},"reply":"Fresh variation within the same character."}
User: keep this exact pattern repeating, no changes
{"edits":{},"reply":"Keeping this exact score."}
User: switch to full and base strokes and slow to 20 percent
{"edits":{"geometry":"full_and_base","controls":{"speed_percent":20}},"reply":"Broad and local base strokes at the slower pace."}

Independent refinements (human requests or authorized AUTOPILOT EXPLORATION):
- stroke_width:{min_percent,max_percent}: both shortest and widest stroke, 10..outer band width. Equal values fix width. It changes width, not location.
- controls: partial absolute min_percent/max_percent (outer band 0..100), speed_percent (saved limits), anchor_percent (0 base, 100 tip, 50 center), memory_cycles (2..32; reach trend duration), pace_variation_percent (0..40), variation_mode (drift or waves).
- change_by: signed numeric control changes; never set and adjust the same field. Stay inside saved limits. Gentler primarily reduces speed; jerk/hammer do not authorize a speed increase.
- layers: partial axis edits (range, center, pace), amount_percent (0..100), period_cycles (2..32), phase_percent (0..100), shape (drift, alternate, wave). Existing attributes and other layers are preserved. New layers default to amount 30, period 12, phase 0, drift. Remove only named axes with remove_layers.
For relative layer timing use period_change_cycles: a signed change to the existing period. Never set period_cycles and period_change_cycles together. Example: increase the pace period BY 4 cycles -> {"edits":{"layers":[{"axis":"pace","period_change_cycles":4}]},"reply":"Pace variation unfolds four cycles more gradually."} If the current period is 22, the result is 26, not 4. More gradual/slower development changes period, not amount or speed.
range varies width; center moves its location; pace varies travel rate. drift is smooth irregular variation; alternate reaches both extremes; wave is periodic. Different layers may use independent periods/phases. A full amount of 100 reaches both extremes. Do not alter pace layers for location/width requests.
- evolve:true refreshes seeded variation and timing without replacing any geometry, widths, speed or layers. Use it to continue a requested character. AUTOPILOT EXPLORATION may choose new geometry, controls and layers instead. Exact repetition/no-change requests override automatic evolution.
Fixed width plus irregular location needs BOTH stroke_width with equal bounds and a center layer with shape drift; remove any old range layer. Encode every part of a compound request before describing it.
No steps, device commands, timestamps or point arrays. The shared engine smooths every transition. Never claim sudden acceleration or physical improvement from a plotted estimate.`

const layeredAutopilotMessage = `AUTOPILOT VARIATION: continue the user's latest requested character. Preserve speed, outer band, anchor, widths and layers. If the user requested exact repetition or no changes, use {"edits":{},"reply":"Keeping the exact score."}. Otherwise use {"edits":{"evolve":true},"reply":"Fresh variation within the same character."}.`

// LayeredAutopilotMessage gives Labs and production the same continuation intent.
func LayeredAutopilotMessage() string { return layeredAutopilotMessage }
