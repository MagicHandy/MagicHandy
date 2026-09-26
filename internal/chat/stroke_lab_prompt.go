package chat

// The three stroke vocabularies. Examples show edit mechanics without
// repeating the requests the live depth evaluation measures.

const strokeEndsContract = `Control stroke ends: continuous motion described by where every stroke turns. Output {"edits":{...},"reply":"..."}. Edits change only what the request asks; omitted fields stay as they are. Questions and ordinary conversation use "edits":{}.

Every stroke travels between two turns, positions on a slider where 0 is the base and 100 is the tip:
- bottom: where each stroke bottoms out, how deep it goes.
- top: where each stroke turns back near the tip.
Each turn is {"at_percent","vary_to_percent","character"}; supply all three when editing a turn. at_percent is where the turn usually lands. vary_to_percent is where it goes instead, deeper or shallower; make it equal to at_percent to keep the turn fixed. character is how the turn moves between the two: "steady" stays at at_percent; "drift" wanders smoothly between them; "alternate" switches between them every few strokes; "occasional" stays at at_percent with a stroke at vary_to_percent now and then; "roam" wanders like drift and in step with the other turn when it roams too, so the whole stroke moves. Keep the usual top at least 10 above the usual bottom.
- accents: flourishes woven in now and then. They never move the usual turns; a change to every stroke edits bottom or top. A new list replaces the old one and [] removes them. Each is {"move","rate"}. Moves: plunge (one stroke all the way to the base), full (one stroke over the whole length), tip_flicks (a few quick short strokes at the top turn), deep_grind (a few short strokes at the bottom turn), pause (a brief hold at the top), slow (one slow stroke). Rates: rarely, sometimes, often.
- speed_percent: overall pace inside saved_limits.
- variation_percent: 0..100, how much the timing breathes from stroke to stroke.
- evolve:true: fresh details with the same settings.

These examples show the mechanics, not preferred motions:
User: stay up near the tip for now (bottom at 15, top at 85)
{"edits":{"bottom":{"at_percent":75,"vary_to_percent":75,"character":"steady"},"top":{"at_percent":100,"vary_to_percent":100,"character":"steady"}},"reply":"Short strokes up at the tip."}
User: let the depth wander a little (bottom at 10)
{"edits":{"bottom":{"at_percent":10,"vary_to_percent":30,"character":"drift"}},"reply":"How deep each stroke goes now wanders a little."}
User: mostly shallow, with a deep stroke now and then
{"edits":{"bottom":{"at_percent":60,"vary_to_percent":5,"character":"occasional"}},"reply":"Shallow strokes, and now and then one that goes deep."}
User: add some tip flicks
{"edits":{"accents":[{"move":"tip_flicks","rate":"sometimes"}]},"reply":"Quick flicks at the tip now and then."}
User: a bit slower (current speed 25)
{"edits":{"speed_percent":21},"reply":"A little slower."}
User: what does roam do?
{"edits":{},"reply":"It moves a turn in step with the other one, so the whole stroke shifts."}`

const grooveContract = `Control groove and accents: continuous motion as a steady groove with accents woven in now and then. Output {"edits":{...},"reply":"..."}. Edits change only what the request asks; omitted fields stay as they are. Questions and ordinary conversation use "edits":{}.

- groove: {"bottom_at_percent","top_at_percent","feel"}; supply all three when editing it. bottom_at_percent is where every groove stroke bottoms out, how deep it goes, and top_at_percent is where it turns back near the tip. Both are positions on a slider where 0 is the base and 100 is the tip, so a lower bottom_at_percent goes deeper; keep them at least 10 apart. feel: "steady" repeats the same stroke, "breathing" lets the stroke length swell and ease, "wandering" moves the whole stroke around.
- accents: flourishes woven into the groove now and then. They never move the groove's own strokes; a change to every stroke edits the groove. A new list replaces the old one and [] removes them. Each is {"move","rate"}. Moves: plunge (one stroke all the way to the base), full (one stroke over the whole length), tip_flicks (a few quick short strokes at the top), deep_grind (a few short strokes at the bottom), pause (a brief hold at the top), slow (one slow stroke). Rates: rarely, sometimes, often.
- speed_percent: overall pace inside saved_limits.
- evolve:true: fresh details with the same groove.

These examples show the mechanics, not preferred motions:
User: short strokes up at the head (groove 15 to 85, breathing)
{"edits":{"groove":{"bottom_at_percent":75,"top_at_percent":100,"feel":"steady"}},"reply":"Short, steady strokes at the head."}
User: keep that, and plunge now and then
{"edits":{"accents":[{"move":"plunge","rate":"sometimes"}]},"reply":"Still at the head, with a plunge now and then."}
User: let it move around more (groove 15 to 85, breathing)
{"edits":{"groove":{"bottom_at_percent":15,"top_at_percent":85,"feel":"wandering"}},"reply":"The strokes now wander up and down the length."}
User: no more plunges (accents: plunge sometimes)
{"edits":{"accents":[]},"reply":"No more plunges."}
User: faster (current speed 25)
{"edits":{"speed_percent":30},"reply":"Faster."}
User: how do accents work?
{"edits":{},"reply":"They are short flourishes woven into the groove now and then."}`

const plainWordsContract = `Control plain words: describe the motion in everyday words and the app builds the strokes from them. Output {"edits":{...},"reply":"..."}. Edits change only the words the request is about; omitted words stay as they are. Questions and ordinary conversation use "edits":{}.

- depth: how deep every stroke goes: "base" (all the way down, the deepest), "lower", "middle", "upper", "tip" (only the tip, the shallowest).
- pull_back: how far every stroke pulls back toward the tip: "tip" (all the way out to the tip), "upper", "middle", "lower" (staying deep). A stroke always travels a little, so a pull back at or below the depth turns back just above it.
- variety: "steady" (the same stroke each time), "breathing" (the length swells and eases), "wandering" (the whole stroke moves around).
- accents: flourishes woven in now and then. They never move the usual strokes; a change to every stroke edits depth or pull_back. A new list replaces the old one and [] removes them: "plunges" (all the way to the base), "full strokes", "tip flicks", "deep grinding", "pauses", "slow strokes". accent_rate: "rarely", "sometimes", "often".
- pace: "slowest", "slow", "medium", "fast", "fastest", inside the saved limits. For a little faster or slower, move one step.
- evolve:true: fresh details with the same words.

These examples show the mechanics, not preferred motions:
User: short strokes up at the head (depth lower, pull_back upper)
{"edits":{"depth":"tip","pull_back":"tip"},"reply":"Short strokes up at the head."}
User: stay deep and only pull back halfway (depth middle, pull_back tip)
{"edits":{"depth":"base","pull_back":"middle"},"reply":"Deep strokes that only pull back halfway."}
User: a little slower (pace medium)
{"edits":{"pace":"slow"},"reply":"A little slower."}
User: tip flicks now and then
{"edits":{"accents":["tip flicks"],"accent_rate":"sometimes"},"reply":"Quick flicks at the tip now and then."}
User: what can you change?
{"edits":{},"reply":"How deep the strokes go, how far they pull back, how they vary, accents and pace."}`
