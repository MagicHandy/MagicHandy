# Alpha.65 release review — 2026-10-20

The maintainer requested an update including PR #329, alongside the startup
recovery changes in PR #332. The integration preserves the exact authentication
head `0fa45cec8ed650dceee853583c26b8b4e1c68a59` and startup head `3092d6a2` above
alpha.64 main `4a46c0ff`. Merge conflicts were additive documentation and locale
entries; the canonical frontend was rebuilt from the combined source.

Publication was subsequently blocked by the final Windows binary vulnerability
scan for GO-2026-6809. No public artifacts were uploaded. The source tag remains
immutable; [alpha.66](alpha66-release-review-2026-10-20.md) carries the same
features with the patched Go toolchain. This record retains the original
feature review and candidate validation evidence.

## Independent source review

The release review inspected PR #329's credential envelope and DPAPI calls,
legacy migration and atomic writes, per-registration refresh serialization,
account revision/generation checks, callback and disconnect lifetime, recovery
admission, and the frontend confirmation flow. No blocking issue was found in
that delta. The broader hosted-model architecture in ADR 0039 remains Proposed;
this release review does not claim a new live ChatGPT login or complete review
of every hosted-model feature.

- Windows protection uses current-user DPAPI with UI forbidden; it never uses
  machine-wide encryption or falls back to plaintext. The existing owner/SYSTEM
  ACLs remain. Non-Windows platforms keep owner-only file storage.
- Legacy JSON migrates atomically while retaining registrations, client and
  host identities, account selection and provider-key bindings. Unchanged reads
  do not rewrite ciphertext. Unknown or unreadable stores are preserved.
- A confirmed host-administrator/controller request can reset only an unreadable
  store. A store repaired by another instance is rechecked and cannot be erased
  by a stale reset. Model/provider definitions are stored separately.
- Network discovery, exchange, verification, refresh and revocation run outside
  the manager state/storage transaction. Separate per-registration locks
  serialize rotating tokens across processes while Status remains available.
  Generation and persisted revisions reject late results after sign-out or
  account switches. Cancellation and Close retain the lifecycle gate.
- Startup's profile lock precedes the datastore and worker graph. HMAC instance
  verification never exports its token; explicit fixed ports and saved hosted
  policies remain strict. The startup records are allowlisted, bounded and
  included through the existing diagnostic ZIP's redaction/admission path.

The Opus startup review and its focused follow-up are recorded in the
[startup review](startup-recovery-review-2026-10-20.md). The Linux CI lint finding
was a directory-versus-file permission false positive: the local inline
justification retains 0700 traversal for its owner and no group/other access.
Linux-targeted lint passes; no global linter or security gate was weakened.

## Validation and acceptance limits

The feature records contain the original complete Go/race, frontend and Windows
installer results. The combined release source passed the full Go and race
suites, both platform lint targets, vet, pure-Go Windows build, frontend
typecheck/localization/919 tests/build and both PowerShell installer suites.
Windows packaging additionally runs the real DPAPI/migration tests, rather than
relying on Linux's owner-only storage fallback. The release CI results remain
publication gates; local checks do not substitute for them.

The isolated simulator at `http://127.0.0.1:7803` uses local Ollama Gemma 4 12B
QAT without copying, migrating, resetting or using existing user credentials.
`check-review-llm.ps1` passed a real generation. A text-only app chat returned
"Hello! How can I help you today?" in 5,441 ms, including 5,055 ms model loading,
one provider call and no repair, fallback or motion. These are readiness results,
not a performance comparison. Its ordinary diagnostic ZIP includes the retained
port recovery. The acceptance uses synthetic OAuth and actual Windows DPAPI
fixtures; it does not claim a live hosted-account or physical-device run.

An initial parallel PowerShell 7 validation run hit native 10048 on a selected
fixed test port. The app correctly refused that bind; the isolated repeat after
the other local processes finished passed. The original failure log is
retained with the local evidence rather than weakening fixed-port assertions.

One release-candidate Linux race run observed an empty Autopilot decision source
after the segment index was armed but before its provenance was recorded. The
test helper now waits for both startup observations before callers inspect the
source; the model/fallback assertions and production motion code are unchanged.
The complete modes race suite passed 20 repetitions, and the model/fallback
startup tests passed 500 repetitions. The failed CI run remains part of the
release evidence; the updated candidate must pass the full CI gates again.

## Packaging and release boundaries

ADR 0014 and the verifier admit only `0.1.0-alpha.65` in addition to previously
reviewed releases; alpha.66 remains rejected. The five payload executables,
native x64 setup, non-solid portable ZIP and three public assets are retained.
No Go module or external runtime dependency is added. Public executables retain
symbol tables for exact-binary vulnerability scans. The scorecard records the
combined core and embedded UI sizes against alpha.64.

Only the clean current main tip is tagged after reviewed PRs and green CI.
The tag workflow repeats quality/adapter tests, builds that exact source, scans
the exact public files with Defender, verifies manifests/checksums and exercises
setup/default-install/over-install/uninstall acceptance before publication.
The prior Microsoft case does not approve the new hashes. Trusted Authenticode
remains the long-term requirement; the existing reviewed unsigned warning stays.

Public notes disclose that Windows credentials become tied to the current
Windows identity, old versions cannot read the migrated envelope, and a move
may require provider sign-in again. Recovery cannot revoke inaccessible remote
grants. ADR 0049's Intiface Stop limitation, unverified loaded COYOTE/vibrator
behavior and native-worker crash recovery limits remain disclosed.
