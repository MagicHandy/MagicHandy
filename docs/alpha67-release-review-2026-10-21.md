# Alpha.67 release review — 2026-10-21

Dates use America/New_York. The maintainer requested merging PR #337 after
green CI and releasing an update. PR #337's exact head `47417dc8` passed every
check and was merged as `fa1ce443`. The changes since alpha.66 are limited to
that PR; the installer startup-test correction was already in alpha.66.

## Source and acceptance review

The sign-in form now reads submitted DOM values so a password manager's silent
fill cannot leave the submit action disabled or send stale React state. Native
required-field validation remains, bootstrap password confirmation still reads
the submitted values, and successful authentication clears the password field.
Tests cover silent fills surviving a render and normal required-field behavior.

The browser boundary admits only top-level GET/HEAD document navigation with no
Origin header to non-API routes on an approved Host. Cross-site API requests,
login and settings writes, script fetches, embedded frames, foreign Origins and
unconfigured Hosts remain rejected. Cache variation includes Origin and Fetch
Metadata while preserving existing Vary fields. Main and remote interfaces have
regressions for loopback and configured public hostnames. Authentication,
controller ownership, network policy, frame protection and Stop are unchanged.
ADR 0029 and the deployment documentation record the precise exception.

The navigation regression failed before the fix. The full Go suite, vet, lint,
pure-Go build, frontend typecheck, 921 frontend tests and canonical bundle build
passed. Candidate CI also passed race, installer, optional Labs, both voice
adapter versions and Windows packaging/lifecycle checks. Actual Edge navigation
with cross-site/navigate/document headers loads sign-in, while a corresponding
private API request is rejected. The maintainer confirmed Firefox sign-in on
the isolated review instance. Its selected local Ollama model completed the
required real generation. These checks used simulated motion and disposable
review data; the installed instance was left running and unchanged.

## Release preparation and boundaries

Only `0.1.0-alpha.67` is added to ADR 0014's reviewed unsigned policy. The next
version and unpublished alpha.65 remain rejected. This is a version-specific
amendment, with no change to source or exact-binary vulnerability scanning,
Defender scanning, manifests/checksums, x64 packaging, installer lifecycle,
green CI, or the requirement to tag the exact current main tip. The prior
Microsoft case does not approve the new hashes. Trusted Authenticode remains
the long-term requirement.

The five payload executables, native x64 setup, non-solid ZIP and three public
assets are unchanged. No dependency or runtime worker is added. The scorecard
records the implementation's +104 B gzip main JavaScript and +512 B comparable
stripped core; release preparation changes only policy, assertions and docs.
The notes retain unsigned packaging, update instructions and existing hardware
acceptance limits. Publication remains conditional on green merged-main and
tag checks, including scanning all five exact Windows payload executables and
the exact public setup lifecycle on a disposable CI host.
