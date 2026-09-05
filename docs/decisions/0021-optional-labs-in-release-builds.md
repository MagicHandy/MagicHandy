# ADR 0021: Optional Labs in every release

- Date: 2026-09-04
- Status: Accepted
- Supersedes: build-time Labs availability in ADRs 0019 and 0020

## Context

The user requested access to the motion and LLM laboratories through Settings
in any release. Their original development-only UI and Go build switches made
that impossible without a different executable. Availability must now be an
explicit saved choice while retaining the shared engine, authoritative state,
unconditional Stop and isolated experimental chat behavior.

## Decision

All ordinary releases compile the existing Labs endpoints and include one
canonical `web/dist`. `labs.enabled` is a persistent settings domain with a
default of false. Settings > General > Enable Labs writes it immediately via
the controller-gated `PUT /api/settings/labs` endpoint. General settings updates
preserve the field when it is omitted, so older clients cannot accidentally
turn Labs off when saving another preference. `/api/state.labs_enabled` reflects
the saved value for navigation and route access.

The small Labs route gate ships in the main UI; the actual workspace and its
stylesheet load on demand after the enabled workspace is opened. Disabled
bookmarks point users to Settings. Retire the alternate embed, Vite exclusion
plugin, disabled UI alias and application build-mode constants. The historical
`build:labs` helper builds the same standard executable. The `magichandy_labs`
tag remains for the offline motion atlas exporter, not app availability.

Every Labs endpoint checks the saved flag and registers cancelable work. The
shared motion start endpoint also checks the flag before admitting a lab
audition, before stopping prior work or compiling its target. Disabling the
setting synchronously cancels registered requests, then drains lab startup
admission and stops a lab-owned engine through existing teardown. Other active
motion is left alone. Lab trial commits share the cancellation lock so a late
response cannot apply after disabling, even across disable/enable cycles.

The gate changes neither pattern generation nor model contracts. The main chat
history remains independent; saved observations remain review evidence and are
never automatically applied to prompts, preferences, training or motion. The
setting does not connect a device, start motion or load a model by itself.

## Validation and costs

Runtime tests cover default-off access, controller restrictions, malformed
updates, persistence/reopen, compatibility with unrelated settings saves,
audition shutdown, canceled startup, rejected late replies after re-enabling,
preserved observations and uninterrupted ordinary motion. These tests now run
in the normal Go suite; the optional-Labs CI job uses the normal release build
and preserves the existing motion/race/build gates.

The release contains additional code and localized strings. Lazy loading keeps
the workspace component tree and CSS out of the initial page. No dependencies
or alternative motion loop are introduced. Affected bundle, binary and runtime
measurements are recorded in the goal scorecard. Previously published binaries
cannot gain the feature without an update.
