//go:build liveeval

package chat

import "strings"

const simpleLayeredTrial = `Control the saved Layered score with a partial edit, not a replacement score. Output action, edits and a brief reply. Omit every unchanged field. Use action none with edits:{} for questions or exact repetition. Start/update authority comes from the current backend state.

Choose only the groups needed:
- controls:{min_percent,max_percent}: outer travel endpoints, 0=base and 100=tip. Set BOTH endpoints when changing reach. The old endpoints are not limits. Full-device travel always needs 0 and 100.
- controls:{speed_percent}: overall speed inside saved_limits. change_by:{speed_percent:n} adds a signed number to current speed. Preserve all geometry for pace-only requests; gentle means slower, brisk means faster. Do not change speed just for new reach or texture.
- geometry is a coordinated change that preserves pace: full_slider sets every stroke to 0..100; full_band fills the existing band; full_and_tip/full_and_base mix full-band sweeps with shorter strokes anchored at that end; alternate_ends alternates short strokes at both band ends; tip_anchor/base_anchor/centered anchor the current width variation; wander lets width and position vary freely.
- stroke_width:{min_percent,max_percent}: shortest and widest stroke, each 10..outer band width. Equal bounds fix the width. To broaden only short strokes, increase the minimum and preserve the maximum. It never expands outer endpoints.
- layers:[{axis,partial fields}]: patch only named range/center/pace modulation. amount_percent 0..100; period_cycles 2..32; phase_percent 0..100; shape drift,alternate,wave. Longer period means slower development, not slower travel. period_change_cycles adds to the existing period. remove_layers:[axis] removes a named layer.
- controls may also change anchor_percent (0=lower band end,100=upper), memory_cycles (2..32), pace_variation_percent (0..40), variation_mode (drift/waves). Omit these unless changing them.
- evolve:true refreshes seeded details without changing the pattern's character. Exact repetition needs empty edits, not evolve.

Geometry manages its own range and center layers. Do not re-add an old center layer to an anchored or full-width geometry: that contradicts it. Pace layers remain independent. A fixed width that wanders needs equal width bounds, a drifting center layer and removal of the range layer.
For a full-device sweep mixed with short tip strokes, use controls:{min_percent:0,max_percent:100} plus geometry:full_and_tip. The geometry alone keeps the old band. For a requested 65..100 band, set controls:{min_percent:65,max_percent:100}; don't emit just one endpoint. These are examples of requested values, not defaults.
Encode every part of a compound request. New instructions replace older conflicting requests. Preserve only what the latest request leaves unchanged. No exact counted routines, raw points or device commands. Write compact JSON; don't restate the saved score. Describe only the edits you actually emitted. Close JSON immediately after the reply. Never claim physical smoothness from these estimates.`

const simpleCreativeTrial = `Control the saved Creative v2 score with a partial patch. Output action, edits:[...], reply. Each edit item contains ONE group or scalar. Omit unchanged groups. Supply every field of a group you edit. Questions and exact repetition use action none and edits:[]; start/update authority comes from backend state.

- range:{min_percent,max_percent}: absolute outer endpoints, 0=base and 100=tip, at least 10 apart. Every stroke stays inside this band. Full-device travel requires BOTH 0 and 100, even after an earlier region restriction. The current band is not a saved limit.
- focus:{position_percent,width_percent,mix_percent,roam_percent}: position 0 anchors local strokes at the lower band end,100 at the upper end. Width is 10..band width. Mix 0=every stroke fills the band;100=only local strokes;between combines full-band and shorter strokes. Roam 0 fixes that position;100 lets it wander. For an anchored tip/base use position100/0 AND roam0. Preserve the other focus values when changing just one.
- speed_percent: overall pace within saved_limits. For a relative request, compute from current_score. Gentle means slower and brisk means faster; preserve reach and pattern when changing pace. Reach requests do not change speed.
- variation_percent:0..100 changes stroke-to-stroke variation,0 repeats exactly. Roam separately controls location. evolve:true refreshes details without changing any controls. Exact repetition needs no edits, not evolve.
- inertia_percent:0..100 shifts the velocity peak later in each stroke, preserving smooth reversals.
- sweep:{faster_direction,contrast_percent}: even with0,or tip/base with1..80 for faster travel toward that end. This is direction timing, not overall pace.
- rebounds:{count,retained_width_percent}:0..4 shrinking returns retaining25..85 percent; count0 removes them. Positive rebounds need focus.mix_percent above0 and enough local width. These are not exact repeating stroke counts.

Full-device sweeps mixed with short tip strokes need range0..100 and focus mix between0 and100,position100,roam0. To widen only the short strokes, raise focus.width_percent and preserve all other focus values and range. To release a tip focus, allow roam; retain mixed reach when both stroke lengths and location should wander. Every stroke over the full slider needs range0..100 and focus.mix_percent0.
Apply every requested group. The latest request replaces older conflicting restrictions. Preserve unmentioned controls. Write compact JSON, never a copy of the whole score. Describe only applied edits, then close JSON. No raw points, timestamps, device commands or invented physical effects.`

func simpleTrialContract(text string) string {
	if start := strings.Index(text, "Control one persistent layered motion score."); start >= 0 {
		endText := "Never claim sudden acceleration or physical improvement from a plotted estimate."
		if end := strings.Index(text, endText); end >= start {
			return text[:start] + simpleLayeredTrial + text[end+len(endText):]
		}
	}
	if start := strings.Index(text, "Control Creative v2, ongoing motion"); start >= 0 {
		endText := "Never claim physical improvement from a plotted estimate."
		if end := strings.Index(text, endText); end >= start {
			return text[:start] + simpleCreativeTrial + text[end+len(endText):]
		}
	}
	return text
}
