# Alpha.73 release review — 2026-10-24

Dates use America/New_York. The maintainer requested review of Claude's
completed work, a pull request and a release. The reviewed branch begins at
`4d41718b`, the alpha.72 main line. Scope is phone keepalive, background desktop
execution, phone Edge/Return, and the release preparation below.

## Review and corrections

[ADR 0055](decisions/0055-phone-remote-keepalive-and-edge.md),
[ADR 0056](decisions/0056-hidden-desktop-acts-for-its-phone.md), and the
[background desktop review](hidden-desktop-remote-review-2026-10-24.md) record
the design and browser checks. A phone in front renews only its own account's
existing live desktop lease, supplies that owner's tickets and never claims
control or revives an expired lease. Hidden desktop observations never renew
the lease. Video and chat continue through the desktop's existing controls
only while the backend reports the phone hold. Direct phone Edge uses the
same mode and engine path as desktop Edge.

Independent review found that an admitted phone Edge/Return request could
change the target after Stop or handoff retired the desktop's authority.
Four deterministic regression cases failed on the original implementation.
Binding each request to the desktop owner's cancellation path before
revalidation makes all four return conflict without changing the plan. A
local Stop cancellation test preserves the unprotected controller's existing
generation policy. Stop remains unauthenticated and always mounted.

The original notice also promised an automatic Stop for unprotected local
control, which does not enforce that expiry policy. The backend now reports
`stop_on_expiry`; only protected sessions show the 15-second Stop notice.
Go and UI tests distinguish both policies and presence states. No new
dependencies, sampler changes, transport changes or parallel motion path are
introduced. The [scorecard](goal-scorecard.md) records matched binary and
embedded UI sizes, including these review corrections.

## Validation and limits

All Go packages, vet, lint, the pure-Go build, frontend typecheck, localization,
986 frontend tests and a fresh canonical UI build pass locally. Windows
PowerShell 5.1 and PowerShell 7 installer suites and the release architecture
checks pass too. Race detection runs in Linux CI because
this host lacks the C compiler it requires. PR packaging, PR checks and merged
main checks must pass before the immutable release tag is created.

The isolated review app uses simulation and an available local Ollama Gemma
12B model without copied credentials or user data. A real generation readiness
probe passes. Desktop chat returned non-empty text in 292 ms; a second message
sent from the phone through the hidden desktop returned in 275 ms. Both used
one provider call, with no repair/fallback, LLM motion Off and the engine idle.
The desktop document was forced hidden for the browser check, past its lease
and presence windows. The phone showed the background session and its correct
local-policy notice. The retained network window contains 41 background reads,
one chat POST and no controller heartbeats (the event buffer is bounded, so
this count is not a complete traffic trace). Normal visibility was restored.
The installed instance and Claude's existing
review instance remain running without reconfiguration or restart.

Browser suspension can still halt background video/chat execution. Browser
Bluetooth disconnects on hide; muted-video timer throttling can lapse media
synchronization and safely stop its motion. Protected controller expiry and
cross-account/permission fences have automated coverage. No physical device
is connected or commanded for this review. Alpha.72's motion behavior and
known Intiface Stop, multi-Handy, COYOTE and vibrator limitations remain.

## Release policy

Only `0.1.0-alpha.73` is added to ADR 0014's reviewed unsigned policy.
Unpublished alpha.65 and future alpha.74 remain rejected. No dependency,
worker protocol, loader, compression format, public asset set or installer
contract changes. Source and five-executable vulnerability scans, exact
artifact Defender scans, provenance, manifest/checksum checks and installer
lifecycle gates remain mandatory. Microsoft's prior case does not approve the
new hashes; trusted signing remains the long-term requirement.

The tag workflow must repeat validation against current main and publish only
setup, portable ZIP and the checksum file. Public notes disclose unsigned
status, update instructions and remaining device/browser limitations.
