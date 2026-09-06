# Video playback and live seeking review — 2026-09-06

Parent: `e16ee9f9` (`codex/data-observation-efficiency`, PR #255). This review
traces the browser's paired controls, decoder readiness, clock alignment,
request/session fencing, range streaming, script loading/cache invalidation,
filtering, and compilation/startup through the shared motion engine.

## Findings and changes

1. **High: smoothing could stall a seek for seconds.** Each removed reversal
   rebuilt all reversal anchors and shifted the remaining points. Dense small
   reversals made both work and cumulative allocation quadratic. A 100,000-action
   synthetic script took 9.99 seconds and allocated 100.9 GB in one bounded
   reproduction. `reversalFilter` now maintains linked point, nonzero-edge and
   anchor indexes, plus a minimum-index candidate heap. Only neighboring
   eligibility changes after a deletion. Work is O(n log n), storage O(n), and
   the original leftmost-first removal policy is preserved. Plateaus still use
   their last point as a reversal anchor; endpoints and monotonic detail remain.
2. **High: Stop/controller loss during a scrub could allow queued resume.**
   The old early-return condition looked for active/arming/buffering state,
   although a scrub had already cleared those flags. Both transitions now
   revoke pending seek/filter continuations and request generations. A failed
   seek Stop also returns a failed barrier instead of being swallowed and
   followed by an automatic arm.
3. **Medium: old heartbeat responses/errors overwrote newer playback.**
   Response publication now checks session identity, playback generation,
   event sequence and cancellation. Replacing playback aborts the old heartbeat;
   its late success or failure cannot change the current seek or playing state.
4. **Medium: overlapping seeks could resume according to an obsolete target.**
   Every commit owns a new continuation ID and inherits the outstanding Stop.
   Releasing a second scrub before that Stop completes replaces the first
   commit. Seeking beyond the script and then back into it correctly re-arms
   the latest target. Repeated buffering events likewise retain their original
   pending Stop instead of replacing it with an already-resolved promise.
5. **Medium: metadata writes reset a live session.** Updating the stored video
   duration no longer clears/reloads the script, synchronization state or play
   intent. Script loading has its own abort lifetime, so Stop during loading
   cancels playback admission without leaving the player permanently loading.
6. **Medium: a canceled backend arm could stop another motion source.**
   Playing requests check cancellation before and after the media lifecycle
   lock, before consuming the event fence or replacing motion. Stop events
   retain detached engine teardown even if their HTTP client disconnects.
7. **Seek preparation and interaction overhead.** Canonical media timelines
   validate every point and take one owned copy instead of sorting, copying and
   constructing a discarded curve. Noncanonical input retains the same
   normalization. The engine still validates its input and owns its sampler.
   Decoder readiness now also requires `seeking == false`; `seeked` immediately
   checks readiness rather than waiting up to the 100 ms polling interval.
   Polling remains a fallback for missing browser readiness events. Holding an
   arrow key on the script timeline previews one gesture and commits on release
   or blur, avoiding a Stop/arm request for every key repeat.

The original eight browser regressions and backend canceled-arm regression
were run before the fixes and failed. Eleven lifecycle regressions now cover these
cases, alongside the existing 21 synchronized-player tests and keyboard tests.
The filter is compared point-for-point with a frozen copy of the original
algorithm on 10,000 deterministic scripts, including plateaus, random walks,
fractional positions, irregular timing and differing thresholds. A separate
100,000-point test checks the maximum dense-chatter result and preserved detail.
Normalization tests compare legacy output/errors and verify slice ownership.

## Measured seek processing

Windows/amd64, Go 1.26.4, Ryzen 9 9950X3D. Reproduce with:

```sh
go test ./internal/media -run '^$' -bench '^BenchmarkMediaSeek' -benchmem -count=3
```

`BenchmarkMediaSeek` includes slicing, optional peak rounding and shared-engine
plan compilation for 100,000 authored actions. It excludes transport and decoder
latency. The table gives medians of three runs; allocation counts are rounded
only for display.

| Seek | Before | After | Allocated bytes before → after |
| --- | ---: | ---: | ---: |
| Start | 4.10 ms | 1.69 ms | 12,845,260 → 6,422,726 |
| Middle | 2.42 ms | 0.89 ms | 6,422,722 → 3,211,462 |
| Last tenth | 0.524 ms | 0.176 ms | 1,310,907 → 655,545 |
| Middle, rounding requested | 3.16 ms | 1.12 ms | 7,602,369 → 4,391,106 |

The dense smoothing benchmark includes slicing/filtering/normalization. Its
one-point oscillations are an adversarial workload, not a typical authored
script. The slow parent uses one iteration per size; candidate times below are
medians of three repeated benchmark runs.

| Actions | Before | After | Cumulative allocation before → after |
| --- | ---: | ---: | ---: |
| 1,000 | 1.05 ms | 0.131 ms | 5.85 MB → 0.157 MB |
| 10,000 | 114.42 ms | 1.67 ms | 785.28 MB → 1.80 MB |
| 100,000 | 9,994.61 ms | 14.04 ms | 100,866.34 MB → 18.85 MB |

The worst fixture is approximately 700 times faster. These are cumulative
allocated bytes per call, **not simultaneous resident memory**. No whole-app or
physical-device latency claim follows from the microbenchmarks. Raw output is
retained in ignored `.scratch/video-seek-review/benchmark-*.txt` and
`smoothing-before.txt`; the benchmark source ships in the repository.

## Architecture and remaining limits

Range reads already use `http.ServeContent`; no custom streaming or transport
path was introduced. Source identity is still checked before using cached
scripts, and existing edit-in-place tests remain green. Filters continue to run
after slicing/rate scaling: caching an entire filtered script would change
anchor-dependent behavior and the reported filter effect. Clock corrections
continue to move the video toward the engine's transport-aligned clock.

`SyncedVideoPlayer` remains a large orchestration component. Request invalidation
is centralized here, but a later decomposition into session, decoder and control
owners would improve maintainability. That work needs the race regressions
added here; a broad hook rewrite is outside this focused correction. The backend
media lifecycle mutex also remains a serialization point. Canceled waiters are
rejected after acquiring it; this does not promise an immediately cancellable
mutex wait. Transport setup/prebuffering and decoder keyframe distance can still
dominate real-device seeks. Their timing policies and queue sizes are unchanged.

No motion character, smoothing policy, calibration, speed cap, shared sampler,
sanitizer, transport payload construction, or Stop teardown has been redesigned.
The filter change is an equivalent implementation of existing point selection,
not a new evaluated pattern or LLM mapping. The numerical oracle and simulator
trace verify that scope; no new motion-character atlas or hardware validation
is claimed. R25's remaining supported-owner/subjective-alignment work stays open.

## Validation and browser acceptance

Full Go tests/race/vet, golangci-lint v2.12.2, import-boundary and goleak gates,
pure-Go release build, frontend typecheck/build and **492 tests in 67 files**
pass. No dependency or gate threshold changes. The single canonical `web/dist`
is rebuilt; [budget measurements](perf-baseline.md#2026-09-06--video-playback-and-live-seeking)
include the binary and browser payload deltas.

The isolated review build runs at `http://127.0.0.1:49943/#/videos`. Its generated
90-second H.264 test-pattern video and 901-action script are under ignored
`.scratch/video-seek-review/fixtures/`. Smoothing is 3%, playback 1x, offset 0;
the transport is exclusively `fake_handy` (`--simulate-motion`). The browser
demonstrated initial play, forward seek to 45.050 s, backward seek to 14.950 s,
pause, a paused seek to 30.050 s, resume and permanent Emergency Stop.

Both live seeks produced exactly one new engine arm. Trace time from the media
Stop status to the new armed status was 21.64 ms forward and 13.27 ms backward.
These two warm local samples include browser/request scheduling between events,
but do not measure display presentation or real transport latency. A subsequent
heartbeat after the backward seek reported −1 ms drift. The final trace has
45 rows, 34 successful simulator transport results, no failed results and no
drops; simulator transport latency reports 0 ms. Exports and state evidence are
in `.scratch/video-seek-review/final-trace.json` and `acceptance-summary.json`.
Motion remained stopped during the later inspection. The final visible frame
is held around 43.5 s, with the timeline, controls and permanent Stop visible.

The exact review app uses local Ollama
`huihui_ai/granite4.1-abliterated:3b`, with LLM motion Off. The required
`scripts/check-review-llm.ps1` probe completed real generation and returned
`ready`. Earlier review processes and browser tabs are preserved.
