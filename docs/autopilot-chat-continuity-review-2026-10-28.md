# Chat interrupting an Autopilot upload — 2026-10-28

## Observed failure

The installed alpha.77 instance used Original Handy Cloud, Layered, Autopilot,
and ChatGPT GPT-6.1 Terra. A manual pace-variation request stopped motion, then
motion restarted. Read-only saved-chat evidence and the authenticated live
motion trace identify this sequence (UTC):

| Time | Trace evidence |
| --- | --- |
| 14:13:39.782 | Autopilot commits a new target on the existing stream. |
| 14:13:40.046 | Chat activity supersedes Autopilot; its point-upload request fails at the same timestamp, after 166 ms. |
| 14:13:40.324 | `recovery_retarget_points` confirms an explicit recovery Stop. |
| 14:13:46.414 | Terra returns a valid `start` action, one provider call, no repair or fallback, 6,367 ms request duration. |
| 14:13:48.962 | Startup positioning playback begins. |
| 14:13:52.163 | Normal pattern playback starts after positioning and prebuffering. |

The preceding successful append acknowledgements were approximately 315–349 ms.
The failed upload was canceled early rather than exceeding the normal request
timeout. Its raw error is redacted, so the exact cancellation cause is confirmed
by the code path and an executable reproduction rather than an unredacted log.
Chat calls `NotifyChatActivity`, canceling the active mode-operation context.
That context also owned `Engine.ApplyTarget`'s in-flight upload. The engine
stopped because the device might already have accepted the canceled command.

The model's `start` was a response to the stopped engine. It did not request the
pause. No wording-dependent interpretation is involved.

Private chat, device identifiers and raw diagnostic exports remain outside git.

## Change

Retargeting an already-running engine separates request admission from dispatch:

- A canceled request waiting for target ownership is rejected without changing
  the target or sending commands.
- Refilling the existing stream may finish even if the requesting planner is
  superseded. The caller is checked again before the new target commits.
- An admitted target upload completes on a bounded ten-second context that is
  still canceled by the active run. This prevents routine chat preemption from
  fabricating a transport failure.
- Stop, Pause, run replacement, real transport errors and startup cancellation
  retain their existing safety behavior. Genuine uncertain uploads still stop.

This uses the shared engine and existing transport interface. It changes no
prompt, schema, speed mapping, generated geometry, sampler, transition fitter,
device limits, connection setting or provider default.

## Validation

The integration regression runs a real mode manager and motion engine with an
inert transport that delays an Autopilot upload acknowledgement. Chat is sent
at that boundary, then a manual target is applied. The original code reproduced
the reported stopped-engine state. With the fix, ten repeated runs retained a
single stream, exactly one initial Play, no intervening Stop, and the manual
target was adopted.

Additional engine tests cover canceled queued targets, cancellation during
existing-buffer refill, cancellation after target commitment, Pause and Stop
during the upload, and a true transport timeout after request cancellation.
Startup cancellation and uncertain transport-failure tests remain required.

Completed broader validation:

- `go test ./...` and `go test -race -timeout 20m ./...` passed, including the
  motion/modes lifecycle and import-boundary gates.
- `go vet ./...`, `golangci-lint run ./...`, formatting and diff checks passed.
- The `CGO_ENABLED=0` application build passed.
- Frontend typecheck/localization, all 1,069 tests across 130 files, and the
  production build passed. The canonical embedded bundle is unchanged.
- The isolated current-source review app at `http://127.0.0.1:50308/#/chat`
  passed `scripts/check-review-llm.ps1` against available local Gemma 12B. Its
  text-only app chat returned a nonempty answer in 306 ms with no repair,
  fallback or motion. Autopilot and LLM motion remain off; devices are simulated.

## Power-interruption integrity check

A newly added regression-test file was found filled with NUL bytes after a
power interruption. It was restored from the authored patch before the passing
checks above. An independent content-hash audit reread all 2,095 tracked files,
with Git clean filters applied rather than trusting filesystem timestamps.
Only the four intentional tracked edits differed from the index. All 2,094
text/source files, including the three new files, were scanned for NUL bytes;
none remained. `git fsck --full --no-dangling` passed with exit code zero and
no object-integrity errors. The canonical frontend was rebuilt and matched its
tracked contents. The separate integrity report is retained in ignored review
evidence.

## Physical follow-up

This investigation reads the installed instance without connecting, controlling
or moving hardware. Automated reproduction uses an inert transport. When the
next device test is explicitly authorized, send chat during repeated
Layered Autopilot boundaries on Handy Cloud and confirm uninterrupted motion,
then verify Stop during an update. Capture the trace and acknowledgement
latencies. Numerical continuity does not establish physical smoothness.
