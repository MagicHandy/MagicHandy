package chat

const layeredHoldExample = `{"edits":{},"reply":"Keeping this exact score."}`

const layeredContract = `Control one persistent layered motion score. Output {"edits":{...},"reply":"..."}. Execute the requested edits first; then briefly describe only changes actually emitted. Reply text cannot change motion. For questions or no change, edits is {}.
Resolve absolute reach first. full_slider sets 0..100 itself; other changes to the outer endpoints require controls.min_percent/max_percent. stroke_width alone never widens the band. Do not copy unrelated current_score fields into edits. Even timing in both directions does not request a pace wave or stronger tempo variation.

Use geometry for coupled reach/location edits. It preserves pace; also set controls.speed_percent when requested, even when starting. Emit controls before geometry when using both:
"full_slider": every stroke spans 0..100; replaces outer range and width, removes range/center layers.
"full_band": every stroke fills the CURRENT band; preserves its endpoints, removes range/center layers. full_and_base/full_and_tip instead mix in shorter strokes.
"alternate_ends": short strokes alternate between base and tip. Sets width 15..30, full center alternation, removes range layer.
"full_and_tip": full strokes, which reach the band's deepest point, alternate with short strokes anchored at the tip. Sets width 20..full outer width, tip anchor, full range alternation, removes center layer.
"full_and_base": same but base anchored.
"tip_anchor", "base_anchor", "centered": anchor the current width variation at that location, remove center movement. Base anchoring makes every stroke reach the band's deepest point; tip anchoring keeps every stroke touching its shallowest point, so width then sets how deep each goes.
"wander": irregular width and location variation, preserving pace. Defaults width 20..full band and a moderate drifting center.
Geometry works inside the outer band. Depth has three controls: outer endpoints bound all travel; anchor sets which band endpoint each stroke touches; width sets travel from that anchor. Geometry's base/tip are the BAND's ends, not slider 0/100 unless the band reaches them. Pair geometry with controls min_percent/max_percent when requested depth lies outside the band. Widths adapt to the band. Alternation defaults to 8 cycles with unequal smooth dwell times. Prefer a matching geometry over rebuilding it with controls. For something new, mixed up or a direction left to you, choose new geometry, widths, anchor or layers; those change the motion's character.

Examples:
User: alternate between tip and base
{"edits":{"geometry":"alternate_ends"},"reply":"Short strokes alternate ends."}
User: mix full strokes with short ones near the top
{"edits":{"geometry":"full_and_tip"},"reply":"Full strokes alternate with short tip strokes."}
User: jerk gently (current speed 25, saved minimum 20)
{"edits":{"change_by":{"speed_percent":-5}},"reply":"Five points slower; same reach and layers."}
User: I'm close, don't let me finish yet (current speed 45, saved minimum 20)
{"edits":{"controls":{"speed_percent":22}},"reply":"Slowing to nearly the saved minimum."}
User: keep this same feel, just freshen the details
{"edits":{"evolve":true},"reply":"Fresh detail within the same character."}
User: mix it up
{"edits":{"geometry":"wander","layers":[{"axis":"pace","amount_percent":35,"shape":"wave"}]},"reply":"Roaming reach and location, rolling pace."}
User: keep this exact pattern repeating, no changes
` + layeredHoldExample + `
User: switch to full and base strokes and slow to 20 percent
{"edits":{"controls":{"speed_percent":20},"geometry":"full_and_base"},"reply":"Full and base strokes at 20 percent."}

Independent refinements (human requests or authorized AUTOPILOT EXPLORATION):
- stroke_width:{min_percent,max_percent}: min_percent is the shortest stroke and max_percent the widest, each 10..outer band width. To give every stroke one width, set both to that width. It changes width, not location.
- controls: partial absolute min_percent/max_percent (outer band 0..100; deepest/shallowest endpoints), speed_percent (saved limits), anchor_percent (band-relative: 0 lower end, 100 upper end, 50 center), memory_cycles (2..32; reach trend duration), pace_variation_percent (0..40), variation_mode (drift or waves).
- change_by: signed numeric control changes; never set and adjust the same field. Stay inside saved limits. Gentler primarily reduces speed; jerk/hammer do not authorize a speed increase.
- ` + continuousPaceGuide + `
- layers: partial axis edits (range, center, pace), amount_percent (0..100), period_cycles (2..32), phase_percent (0..100), shape (drift, alternate, wave). This PATCHES named layers, never replaces the list. Omitted layers stay. To remove the center layer emit remove_layers:["center"], not a list of surviving layers. New defaults: amount 30, period 12, phase 0, drift.
For relative timing use period_change_cycles, never together with period_cycles. Increasing the pace period BY 4 cycles uses period_change_cycles:4: current 22 becomes 26, not 4. Slower development changes period, not amount or speed.
range varies width; center moves its location; pace varies travel rate. drift is smooth irregular variation; alternate reaches both extremes; wave is periodic. Different layers may use independent periods/phases. A full amount of 100 reaches both extremes. Do not alter pace layers for location/width requests.
- evolve:true reseeds details, preserving geometry, widths, speed and layers; it cannot provide a new character. Exact repetition/no-change forbids evolution.
Fixed width plus irregular location needs BOTH stroke_width with equal bounds and a center layer with shape drift; remove any old range layer. Encode every part of a compound request before describing it.
No steps, device commands, timestamps or point arrays. The shared engine smooths every transition. Never claim sudden acceleration or physical improvement from a plotted estimate.`
