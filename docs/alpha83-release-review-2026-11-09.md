# Alpha.83 release integration review — 2026-11-09

## Scope and handoff gate

The owner requested review of Claude's recent changes and an update, using
Updates for release integration. Preparation starts from released alpha.82
`98f2ea83c70f43d8999a4e6a56af15900eb2cea9`. Claude PR #363
(`08cc027672ac979af0a32d49c36563b9688ab437`) concerns Edge cancellation
messages; PR #364 (`120e40188080a4e32e62d52df6c863df06fcf448`) concerns
Advanced Layered Autopilot direction and recent-score observations.

**Preparation status: waiting for the source-review handoff.** These original
PR heads are not a claim of final source acceptance. The source reviewer owns
the combined review checkout, review fixes, fresh model/evaluation probes,
compiled-motion visual review, simulator UI and a working review app. Updates
does not edit that checkout or its branches. No merge, tag or publication may
precede the reviewed source and evidence handoff.

## Release preparation

Only alpha.83 joins the reviewed unsigned public policy; alpha.65 stays
excluded and the negative version test advances to alpha.84. The active
packaging/version documentation and ADR 0014 move with that version. This
does not change any CI workflow, vulnerability scanner pin, compiler/runtime
pin, native x64/non-solid packaging contract or mandatory safety gate.

After the handoff, this record will identify the accepted source commit,
review findings and corrections, fresh evaluation/visual results and their
limits, simulator and text-only LLM checks, and budget measurements. Updates
then requires green checks on the final reviewed PR, merged main and immutable
tag, followed by independent verification of the three public downloads.
Source/five-binary vulnerability scans, exact-artifact Defender, full installer
lifecycle, manifests/checksums and exact-current-main provenance remain
mandatory. The old Microsoft case does not approve the new binary hashes.

Installed app/data and user-launched sessions are preserved. This release
task authorizes no hardware command, installed-session restart, credential
copying or repeat of the earlier private data recovery. Runtime and private
evaluation artifacts remain ignored under `.scratch/`.
