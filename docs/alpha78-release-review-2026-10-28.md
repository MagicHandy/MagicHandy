# Alpha.78 release review — 2026-10-28

## Source and authorization

The maintainer requested an integrity check after a power interruption, an
update and handoff to Updates. The source thread supplied PR #355 (Terra
default, `7e57d6d1`) and stacked PR #356 (continuity, `c7f7111d`). Updates
continues in the existing `MagicHandy-vibrator-level` worktree. Remote discovery
found alpha.77 and main `892d555b`; alpha.78 was unused. Both changes must merge
through reviewed green PRs before the immutable release tag is placed on the
current merged main commit. No unrelated checkout or running session is replaced.

## Integrity and source review

After the interruption, the source thread found one new test file filled with
NUL bytes and reconstructed it from its authored patch before full validation.
Updates independently reran the clean-filter content audit before release edits:
all 2,098 tracked files match their index hashes; all 2,094 source/text files are
NUL-free; there are no untracked source files. `git fsck --full --no-dangling`
returns zero with no integrity errors. Ordinary dangling objects are retained.
Raw private diagnostics, runtime state and audit logs remain ignored.

Reviewed the shared connection editor's empty-choice and stale-response guards,
catalog fallback, saved readiness preservation, Low-effort selection and all five
locale changes. The runtime prompts, parser and inference path are unchanged.
The opt-in evaluator correction recognizes retained-level follow semantics;
explicit Off/steady zero still mean zero. The correction does not relax runtime
acceptance. The [Terra evaluation](chatgpt-terra-evaluation-2026-10-28.md) retains
113 matched requests per model, 111/113 intent checks for each, median completion
3.284 s versus 4.045 s and the original arithmetic/drift/join failures. These
focused fixtures do not establish universal quality, long-context or physical
performance. Expanded opt-in harness lint has 52 existing findings; the normal
release lint gate is unchanged.

Updates inspected the five retained linear overview sheets and both vibration
sheets. Counted regions, coverage edits, anchored returns and rate contrasts
remain distinct; the wrong 70% arithmetic edit remains visible alongside correct
65% repeats. Follow is an allowed shared-motion drive range, explicit Off stays
zero, and retained zero-follow is labeled rather than plotted as false silence.
The source review retains detailed outliers and failed joins. These plots are
commanded estimates and do not certify physical feel.

Reviewed request admission, target serialization, lead refill, target commit,
dispatch run epochs and run-context cancellation. The bounded ten-second
dispatch context removes planner/request cancellation only for existing-run
refill and admitted uploads. Caller cancellation is checked before commitment;
wire commands still bind to the active run, so Stop/Pause/run replacement cancel
them. Startup cancellation and genuine uncertain-upload recovery stay enforced.
No second engine, transport path, sampling, geometry, speed, transition or protocol
change is introduced. See the [continuity review](autopilot-chat-continuity-review-2026-10-28.md)
for the observed Handy Cloud timing and regression that failed before the fix.

## Validation and footprint

The source thread completed full ordinary and Windows race Go suites, vet,
normal golangci-lint (zero issues), architecture/lifecycle tests, pure-Go build,
frontend typecheck/localization, 1,069 tests in 130 files and production build.
The real manager/shared-engine inert-transport overlap regression passed ten
repeats with one initial Play, no intervening Stop, the same stream and adoption
of the manual target. Boundary tests cover queued cancellation, refill,
commitment, Stop, Pause, deadline failure and unchanged startup cancellation.
Updates' focused architecture/config/motion/modes/transport/chat/HTTP suites pass.
PowerShell 7 installer tests pass. The fresh review generation probe succeeds;
a text-only app chat returns a nonempty reply in 330 ms with no repair/fallback
and motion off. Terra's first package run hit the 45-minute workflow limit during
artifact/lifecycle verification, without available job logs; only that canceled
job is retried on unchanged source. Final PR, merged
main and tag workflows must pass their full required gates before publication.

The [scorecard](goal-scorecard.md) records the Terra-only stripped core and
canonical UI measurements. The final continuity build measures 24,337,408 B (+512 B versus alpha.77)
with the same Go 1.26.10 pure-Go stripped review flags; shipping retains symbols
with `-w` and no `-s`. No dependencies, managed runtime pins, workers, embedded
assets beyond the canonical UI, or background loops are added. No RSS or
cold-start improvement is claimed.

## Release acceptance and artifact policy

Only alpha.78 is added to ReviewedUnsignedPublic; alpha.65 remains excluded and
alpha.79 remains rejected. Trusted signing is unavailable. The old Microsoft
case does not approve new hashes. The exact public directory Defender scan,
source and five exact-binary vulnerability scans, native x64/non-solid setup,
manifest/hash checks, three explicit assets, installer lifecycle and clean
current-main tag provenance remain mandatory. Updates verifies the actual
published downloads after the tag workflows succeed.

No physical device is connected or commanded. The next explicitly authorized
test must send chat across repeated Layered Autopilot boundaries on Handy Cloud,
confirm uninterrupted playback and Stop during upload, and retain the trace and
acknowledgement latencies. Existing vibrator acceptance gaps, Intiface Stop
limitations and installed-app full-range speed discrepancy remain open and are
disclosed in the public notes.

The current-source isolated app stays at `http://127.0.0.1:50308/#/chat`, using
`-simulate-motion`, remote listener disabled and available local Ollama Gemma
12B, without copied credentials. Updates revalidates real text generation and
a text-only app chat response without repair/fallback/motion, then leaves the
review tab open with simulator Idle, Autopilot Off and LLM motion Off. Earlier
review and user sessions remain running.
