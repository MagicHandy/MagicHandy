# ADR 0014: Public Windows Signing Gate

## Status

Accepted; amended 2026-10-26 after Microsoft completed the false-positive
review, for the alpha.9 installer correction, alpha.10's runtime readiness
corrections, alpha.11's update-discovery and clean-machine voice correction, and
alpha.13's restored setup distribution after alpha.12 was withdrawn, and the
reviewed alpha.14 through alpha.64 and alpha.66 through alpha.76
package-preserving releases.
This supersedes ADR 0013 where that ADR defines public unsigned setup
publication.

## Context

### Alpha.76 amendment

The maintainer requested publication of the prepared Layered review follow-up
in PR #353. The [alpha.76 review](../alpha76-release-review-2026-10-26.md)
records the reproduced fixes, source review, rendered comparisons, timing and
Stop regressions, footprint and remaining physical acceptance. The new joins
are not claimed as physically accepted, and the installed-app speed discrepancy
remains unresolved.

This amendment adds only `0.1.0-alpha.76` to the reviewed unsigned policy;
alpha.77 and later require another review, and alpha.65 remains excluded.
The five-executable payload, native x64 loader, non-solid ZIP compression and
three public artifacts are unchanged. Source and exact-binary vulnerability
scans, Defender scanning, manifests/checksums, installer lifecycle, green CI,
review and exact-current-main provenance remain mandatory. Prior Microsoft
review is not approval of these new binary hashes. Trusted signing remains
required for unrestricted public setup publication.

### Alpha.75 amendment

The maintainer requested the physically accepted Layered speed improvement
and release of the pending voice, UI, Stop/network and Autopilot fixes. The
[alpha.75 review](../alpha75-release-review-2026-10-26.md) records scope,
physical feedback, production integration, tests and remaining limitations.
This amendment adds only `0.1.0-alpha.75` to the reviewed unsigned policy;
alpha.76 and later require another review, and alpha.65 remains excluded.
The five-executable payload, loader, setup/ZIP/checksum contract, source and
exact-binary vulnerability scans, Defender scan, installer lifecycle and
exact-main provenance requirements are unchanged. Prior Microsoft review
is not approval of these new binary hashes. Trusted signing remains required
for unrestricted public setup publication.

### Alpha.74 amendment

The maintainer requested the current vibrator and local-model improvements,
Claude's consolidated model settings, an Intermediate planning latency toggle,
and a llama.cpp update followed by publication. The
[alpha.74 review](../alpha74-release-review-2026-10-25.md) records source review,
model and simulated-device evidence, UI checks and remaining physical limits.
The optional managed llama.cpp runtime advances to upstream b11533 with a pinned
commit and verified CPU/CUDA archive hashes. It is downloaded only on request;
no model or llama.cpp binary is added to the release payload.

This amendment adds only `0.1.0-alpha.74` to the reviewed unsigned policy;
alpha.75 and later require another review, and unpublished alpha.65 remains
excluded. The five payload executables, native x64 loader, non-solid ZIP
compression, three public assets and installation contract are unchanged.
Source and exact-binary vulnerability scans, exact-artifact Defender scanning,
manifest/checksum verification, installer lifecycle, green CI, review and
current-main provenance remain mandatory. Microsoft's prior case does not
approve these new hashes. Trusted Authenticode remains the long-term requirement.

### Alpha.73 amendment

The maintainer requested review of Claude's completed remote keepalive and Edge
work, a pull request, and an update. The
[alpha.73 review](../alpha73-release-review-2026-10-24.md) records the source
review, the in-flight Stop/handoff correction, honest expiry notices, tests,
budget measurements and browser limits. No physical-device claim is expanded.

This amendment adds only `0.1.0-alpha.73` to the reviewed unsigned policy;
alpha.74 and later require another review, and unpublished alpha.65 remains
excluded. The five payload executables, dependencies, native x64 loader,
non-solid ZIP compression, three public assets and installation contract are
unchanged. Source and exact-binary vulnerability scans, exact-artifact Defender
scanning, manifest/checksum verification, installer lifecycle, green CI, review
and current-main provenance remain mandatory. Microsoft's prior case does not
approve these new hashes. Trusted Authenticode remains the long-term requirement.

### Alpha.72 amendment

The maintainer requested implementation of the preferred real-device smoothness
candidate and an update after matched Original Handy Cloud auditions. The
[alpha.72 review](../alpha72-release-review-2026-10-24.md) records source,
physical comparisons, bounded sampling, model-response evidence and remaining
limits. The release keeps the existing precise path for imported media,
transitions, immediate transports, Handy 2 and multi-device sessions. Residual
turn hesitation is disclosed; physical preference is not a guarantee of all
future patterns or devices.

This amendment adds only `0.1.0-alpha.72` to the reviewed unsigned policy;
alpha.73 and later require another review, and unpublished alpha.65 remains
excluded. The five payload executables, dependencies, native x64 loader,
non-solid ZIP compression, three public assets and installation contract are
unchanged. Source and exact-binary vulnerability scans, exact-artifact Defender
scanning, manifest/checksum verification, installer lifecycle, green CI, review
and current-main provenance remain mandatory. Microsoft's prior case does not
approve these new hashes. Trusted Authenticode remains the long-term requirement.

### Alpha.71 amendment

The maintainer requested merging PRs #344 and #345 after green CI and publishing
an update. The completed Claude Opus 5.5 Max consultations, shared-engine motion
reviews, live ChatGPT/local-model evaluations and simulator checks are recorded
in [alpha71-release-review-2026-10-23.md](../alpha71-release-review-2026-10-23.md).
Physical comparison remains pending and is disclosed in the public notes;
compiled paths and immediate command acceptance do not establish device feel
or instantaneous response through a buffered transport.

This amendment adds only `0.1.0-alpha.71` to the reviewed unsigned policy;
alpha.72 and later require another review, and unpublished alpha.65 remains
excluded. The five payload executables, dependencies, native x64 loader,
non-solid ZIP compression, three public assets and installation contract are
unchanged. Source and exact-binary vulnerability scans, exact-artifact Defender
scanning, manifest/checksum verification, installer lifecycle, green CI, review
and current-main provenance remain mandatory. Microsoft's prior case does not
approve these new hashes. Trusted Authenticode remains the long-term requirement.

### Alpha.70 amendment

The maintainer requested an update after PR #343's interactive response-repair,
stopped-motion intent and Layered join review, including adaptations evaluated
with the local model. The completed Claude Opus 5.5 review, addressed findings,
live ChatGPT and local-model evaluations, and shared-engine visual evidence are
recorded in
[hosted-response-repair-review-2026-10-23.md](../hosted-response-repair-review-2026-10-23.md).
Remaining local semantic misses and physical-device acceptance limits remain
disclosed. Model and compiled-motion results do not establish physical feel.

This amendment adds only `0.1.0-alpha.70` to the reviewed unsigned policy;
alpha.71 and later require another review, and unpublished alpha.65 remains
excluded. The five payload executables, dependencies, native x64 loader,
non-solid ZIP compression, three public assets and installation contract are
unchanged. Source and exact-binary vulnerability scans, exact-artifact Defender
scanning, manifest/checksum verification, installer lifecycle, green CI, review
and current-main provenance remain mandatory. Microsoft's prior case does not
approve these new hashes. Trusted Authenticode remains the long-term requirement.

### Alpha.69 amendment

The maintainer requested publication after PR #341's counted Layered phrase,
reach-preservation and transition review. The completed Claude Opus 5.5 Max
review, addressed findings, live ChatGPT evaluations and shared-engine visual
acceptance are recorded in
[alpha69-release-review-2026-10-23.md](../alpha69-release-review-2026-10-23.md).
Physical Original Handy / Cloud acceptance remains pending and is explicitly
disclosed; commanded endpoint coverage is not physical carriage telemetry.

This amendment adds only `0.1.0-alpha.69` to the reviewed unsigned policy;
alpha.70 and later require another review, and unpublished alpha.65 remains
excluded. The payload executables, dependencies, native loader, compression,
artifact set and installation contract are unchanged. Source and exact-binary
vulnerability scans, exact-artifact Defender scanning, manifest/checksum
verification, installer lifecycle, green CI and current-main provenance remain
mandatory. Microsoft's prior case does not approve these new hashes. Trusted
Authenticode remains the long-term requirement.

### Alpha.68 amendment

The maintainer requested an update after PR #339's WAN session, hosted motion
and Layered smoothness review. The source, completed Claude consultation,
live-model and simulated-motion acceptance evidence is recorded in
[alpha68-release-review-2026-10-22.md](../alpha68-release-review-2026-10-22.md).
Real WAN repetition and physical Handy Cloud comparison remain pending and
are not claimed as release validation.

This amendment adds only `0.1.0-alpha.68` to the reviewed unsigned policy;
alpha.69 and later require another review, and unpublished alpha.65 remains
excluded. No dependency, payload executable, native loader, compression format,
artifact set or installation contract changes. Source and exact-binary
vulnerability scans, exact-artifact Defender scanning, manifest/checksum
verification, installer lifecycle, green CI and current-main provenance remain
mandatory. Microsoft's prior case does not approve these new hashes. Trusted
Authenticode remains the long-term requirement; the unsigned warning and
existing hardware acceptance limits remain disclosed.

### Alpha.67 amendment

The maintainer requested merging PR #337 after green CI and releasing an
update. Alpha.67 fixes browser password-manager sign-in and linked page
navigation, and documents existing external DNS-01/alternate-port HTTPS setup.
The source and browser acceptance review is recorded in
[alpha67-release-review-2026-10-21.md](../alpha67-release-review-2026-10-21.md).

This amendment adds only `0.1.0-alpha.67` to the reviewed unsigned policy;
alpha.68 and later require another review, and unpublished alpha.65 remains
excluded. No dependency, payload executable, native loader, compression format,
artifact set or installation contract changes. Source and exact-binary
vulnerability scans, exact-artifact Defender scanning, manifest/checksum
verification, installer lifecycle, green CI and current-main provenance remain
mandatory. Microsoft's prior case does not approve these new hashes. Trusted
Authenticode remains the long-term requirement; the unsigned warning and
existing hardware acceptance limits remain disclosed.

### Alpha.66 amendment

Alpha.65's final Windows symbol scan blocked publication for GO-2026-6809 in
Go 1.26.9. No release or public artifacts were created. Its source tag remains
immutable. Alpha.66 retains PR #329's cloud authentication changes and PR #332's
startup recovery, and raises the build and source-installer minimum to Go
1.26.10, which distrusts the compromised intermediate certificates.
Source installations using Go 1.27 require the corresponding fixed patch,
1.27.3 or newer; the installer rejects 1.27.0 through 1.27.2.

This amendment admits only `0.1.0-alpha.66`, removes the unpublished alpha.65
from the active allowlist, and leaves alpha.67 and later unreviewed. The payload,
packaging and signing shape is unchanged. The security review and budget result
are recorded in [alpha66-release-review-2026-10-20.md](../alpha66-release-review-2026-10-20.md).
All source and exact-binary vulnerability checks, Defender scanning, checksum
and manifest verification, installer lifecycle, green CI and current-main
provenance gates remain mandatory. The prior Microsoft case does not approve
these new hashes. Trusted Authenticode remains the long-term requirement.

### Alpha.65 amendment

On 2026-10-20 the maintainer requested an update including PR #329 alongside
startup recovery from PR #332. This amendment adds only `0.1.0-alpha.65` to the
reviewed unsigned Windows policy; alpha.66 and later still require their own
review. The release combines bounded local port fallback and instance discovery
with responsive cloud authentication and current-user DPAPI credential storage.
The retained startup records are included in the existing diagnostic report.
Explicit fixed ports and hosted deployments remain strict; no HNS reset is made.

The independent release review of the authentication delta, migration and
recovery boundaries, startup integration, and validation is recorded in
[alpha65-release-review-2026-10-20.md](../alpha65-release-review-2026-10-20.md).
No external runtime dependency or payload executable is added. DPAPI is supplied
by Windows and called through the existing pure-Go Windows bindings. The source
and exact-binary vulnerability scans, exact-artifact Defender scan, checksum and
manifest verification, installer lifecycle, green CI, review and current-main
provenance gates remain mandatory. Microsoft's earlier case does not approve
these new hashes; trusted Authenticode remains the long-term requirement.
The credential downgrade/move limitations and existing hardware acceptance
limits are disclosed in the release notes.

### Alpha.64 amendment

On 2026-10-18 the maintainer requested releasing the completed hosted Layered
and external-context fixes from PR #328. This amendment adds only
`0.1.0-alpha.64` to the reviewed unsigned Windows policy; alpha.65 and later
still require their own review. The release fixes ambiguous structured motion
operations, distinguishes temporary from lasting hosted Autopilot directions,
and exposes separate external request and history budgets. Local prompts and
inference budgets remain unchanged. The shared geometry validator fits retained
stroke widths to explicitly narrowed bands; the engine, transports and Stop
path retain their existing boundaries.

The release record is
[alpha64-release-review-2026-10-18.md](../alpha64-release-review-2026-10-18.md).
No dependency, payload executable, native loader, compression format, artifact
set or installation contract is added or changed. Source and exact-binary
vulnerability scans, the exact-artifact Defender scan, manifest/checksum
verification, installer lifecycle, review and current-main provenance remain
mandatory. Microsoft's earlier case does not approve these new hashes; trusted
Authenticode remains the long-term requirement. Existing hardware acceptance
limits and the Intiface Stop issue remain disclosed.

### Alpha.63 amendment

On 2026-10-16 the maintainer requested merging PR #326 and publishing an update.
This amendment adds only `0.1.0-alpha.63` to the reviewed unsigned Windows
policy; alpha.64 and later still require their own review. It packages the
external Autopilot timing and prepared-decision changes described in ADR 0050.
Auto timing is opt-in; prepared decisions retain the existing shared-engine,
controller, pause and Stop boundaries. Local planning remains unchanged.
The release record is [alpha63-release-review-2026-10-16.md](../alpha63-release-review-2026-10-16.md).

No payload executable, dependency, native loader, compression format, artifact
set or installation contract is added or changed by this release preparation.
The five payload executables, source and exact-binary vulnerability scans,
Defender scan of the exact public files, checksums, manifests, installer
lifecycle and current-main provenance gates remain mandatory. The prior
Microsoft case does not approve the new hashes, and trusted Authenticode
remains the long-term requirement. ADR 0049's Intiface Stop limitation and
other physical acceptance limits remain disclosed.

### Alpha.62 amendment

The maintainer requested publication after the device and motion review. This
amendment adds only `0.1.0-alpha.62` to the reviewed-unsigned Windows policy;
later versions still require their own review. It carries COYOTE 3.0 and native
Handy Bluetooth, linked sessions and vibrator output, Claude's device UI,
reviewed local/hosted motion corrections, observed Intiface smoothing and the
network/crypto dependency update. ADRs 0044–0049 and their review records state
the scope and known hardware limits. The upstream Intiface Stop defect remains
open and is disclosed in the app and release notes (ADR 0049).

The core remains pure Go. The native Bluetooth worker joins the four existing
payload executables; all five use the same exact-binary scans and manifest
checks. The native x64 loader, non-solid ZIP, three public assets and
data-preserving over-install contract remain. Main-tip provenance, review,
quality gates, adapter tests, source and binary vulnerability scans, Defender,
checksums and exact-installer lifecycle tests remain mandatory. Microsoft's
original case does not pre-clear the new hashes; trusted Authenticode is still
the long-term policy.

### Alpha.61 amendment

On 2026-10-11 the user requested an update release of the completed review
branch. This amendment adds only `0.1.0-alpha.61` to the reviewed-unsigned
Windows alpha policy; it grants no standing permission to later versions.

The release includes optional asynchronous motion review, inspectable chat
failures and prompt assignment, model-chosen quiet check-ins, longer external
conversation windows, Creative v2 pace and sampling corrections, and selectable
diagnostic reports. ADRs 0040–0043 and the dated review records document their
evidence and limits. The motion changes preserve the shared engine and existing
physical limits; hardware feel has not been physically verified. No dependency
or CGo is added. The goal scorecard records binary, UI and runtime costs.
Hosted connections remain experimental; follow-ups #306 and #307 remain open.

The native x64 loader, non-solid ZIP compression, three public assets, four
pure-Go payload executables and data-preserving over-install contract remain.
Source must reach main through reviewed pull requests and green CI. Main-tip
provenance, full quality gates, adapter tests, source and exact-binary
vulnerability scans, exact-artifact Defender scanning, manifests, checksums
and installer lifecycle verification remain mandatory. The original Microsoft
case does not pre-clear these new hashes; trusted Authenticode remains the
long-term policy.

### Alpha.60 amendment

On 2026-10-10 the user requested an update release after four fixes reached
`main` through reviewed pull requests (#312, #313, #314 and #315). This
amendment adds only `0.1.0-alpha.60` to the reviewed-unsigned Windows alpha
policy; it grants no standing permission to later versions.

The release stops a running Intiface scan once an actuator is selected and
shows that device as connected, renames the device connection choice to
Protocol, gives the Local LLM and Managed models settings cards their header
rows, and allows a newly installed app 30 seconds instead of 10 to start
before the launcher treats it as failed. No dependency, model call, motion path
or resident state is added, and no CGo is introduced. Follow-ups #306 and #307
remain open.

The native x64 loader, non-solid ZIP compression, three public assets, four
pure-Go payload executables and data-preserving over-install contract remain.
Source must reach main through reviewed pull requests and green CI. Main-tip
provenance, full quality gates, adapter tests, source and exact-binary
vulnerability scans, exact-artifact Defender scanning, manifests, checksums
and installer lifecycle verification remain mandatory. The original Microsoft
case does not pre-clear these new hashes; trusted Authenticode remains the
long-term policy.

### Alpha.59 amendment

On 2026-10-10 the user requested an update release after the model settings
rebuild and the visualizer smoothness fixes reached `main` through reviewed pull
requests (#309 and #310). This amendment adds only `0.1.0-alpha.59` to the
reviewed-unsigned Windows alpha policy; it grants no standing permission to
later versions.

The release rebuilds the hosted-model settings around Model roles and an
External providers list that can remove providers; the backend deletes a
removed connection's stored API key only after the settings save succeeds.
Setup labels the hosted options Experimental, and
[ADR 0039](0039-hosted-model-connections.md) remains "Proposed; pending final
independent review". The Handy 2 dot display and the Intiface scanning bar now
move on CSS transform transitions like the original Handy view. No dependency,
model call, motion path or resident state is added, and no CGo is introduced.
Follow-ups #306 and #307 remain open.

The native x64 loader, non-solid ZIP compression, three public assets, four
pure-Go payload executables and data-preserving over-install contract remain.
Source must reach main through reviewed pull requests and green CI. Main-tip
provenance, full quality gates, adapter tests, source and exact-binary
vulnerability scans, exact-artifact Defender scanning, manifests, checksums
and installer lifecycle verification remain mandatory. The original Microsoft
case does not pre-clear these new hashes; trusted Authenticode remains the
long-term policy.

### Alpha.58 amendment

On 2026-10-10 the user requested an update release after the hosted-model,
setup and visual-system work reached `main` through reviewed pull requests
(#299, #300, #301, #303, #304 and #305). This amendment adds only
`0.1.0-alpha.58` to the reviewed-unsigned Windows alpha policy; it grants no
standing permission to later versions.

The release adds hosted model connections (ChatGPT sign-in, OpenAI API,
OpenRouter and compatible endpoints) under
[ADR 0039](0039-hosted-model-connections.md), which remains "Proposed; pending
final independent review"; the release notes present hosted connections as a
preview. It also ships the redesigned setup, the Graphite visual system,
device-specific visualizers, the Creative v2 schema and reach-guide changes,
and the Go 1.26.9 toolchain. No CGo dependency is added; the
[goal scorecard](../goal-scorecard.md) records each change's binary and
bundle sizes. Local Gemma 12B passed the strict Creative v2 path 28/28 on the
managed llama.cpp runtime before merge, as recorded in the
[reach-guide review](../creative-v2-reach-guide-review-2026-10-09.md).
Follow-ups #306 (a credential lock held across network calls) and #307
(encrypting stored hosted credentials at rest) remain open.

The native x64 loader, non-solid ZIP compression, three public assets, four
pure-Go payload executables and data-preserving over-install contract remain.
Source must reach main through reviewed pull requests and green CI. Main-tip
provenance, full quality gates, adapter tests, source and exact-binary
vulnerability scans, exact-artifact Defender scanning, manifests, checksums
and installer lifecycle verification remain mandatory. The original Microsoft
case does not pre-clear these new hashes; trusted Authenticode remains the
long-term policy.

### Alpha.57 amendment

On 2026-10-07 the user requested an update release after the Handy Cloud
connection-key review and correction in PR #297. This amendment adds only
`0.1.0-alpha.57` to the reviewed-unsigned Windows alpha policy; it grants no
standing permission to later versions.

Cloud authentication now follows the current API v3 key schema (5–64 ASCII
letters or digits), accepting five-character keys that the old eight-character
minimum incorrectly rejected. Case and pasted outer-whitespace handling are
preserved. The [reference review](../handy-cloud-api-review-2026-10-07.md)
records the older help article's narrower maximum, the SSE documentation
inconsistency, mock regression coverage and the absence of live verification
of the reporting user's key or device. No dependency, motion path or UI asset
is added; the [goal scorecard](../goal-scorecard.md) records equal baseline and
candidate binary sizes.

The native x64 loader, non-solid ZIP compression, three public assets, four
pure-Go payload executables and data-preserving over-install contract remain.
Source must reach main through reviewed pull requests and green CI. Main-tip
provenance, full quality gates, adapter tests, source and exact-binary
vulnerability scans, exact-artifact Defender scanning, manifests, checksums
and installer lifecycle verification remain mandatory. The original Microsoft
case does not pre-clear these new hashes; trusted Authenticode remains the
long-term policy.

### Alpha.56 amendment

On 2026-10-05 the user requested an update release after the guided voice
setup in PR #295 and test-fixture speedup in PR #290. Both passed all thirteen
checks before merging. This amendment adds only `0.1.0-alpha.56` to the
reviewed-unsigned Windows alpha policy; it grants no standing permission to
later versions.

Easy Setup offers Qwen3-TTS and Chatterbox, prefers Qwen unless the combined
LLM/voice memory allowance is insufficient, and guides a local audio sample
and exact transcript before enabling Qwen speech. Deferral keeps spoken
replies off. The [setup review](../easy-setup-voice-review-2026-10-05.md) records
validation, the estimated nature of the memory allowances, and the remaining
voice-listening and simultaneous inference measurements. No dependency was
added; the [goal scorecard](../goal-scorecard.md) records the binary and UI
budgets. The race optimization changes only test fixtures and supporting
documentation; each fixture retains its own datastore and normal settings
initialization.

The native x64 loader, non-solid ZIP compression, three public assets, four
pure-Go payload executables, and data-preserving over-install contract remain.
Source must reach main through reviewed pull requests and green CI. Main-tip
provenance, full quality gates, adapter tests, source and exact-binary
vulnerability scans, exact-artifact Defender scanning, manifests, checksums,
and installer lifecycle verification remain mandatory. The original Microsoft
case does not pre-clear these new hashes; trusted Authenticode remains the
long-term policy.

### Alpha.55 amendment

On 2026-10-03 the user requested an update release after the Creative v2
response-validation investigation and hardening reviewed in PR #293. This
amendment adds only `0.1.0-alpha.55` to the reviewed-unsigned Windows alpha
policy; it grants no standing permission to later versions.

Creative v2 now requires a non-empty reply in its output schema and identifies
the specific field that fails validation. Invalid proposals retain the same
one-generation, no-fallback rejection before shared-engine dispatch. The
[investigation](../creative-v2-rejection-review-2026-10-03.md) records the
regressions, successful local model checks and the reporting user's still
unconfirmed cause. No dependency was added; the
[goal scorecard](../goal-scorecard.md) records the binary change.

The native x64 loader, non-solid ZIP compression, three public assets, four
pure-Go payload executables, and data-preserving over-install contract remain.
Source must reach main through reviewed pull requests and green CI. Main-tip
provenance, full quality gates, adapter tests, source and exact-binary
vulnerability scans, exact-artifact Defender scanning, manifests, checksums,
and installer lifecycle verification remain mandatory. The original Microsoft
case does not pre-clear these new hashes; trusted Authenticode remains the
long-term policy.

### Alpha.54 amendment

On 2026-10-01 the user requested an update release after the Freestyle stream
implementation and fixes reviewed in PR #291. All thirteen pull-request checks
passed before that implementation reached main. This amendment adds only
`0.1.0-alpha.54` to the reviewed-unsigned Windows alpha policy; it grants no
standing permission to later versions.

Freestyle now supplies a continuous semantic stroke stream to the shared
engine, with independent preferences and explicit session shapes. The
[stream decision](0038-freestyle-stroke-stream.md),
[implementation review](../freestyle-stream-review-2026-10-01.md), and
[goal scorecard](../goal-scorecard.md) record the architecture, regression
fixes, visual evidence and measured budgets. No dependency was added. Physical
feel and transport latency acceptance remain open.

The native x64 loader, non-solid ZIP compression, three public assets, four
pure-Go payload executables, and data-preserving over-install contract remain.
Source must reach main through reviewed pull requests and green CI. Main-tip
provenance, full quality gates, adapter tests, source and exact-binary
vulnerability scans, exact-artifact Defender scanning, manifests, checksums,
and installer lifecycle verification remain mandatory. The original Microsoft
case does not pre-clear these new hashes; trusted Authenticode remains the
long-term policy.

### Alpha.53 amendment

On 2026-09-30 the user requested an update release with the Remote link fix
reviewed in PR #288. This amendment adds only `0.1.0-alpha.53` to the
reviewed-unsigned Windows alpha policy; it grants no standing permission to
later versions.

The update lets the main app open the Remote again, under the browser boundary
exception recorded in [ADR 0033](0033-remote-listener-and-account-scope.md), and
serializes the Gemma 4 chat template install that could fail on Windows. No
dependency was added; the [goal scorecard](../goal-scorecard.md) records the
binary change.

The native x64 loader, non-solid ZIP compression, three public assets, four
pure-Go payload executables, and data-preserving over-install contract remain.
Source must reach main through reviewed pull requests and green CI. Main-tip
provenance, full quality gates, adapter tests, source and exact-binary
vulnerability scans, exact-artifact Defender scanning, manifests, checksums,
and installer lifecycle verification remain mandatory. The original Microsoft
case does not pre-clear these new hashes; trusted Authenticode remains the
long-term policy.

### Alpha.52 amendment

On 2026-09-30 the user requested an update release after the setup, model and
runtime work reviewed in PR #286. This amendment adds only `0.1.0-alpha.52` to
the reviewed-unsigned Windows alpha policy; it grants no standing permission to
later versions.

The update adds Easy setup and curated model downloads to guided setup, a
user-started model test with known chat template fixes, a reply length setting,
and release-pinned llama.cpp runtime updates on `b11149`. ADRs
[0034](0034-curated-model-downloads.md), [0035](0035-model-test-and-template-fixes.md),
[0036](0036-managed-runtime-updates.md) and [0037](0037-easy-setup.md), the
[model suitability review](../model-suitability.md) and the
[goal scorecard](../goal-scorecard.md) record the findings, tests and measured
budgets. No dependency was added.

The native x64 loader, non-solid ZIP compression, three public assets, four
pure-Go payload executables, and data-preserving over-install contract remain.
Source must reach main through reviewed pull requests and green CI. Main-tip
provenance, full quality gates, adapter tests, source and exact-binary
vulnerability scans, exact-artifact Defender scanning, manifests, checksums,
and installer lifecycle verification remain mandatory. The original Microsoft
case does not pre-clear these new hashes; trusted Authenticode remains the
long-term policy.

### Alpha.51 amendment

On 2026-09-29 the user requested an update release after the video, remote,
prompt and launch-console work reviewed in PR #284. This amendment adds only
`0.1.0-alpha.51` to the reviewed-unsigned Windows alpha policy; it grants no
standing permission to later versions.

The update adds video curation, a separate remote listener and remote-only
accounts, and corrects capability-aware chat prompts and terminal activity
retention. The [completion review](../claude-progress-review-2026-09-29.md),
[remote review](../remote-access-review-2026-09-29.md),
[prompt review](../prompt-regression-review-2026-09-29.md), and
[console review](../console-activity-review-2026-09-29.md) record the findings,
tests, measured budgets and remaining hardware/WAN validation limits. No
dependency was added.

The native x64 loader, non-solid ZIP compression, three public assets, four
pure-Go payload executables, and data-preserving over-install contract remain.
Source must reach main through reviewed pull requests and green CI. Main-tip
provenance, full quality gates, adapter tests, source and exact-binary
vulnerability scans, exact-artifact Defender scanning, manifests, checksums,
and installer lifecycle verification remain mandatory. The original Microsoft
case does not pre-clear these new hashes; trusted Authenticode remains the
long-term policy.

### Alpha.50 amendment

On 2026-09-26 the user requested an update release with the depth
corrections for Creative v2 and Layered chat, the new LLM Lab modes, a
page-return fix and Creative v2 as the default for new installs. PRs #279,
#280, #281 and #282 passed all thirteen checks before merging. This
amendment adds only `0.1.0-alpha.50` to the reviewed-unsigned Windows alpha
policy; it grants no standing permission to later versions.

The update gives every motion prompt one depth frame, described in the
[depth review](../motion-depth-review-2026-09-25.md). The optional LLM Lab
gains a Lab-only stroke score with three model vocabularies, a mode
comparison and a streamlined page, described in the
[stroke vocabulary review](../lab-stroke-modes-review-2026-09-26.md). A page
returning from the background keeps its view instead of reloading, and new
installs start on Creative v2 ([ADR 0015](0015-selectable-llm-motion-modes.md)).
No dependency was added.

The native x64 loader, non-solid ZIP compression, three public assets, four
pure-Go payload executables, and data-preserving over-install contract remain.
Main-tip provenance, full quality gates, adapter tests, source and exact-binary
vulnerability scans, exact-artifact Defender scanning, manifests, checksums,
and installer lifecycle verification remain mandatory. The original Microsoft
case does not pre-clear these new hashes; trusted Authenticode remains the
long-term policy.

### Alpha.49 amendment

On 2026-09-25 the user requested an update release after the branded launch
console. PR #277 passed all thirteen checks before merging. This amendment adds
only `0.1.0-alpha.49` to the reviewed-unsigned Windows alpha policy; it grants
no standing permission to later versions.

The update replaces the JSON log stream in the console window that opens with
the app by an interactive launch console, described in the
[UI design guide](../ui-design.md#launch-console). Its Stop key uses the
existing emergency-stop path. No dependency was added.

The native x64 loader, non-solid ZIP compression, three public assets, four
pure-Go payload executables, and data-preserving over-install contract remain.
Main-tip provenance, full quality gates, adapter tests, source and exact-binary
vulnerability scans, exact-artifact Defender scanning, manifests, checksums,
and installer lifecycle verification remain mandatory. The original Microsoft
case does not pre-clear these new hashes; trusted Authenticode remains the
long-term policy.

### Alpha.48 amendment

On 2026-09-25 the user requested an update release after the motion
naturalness review and its pace, variety and recall follow-up. PR #275 passed
all thirteen checks before merging. This amendment adds only
`0.1.0-alpha.48` to the reviewed-unsigned Windows alpha policy; it grants no
standing permission to later versions.

The update changes Autopilot continuity, pace, variety and recall, and phrases
Creative v2 strokes inside the shared plan, as recorded in
[ADR 0031](0031-model-judged-autopilot-continuity.md). The
[naturalness review](../motion-naturalness-review-2026-09-24.md) records the
simulated evidence and the remaining physical-device limits. No dependency was
added.

The native x64 loader, non-solid ZIP compression, three public assets, four
pure-Go payload executables, and data-preserving over-install contract remain.
Main-tip provenance, full quality gates, adapter tests, source and exact-binary
vulnerability scans, exact-artifact Defender scanning, manifests, checksums,
and installer lifecycle verification remain mandatory. The original Microsoft
case does not pre-clear these new hashes; trusted Authenticode remains the
long-term policy.

### Alpha.47 amendment

On 2026-09-20 the user requested an update release after the guided HTTPS,
transport, and mobile Chat work. PRs #271, #272, and #273 passed all thirteen
checks before merging in dependency order. This amendment adds only
`0.1.0-alpha.47` to the reviewed-unsigned Windows alpha policy; it grants no
standing permission to later versions.

The update adds the [guided certificate flow](0030-guided-https-certificates.md),
corrects Handy Bluetooth wire units and transport session lifecycle, and improves
mobile Chat spacing and connection-button alignment. The
[transport review](../bluetooth-intiface-review-2026-09-20.md) records the
certificate-listener cleanup race found and fixed in CI, simulated motion
evidence, and the remaining real-device limits. The
[Chat review](../chat-mobile-review-2026-09-20.md) records viewport and real-model
acceptance; physical mobile-keyboard testing remains open.

The native x64 loader, non-solid ZIP compression, three public assets, four
pure-Go payload executables, and data-preserving over-install contract remain.
The guided certificate dependency is pure Go and recorded in ADR 0030, with
its size cost in the scorecard. Main-tip provenance, full quality gates,
adapter tests, source and exact-binary vulnerability scans, exact-artifact
Defender scanning, manifests, checksums, and installer lifecycle verification
remain mandatory. The original Microsoft case does not pre-clear these new
hashes; trusted Authenticode remains the long-term policy.

### Alpha.46 amendment

On 2026-09-19 the user requested completion of LAN/WAN preparedness followed
by an update release, using security and reliability of the implemented modes
as the release criterion. This amendment adds only `0.1.0-alpha.46` to the
reviewed-unsigned Windows alpha policy; it grants no standing permission to
later versions.

The update includes session-bound LAN/WAN control, the consolidated settings
and notice preferences, motion/transport corrections, full-page dialog
backdrops and the confirmed Chatterbox/PowerShell installer fixes. The final
[LAN/WAN review](../lan-wan-release-readiness-2026-09-19.md) closes queued
administrator-write revocation gaps and updates Go to 1.26.8. Source and all
four exact payload executables receive pinned vulnerability scans. Release
builds retain the compact symbol table (`-w`, without `-s`) so the scanner can
identify linked code; the measured core remains below its size budget.

The native x64 loader, non-solid ZIP compression, three public assets, pure-Go
payload and data-preserving over-install contract remain. Main-tip provenance,
full quality gates, adapter tests, exact-artifact Defender scanning, manifests,
checksums and installer lifecycle verification are mandatory. The recorded
Microsoft case covers the original submitted hash, not the new artifacts;
trusted Authenticode remains the long-term policy.

### Alpha.45 amendment

On 2026-09-12 the user requested publication after the TTS reliability and
downloadable installer failure-report work. This amendment adds only
`0.1.0-alpha.45` to the existing reviewed-unsigned Windows alpha policy. It
grants no standing permission to later versions.

PR #264 passed all thirteen checks before merging. Alpha.45 removes artificial
streamed-speech gaps, adds bounded adaptive buffering and prompt cancellation,
preserves useful installer failure details, and offers redacted, retained
failure reports through a one-click download. The original reported Chatterbox
failure remains unconfirmed; CPU dependency installation and native imports
passed, while actual model listening and RTX 3070 throughput remain open. See
the [TTS review](../tts-reliability-review-2026-09-12.md) and
[failure-report review](../installer-failure-report-review-2026-09-12.md).

The native x64 loader, non-solid ZIP compression, three public assets, four
pure-Go payload executables and data-preserving over-install contract remain.
No dependencies or model/package pins change. Main-tip provenance, full quality
gates, Python adapter tests, Defender scanning of the exact artifacts,
manifests, outer checksums and exact installer lifecycle verification remain
mandatory. The Microsoft case does not pre-clear new hashes, and trusted
Authenticode remains the long-term policy.

### Alpha.44 amendment

On 2026-09-08 the user requested an update release after reviewing PR #261's
AI runtime improvements and its explicit acceptance limits. This release
amendment adds only `0.1.0-alpha.44` to the existing reviewed-unsigned Windows
alpha policy. It grants no standing permission to later versions.

PR #261 passed all thirteen checks before merging. Alpha.44 ships bounded LLM
prompts, coordinated model replacement, progressive speech and cancellation,
Parakeet/ElevenLabs corrections, and staged optional-TTS updates. Actual speech
model listening, shared-GPU tuning and full multi-GiB module-update acceptance
remain open as recorded in the [AI implementation review](../ai-runtime-improvements-2026-09-08.md).
The release notes make these alpha limitations and the separate managed-module
update requirement visible to users.

The native x64 setup loader, non-solid ZIP compression, three public assets,
four pure-Go payload executables and data-preserving over-install contract
remain. The shared Python streaming helper is now a required payload file.
The tag workflow adds both Python 3.10 and 3.11 adapter tests as publication
dependencies. Main-tip provenance, full quality gates, Defender scanning of
the exact artifacts, manifests, outer checksums and exact installer lifecycle
verification remain mandatory. The Microsoft case does not pre-clear new
hashes, and trusted Authenticode remains the long-term policy.

### Alpha.43 amendment

On 2026-09-06 the user explicitly approved publishing the startup-arrival fix
as alpha.43 under the existing reviewed-unsigned policy and updating/restarting
the installed instance while preserving its settings, chats and models. Only
`0.1.0-alpha.43` is added to the allowlist. No later version is authorized.

PR #259 corrects the target used for startup arrival verification: a fractional
seek target must be compared after the transport's output quantization, with
the existing 1% tolerance and Stop behavior preserved. The installed alpha.42
failure was reproduced from captured Cloud REST telemetry before the fix;
see the [investigation and regression evidence](../startup-arrival-review-2026-09-06.md).

Packaging, dependencies and installer behavior are unchanged. PR/main/tagged
quality gates, exact-main-tip provenance, Defender scanning of the exact
artifacts, manifests/checksums and installer acceptance remain mandatory.
The original Microsoft case does not pre-clear these new hashes. Updating the
installed app does not authorize a hardware playback test; physical confirmation
of the corrected startup remains a separate user evaluation.

### Alpha.42 amendment

On 2026-09-06 the user explicitly approved publishing alpha.42 under the
existing reviewed-unsigned Windows policy, with only this version added to
the allowlist. PRs #253–#257 merged lifecycle/state, datastore/observation,
video-seeking and funscript-filter improvements. The release preparation also
fixes the unsafe-startup cancellation race found by the post-merge Go check;
see the [failure investigation](../alpha42-release-validation-2026-09-06.md).

The package layout, native x64 loader, compression, dependencies and installer
lifecycle are preserved. No later version is authorized. The release PR and
merged main must pass the existing checks before an immutable tag is pushed.
Defender must scan the exact release directory; manifests, checksums, installer
lifecycle and main-tip provenance must verify before publication. Microsoft's
completed case concerns its original alpha.6 artifact, not advance clearance
for these new hashes. Physical video/filter acceptance remains open as recorded
in the [filter review](../funscript-filter-review-2026-09-06.md).

### Alpha.41 amendment

The user requested publication on 2026-09-06 after the continuous chat request
validation review. Alpha.41 replaces language-based edit guards in Creative v2
and Layered with model-declared actions and backend-state validation. It
preserves the package layout, installer behavior, dependencies, and release
process. Only this additional version is added to the reviewed unsigned
allowlist; the amendment grants no permission to later versions. Source must
reach main through the reviewed PR and green CI, followed by the existing
Defender scan, exact-package verification, installer lifecycle, and main-tip
tag provenance gates. The completed Microsoft case concerns its original
artifact, not advance clearance for these new hashes. See the
[evaluation evidence](../continuous-request-review-2026-09-06.md).

### Alpha.40 amendment

The alpha.40 update preserves the package layout and release process
while adding the merged motion/Labs work and the llama.cpp conversation and
Autopilot corrections, including the continuous-mode handoff crash fix. Its
explicit version is included in the reviewed unsigned allowlist. Publication
was requested on 2026-09-05 after fixing the reported crash; the source still
reaches main through the reviewed PR and green CI. This grants no standing
permission to later versions.
Defender scanning, exact-package verification, installer lifecycle, main-tip
tag provenance and explicit publication approval remain required. See the
[evaluation evidence](../llama-autopilot-review-2026-09-05.md).

The unsigned `v0.1.0-alpha.6` Inno Setup executable was classified at launch by
Microsoft Defender as `Behavior:Win32/DefenseEvasion.A!ml`. Windows then blocked
launch with ShellExecuteEx error 225. The downloaded file matched the release
SHA-256 (`42b47976da6ed0431e8dea8bce63f15fcb48fbf9ae962e44a82a691d33cf85bd`),
the tagged source and GitHub Actions build passed all release gates, and a
separate static Defender scan did not report a threat. Those facts establish
provenance; they do not establish publisher identity or make a severe runtime
detection acceptable to ask users to bypass.

A point-in-time VirusTotal report for that exact hash showed 4 of 71 engines
flagging the file: Microsoft, DeepInstinct, SecureAge, and Skyhigh. Its sandbox
summary showed no direct malware detection, while its structural and behavior
metadata identified an unsigned Inno Setup executable with an overlay and the
generic `obfuscated` and `self-delete` tags. That mixed result neither proves
malicious behavior nor independently establishes a false positive.

The GitHub release was withdrawn without moving or replacing its immutable tag.
The exact executable was submitted to Microsoft Security Intelligence as
submission `15c1e36d-fb35-4c5d-85de-83707169818a` for false-positive analysis.
On 2026-08-03 Microsoft completed that case with final determination
`Not malware`, reported no current cloud or client detection, and stated that
the detection had been removed. That resolves the Defender classification for
the exact alpha.6 artifact. It does not give future hashes publisher identity
or pre-clear a differently packaged installer.

A controlled same-payload packaging comparison also found avoidable structural
traits in the withdrawn package: its loader was 32-bit even though the payload
and artifact name were amd64, and the payload was one solid
`lzma2/ultra64` stream. Inno Setup documents its
[64-bit architecture modes](https://jrsoftware.org/ishelp/topic_64bit.htm) and
that the single-file [Setup loader](https://jrsoftware.org/ishelp/topic_setup_usesetupldr.htm)
extracts and runs Setup from a temporary directory. Those traits are consistent
with the report's PE32, opaque overlay, `obfuscated`, and `self-delete`
observations, but correlation is not proof that they alone caused the cloud
classification.

VirusTotal report:
<https://www.virustotal.com/gui/file/42b47976da6ed0431e8dea8bce63f15fcb48fbf9ae962e44a82a691d33cf85bd/detection>

## Decision

1. **Pull-request setup executables are CI-only.** Pull-request workflows build
   an unsigned Inno Setup executable to test custom/default install, upgrade,
   shortcut, uninstall, purge, and clean-reinstall behavior. It remains a
   short-lived workflow artifact labeled `unsigned-ci`; pull requests have no
   release permission.
2. **Setup packaging removes avoidable heuristic triggers.** The amd64 setup
   uses Inno's native x64 loader, `zip/9` compression, and non-solid streams.
   Acceptance reads the PE header and fails if either the setup loader or a
   payload executable is not x64. These constraints remain mandatory for both
   CI and public setup builds.
3. **Alpha.8 through alpha.11, alpha.13 through alpha.64, and alpha.66 through
   alpha.76 reviewed unsigned setup are explicit exceptions.** The tag workflow may publish only those
   listed unsigned setup versions with the
   `ReviewedUnsignedPublic` verification policy and the completed Microsoft
   case ID above. Alpha.9 corrects installer-script argument handling without
   changing the hardened Inno packaging shape. Alpha.10 retains that shape and
   corrects managed llama.cpp cold-load readiness, managed TTS verification,
   and worker process-tree cleanup. Alpha.11 retains the hardened package shape,
   restores prerelease update discovery, and materializes Faster Qwen model
   files outside Hugging Face snapshot links for clean Windows installs.
   Alpha.13 retains the hardened package shape and ships the Ollama GGUF and
   repaired-chat corrections after alpha.12's portable release was withdrawn.
   Alpha.18 through alpha.21 retain the hardened package shape while shipping
   reviewed Autopilot and pattern-pacing corrections. Alpha.22 retains that
   package shape and corrects normalized pattern continuity. Alpha.23 retains
   the same package shape while adding selectable Dynamic LLM motion and the
   update-panel layout correction. Alpha.24 retains that package shape while
   adding calibrated Handy model speed, longer organic Dynamic motion, and the
   compact Chat control redesign. Alpha.25 retains the same package shape and
   corrects a Creative-plan endpoint rounding panic without changing installer
   behavior or payload composition. Alpha.26 retains it while adding explicit
   Creative stroke-length envelopes. Alpha.27 retains it while improving
   Creative reversal easing, short-window variability, and conversational
   correction validation; none changes installer behavior or payload
   composition.
   Alpha.28 retains it while adding C2 Creative phrases and the eighth motion-
   change-rate level. Alpha.29 retains it while adding elapsed-phrase planning,
   making Creative the default, and retiring six physically deselected built-in
   patterns. Alpha.30 retains it while correcting compiled Creative timing,
   perceptual phrase context, and deadline presentation. Alpha.31 retains it
   while improving autonomous phrase contrast, spoken-beat novelty, managed
   inference responsiveness, and redundant one-section response handling.
   Alpha.32 retains it while correcting the default-persona settings deep link.
   Alpha.33 retains it while correcting bounded position-aware startup and the
   autonomous unsafe-state retry loop. Alpha.34 retains it while improving
   qualitative Creative range autonomy and preventing recovery from overriding
   an explicit Pause. Alpha.35 retains it while removing quantization-induced
   stationary edges from Creative retarget and append frames. Alpha.36 retains
   it while calibrating effective Creative pace, improving long-stroke flow,
   retaining stopped-run diagnostics, and repairing empty Autopilot startup
   decisions. Alpha.37 retains it while making the Chat composer follow the
   effective conversation persona. Alpha.38 retains it while adding the
   authenticated HTTPS foundation, account access GUI, profile images, linked
   control-profile selector, and protected-setup login handoff. Alpha.39 retains
   it while simplifying managed Voice settings and hardening the optional
   Parakeet repair path with app-owned download verification. Alpha.39 changes
   optional-module behavior inside the existing payload, but does not change
   the Inno Setup loader, compression, artifact set, or payload composition.
   Inno Setup never receives or stores an account password.
   The tag workflow scans each exact candidate directory with Microsoft
   Defender before lifecycle verification. The
   verifier rejects every other version, so a later unsigned setup requires a
   new reviewed policy change. It builds setup, portable ZIP, and two-entry
   checksum into one dedicated `artifacts/release` directory, runs the full
   lifecycle against that exact setup, and publishes only the three explicit
   paths. An ordinary `UnsignedCI` build cannot enter a GitHub Release.
4. **Alpha.12 remains withdrawn and immutable.** Its portable-only GitHub
   Release was removed, its source tag is not moved or reused, and the corrected
   distribution uses the next prerelease ordinal, alpha.13.
5. **Trusted Authenticode remains the production target.** `SignedPublic`
   requires a protected organizational signing identity, trusted timestamp,
   and `Valid` Authenticode status on the setup executable and every shipped
   payload executable. Verification pins the approved signer thumbprint and
   rejects self-signed certificates. The reviewed unsigned exception does not
   satisfy this long-term publisher-identity requirement.
6. **Signing is a release operation, not a source-build dependency.** Developer
   and CI lifecycle builds remain possible without signing credentials. The
   eventual signing service must expose credentials only to the protected tag
   workflow and must not make private key material available to pull requests.
7. **Warnings are not an installation step.** Documentation must not advise
   users to disable Defender, ignore a malware classification, or use a
   SmartScreen bypass. A newly detected public artifact is withdrawn and
   investigated before another version is published.
8. **Published tags remain immutable.** A withdrawn version keeps its tag and a
   source-tree notice explaining the withdrawal. A corrected release uses the
   next SemVer prerelease ordinal.

Microsoft Artifact Signing and SignPath Foundation are viable protected signing
services. Selecting and provisioning one is an infrastructure decision that
requires organization validation and credentials outside this repository; the
repository gate does not pretend those credentials exist.

## Consequences

Positive:

- Microsoft has adjudicated the original severe Defender result instead of the
  project asking users to override it;
- the exact public setup receives the complete clean-runner lifecycle test;
- public and CI policies remain distinct and cannot be mixed by a broad glob;
  and
- signed setup publication will fail closed on missing signatures or
  timestamps.

Negative:

- public alpha executables still have no publisher identity and may show
  reputation warnings until signing is provisioned;
- Microsoft's determination covers the submitted alpha.6 hash, not alpha.8,
  alpha.9, alpha.10, alpha.11, alpha.13, or any later package; the later exceptions therefore add
  an exact pre-publication Defender scan but still do not establish publisher
  identity;
  and
- a trusted signing service and identity-validation process are still needed
  to retire reviewed unsigned setup exceptions.

## Verification

- `Test-WindowsRelease.ps1 -ArtifactPolicy UnsignedCI` requires the full
  unsigned setup/portable/checksum set, verifies x64 PE machine headers, and
  supports installer lifecycle tests.
- `Test-WindowsRelease.ps1 -ArtifactPolicy PortablePublic` requires exactly a
  portable ZIP and one-entry checksum file and rejects any setup executable.
- `Test-WindowsRelease.ps1 -ArtifactPolicy ReviewedUnsignedPublic` requires an
  alpha.8 through alpha.11, alpha.13 through alpha.64, or alpha.66 through
  alpha.76 version, the recorded Microsoft case ID, the
  setup/portable/checksum set, x64 PE headers, unsigned status, exact hashes,
  and supports the complete installer lifecycle.
- Alpha.9 through alpha.11, alpha.13 through alpha.64, and alpha.66 through
  alpha.76 reviewed setup workflows run Microsoft Defender against the exact public artifact directory
  before verification or release creation.
- `Test-WindowsRelease.ps1 -ArtifactPolicy SignedPublic` requires valid,
  timestamped Authenticode from the explicitly pinned signer on the setup
  executable and every payload EXE.
- The tag workflow publishes only the explicit setup, ZIP, and checksum paths
  under `artifacts/release`.
- Installer integration tests require the reviewed policy, case ID, exact setup
  path, and full lifecycle switches in the tag workflow.
