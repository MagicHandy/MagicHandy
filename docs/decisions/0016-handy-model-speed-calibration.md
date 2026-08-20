# ADR 0016: Explicit Handy Model Speed Calibration

## Status

Accepted and implemented. Matched real-device confirmation remains open.

## Context

MagicHandy's first shape-independent loop-speed model used one semantic travel
rate for every device. That removed pattern-dependent pace, but its 100% value
was the moderate authored `Stroke` rate rather than a physical device envelope.
Users consequently found familiar percentages materially slower than in other
Handy applications.

One universal semantic scale is also physically ambiguous across Handy
generations. The original Handy publishes 110 mm travel and 32–400 mm/s. Handy
2 Standard publishes 125 mm and 32–400 mm/s; Handy 2 Pro publishes 125 mm and a
32–450 mm/s normal range. The same semantic percentage-points/second therefore
cannot mean the same carriage velocity on all three.

The manufacturer does not publish per-model acceleration limits. The Handy 2
Pro page also advertises an optional 800 mm/s overclock, but neither its
acceleration envelope nor a cross-model safety validation is available here.
Motor RPM and marketing descriptions are not sufficient inputs to a hardware
safety limit.

## Decision

Persist one required `handy_model` calibration profile in motion settings and
expose it as three merged radio buttons in the shell-owned Connection menu:

| value | travel | normal speed |
| --- | ---: | ---: |
| `handy_original` | 110 mm | 32–400 mm/s |
| `handy_2_standard` | 125 mm | 32–400 mm/s |
| `handy_2_pro` | 125 mm | 32–450 mm/s |

Existing and missing settings default to `handy_original`, preserving the
installed-base calibration. The backend publishes and validates the allowed
values. The UI labels the buttons `Original`, `2 Standard`, and `2 Pro`, then
shows the selected profile's travel and normal maximum immediately underneath
so the compact choice remains inspectable.

The shared engine converts a selected 1–100 speed to semantic travel using:

```text
progress = (speed_percent - 1) / 99
physical_mm_per_second = minimum + (maximum - minimum) × progress
semantic_percent_per_second = physical_mm_per_second / travel_mm × 100
```

This changes plan timing only. Motion targets and samples remain semantic
0–100, transport interfaces remain unchanged, and no transport gains a private
sampler or model-specific payload. A live profile change follows the existing
settings retarget path. When the optional media speed cap is active, changing
the profile Stops that clock-locked run because accepted points cannot be
rewritten; uncapped media is unaffected.

The exact-curve runtime acceleration and reversal envelopes remain shared.
The Pro overclock range is not offered. A future per-model acceleration change
requires manufacturer data or bounded instrumented hardware evidence and an
explicit update to this decision.

## Consequences

Positive:

- a percentage maps to a documented physical range instead of one historical
  pattern cadence;
- Original and Handy 2 Standard produce the same calculated mm/s at the same
  percentage despite different travel;
- the model choice is visible beside the controls it calibrates and applies
  through the existing backend-authoritative path;
- transport and Stop invariants remain unchanged.

Negative:

- automatic model detection is not available across Cloud, Browser Bluetooth,
  and Intiface, so the user must select Handy 2 explicitly;
- a non-Handy Intiface linear actuator still has no verified physical profile;
- the selector cannot account for undocumented acceleration differences.

## Rejected Alternatives

- **Infer acceleration from motor RPM or product language.** Rejected as an
  unsupported safety claim.
- **Expose Handy 2 Pro overclocking.** Rejected without an acceleration envelope
  and matched safety evidence.
- **Calibrate inside each transport.** Rejected because changing point timing at
  dispatch would create multiple motion models and break the one-engine path.
- **Silently assume one generation.** Rejected because 110 mm and 125 mm travel
  make the same semantic rate physically different.

## Evidence Boundary

Automated tests cover profile defaults and validation, endpoint conversion,
equal physical rates for Original and Handy 2 Standard, plan-level use of the
selected profile, immediate persistence, media-cap invalidation, and unchanged
shared runtime safety bounds. No post-change hardware command was issued. The
matched acceptance run must record selected profile, actual device model,
transport, speed/span, latency, trace, Stop behavior, and subjective feel.
