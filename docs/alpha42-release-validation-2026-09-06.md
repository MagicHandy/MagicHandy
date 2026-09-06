# Alpha.42 release validation — 2026-09-06

The user requested merging PRs #253–#257 and publishing an update. All five PRs
had green checks before merge; main at `4268f0e9` exactly matched the tested
stack tip `57b65847`. Its post-merge [Go test run](https://github.com/MagicHandy/MagicHandy/actions/runs/34051887247)
then failed `TestAutopilotStopsInsteadOfRetryingUnsafeStartupState`:
`unsafe startup decisions = 2, want one attempt with no retry loop`.

## Investigation and correction

This was a scheduling race in production behavior, not an assertion to relax.
`handleStartFailure` recognized `motion.ErrUnsafeStartupState` but deferred all
cancellation to `stopLoopAtGeneration`. Before that goroutine ran or obtained
the lifecycle lock, another ready scheduler tick could request a second model
decision and attempt startup again.

Cancellation of the current operation and its loop now happens synchronously
under the manager state lock, after checking the failed generation still owns
the mode. Asynchronous teardown still waits for the loop and retains the
generation guard so it cannot stop a newer run. Start-operation admission also
rejects an already-canceled parent context. Transient failure backoff, semantic
targets, motion geometry, physical startup acquisition and transport ownership
are unchanged.

`TestUnsafeStartupCannotRetryWhileTeardownWaits` holds the lifecycle lock and
drives two scheduler ticks explicitly, advancing the fake clock one hour
between them. It reproduced the original two-decision failure before the fix.
Afterward it observes exactly one decision, an immediately canceled loop and
closed admission while teardown is still blocked. The original end-to-end
regression remains intact. Both tests pass 100 repetitions normally and 100
with the race detector; the modes package retains its goleak TestMain.

The original failed CI log is retained at
`.scratch/release-alpha42/failed-merge-check.log`. It is not hidden by disabling
the test or weakening the one-attempt requirement. Fresh release-PR, main and
tagged-release gates remain required before publication.

## Publication policy

Automatic approval review initially rejected a new unsigned-release allowlist
entry without specific authorization. The user then explicitly approved the
single alpha.42 exception and requested this failure investigation before
release. [ADR 0014](decisions/0014-public-windows-signing-gate.md) records that
decision. The verifier and its exact-list assertion add only `0.1.0-alpha.42`;
alpha.12 and future versions remain rejected. The existing tag workflow still
requires the exact main tip, Go/race/lint/frontend checks, canonical assets,
Microsoft Defender, package manifests/checksums, and installer acceptance.
