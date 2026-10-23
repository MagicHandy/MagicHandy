# Alpha.71 release review — 2026-10-23

Dates use America/New_York. The maintainer requested merging PRs #344 and #345
after green CI and publishing an update, then explicitly included multi-Handy
support in that update. PRs #344 and #345 are merged. This candidate adds named
Handy connections and the version-specific release preparation on that source.

## Source and acceptance review

The [motion smoothness review](motion-smoothness-review-2026-10-23.md) records
installed-session evidence, Claude Opus 5.5 Max's review and bounded Layered
joins through the existing engine. All 108 matched transition pairs, 45 steady
shaping experiments and 24 real GPT-6 Sol outputs were rendered and inspected.
Four reproduced extra reversals in counted-pattern changes disappear; six
geometry cases keep the existing fallback. Stronger global softness was
rejected because it reduced speed without a consistent smoothness improvement.
Steady curves, transport scheduling, sampler tolerances and local prompts stay
unchanged. Plain-language interruption messages retain redacted technical
evidence and do not turn canceled drafts into motion or correction offers.

The [Edge review](edge-hold-review-2026-10-23.md) records the second completed
Claude consultation, lifecycle fixes and immediate semantic holding control.
The model is used only to plan an Autopilot hold's duration and bounded return.
Manual holds need no inference, and ordinary local prompts/call counts are
unchanged. Edge, Return and Stop share the existing engine, sampler and limits;
Stop retains its independent cancellation path. Late model decisions cannot
resume after newer intent, pause, settings changes or Stop. Disconnects revoke
the hold and automatic recovery. Finite media/programs are excluded.

ChatGPT GPT-6 Sol and local Gemma 4 12B returned 18/18 valid plans and no
refusals across three truthful descriptions and three starting speeds. This
small sample does not establish universal compliance or a wording advantage.
Five variants produced 840 rendered handoff paths, plus 60 steady entries.
The selected variant has 164/168 bounded joins without tiny extra reversals;
remaining cases use the existing fallback. No derivative bound was relaxed.
A fake-transport HTTP run accepted Edge in about 4 ms with about 2 seconds of
points buffered. That measures command acceptance, not physical latency.

Full Go/frontend tests, vet, lint, pure-Go builds, architecture checks and
localization passed during feature review. Linux CI supplies race execution
because this Windows host lacks GCC. The final caption adjustment passed 35
relevant UI tests, typecheck and build. Desktop/mobile visual checks, simulator
Edge/Return, real local-model Autopilot hold/return and an actual text-only chat
without repair/fallback/motion completed on the isolated review app. Canonical
UI and comparable core size changes are recorded in the goal scorecard.

No real hardware was connected or commanded. Physical Handy Cloud comparison,
including buffered response and motion feel, remains outstanding. The later
release request authorizes alpha publication with those limitations disclosed;
it does not establish physical acceptance. The installed app and its data remain
untouched.

## Multi-Handy integration

The [multi-Handy review](multi-handy-review-2026-10-23.md) records Claude's UI
implementation, Codex's integration review, shared-engine startup/dispatch
changes and mixed-connection simulation. Additional Cloud, native Bluetooth and
Intiface Handys have independent configuration and generation-bound owners.
There is still one semantic motion engine. Shared calibration, timing and
sampling honor the slowest participating requirements; connection delays mean
physical synchronization is an estimate. The original Browser Bluetooth bridge
remains a single Handy connection.

The full frontend suite passes 966 tests across 122 files. Full Go tests, vet,
lint, architecture and pure-Go build pass locally. Three simulated protocols
completed three patterns, Stop and repeated Cloud/Intiface reconnects; startup
and mixed-motion command plots were inspected. The review app completed direct
local generation and a text-only app chat with no repair/fallback/motion.
Physical multi-Handy acceptance and long-running radio/network behavior remain
pending. Prior Intiface Stop and loaded electrical/vibration limits still apply.

## Release preparation and boundaries

Only `0.1.0-alpha.71` is added to ADR 0014's reviewed unsigned policy. Alpha.72
and unpublished alpha.65 remain rejected. No dependency, worker protocol, loader,
compression format, public artifact set or installer contract changes. The
version-specific amendment preserves all vulnerability, Defender, provenance,
manifest, checksum and installer-lifecycle gates. Microsoft's prior case does
not approve these new hashes; trusted Authenticode remains the long-term target.

The multi-Handy/release PR and merged-main CI must pass before
tagging. The tag workflow repeats quality checks and scans all five exact
Windows executables, then verifies and exercises the exact public installer on
a disposable host before publication. The three public assets are setup,
portable ZIP and their checksum file. Public notes disclose the unsigned policy,
update instructions, pending physical comparison and existing device limits.
