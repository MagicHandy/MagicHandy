# Claude progress completion review — 2026-09-29

Claude's alpha.48–alpha.50 work was already merged and released. The remaining
implementation was PR #284 (`claude/video-metadata-remote`, reviewed from
`453ff2da`): video curation, a larger watch page with chat beside it, the
Script/Chat/Off choice and a phone remote for video or desktop chat. This pass
reviewed its complete diff and exercised the integrated application. Existing
motion recipes, transports and LLM mode defaults were not changed.

## Findings addressed

| Priority | Reproduction or failure | Resolution |
| --- | --- | --- |
| P1 | An SSE command buffered in the desktop could execute after queue expiry or Emergency Stop, using a newly observed Stop sequence. | A one-shot backend claim revalidates expiry, original login/grant, target and executor. Execution retains the original Stop sequence and discards late claim responses. Local Stop invalidates pending execution immediately. |
| P1 | A remote play/seek/message could affect the video or conversation opened after the command was sent. | Bind target identifiers when sending, check them again when claiming and executing, and discard pending scrubs when their video or Stop sequence changes. |
| P1 | Remote outcomes remained visible to another account after the desktop disconnected or expired; replacing its account retained history. | Redact regardless of presence, clear old-account history on replacement, and keep the private sender reference out of serialized commands. |
| P1 | The source switch announced its new owner before the previous source stopped. Its deferred script-load callback could resume after Stop. | Serialize the handoff, await Stop, keep the old choice on failure, invalidate stale completions, and require Play to start a newly selected script. Opening a video reflects existing background motion instead of calling it Off. |
| P1 | Fullscreen excluded the global Emergency Stop. Native controls on a plain video could enter fullscreen without the app controls. | Move the same Stop control into the fullscreen root and reserve space for it. Plain and paired videos share the app playback controls. |
| P1 | A curation request admitted before logout or grant revocation could commit afterward. | Revalidate login and control permission inside the catalog write transaction. No account dependency is introduced into media. |
| P2 | A late HTTP read could overwrite fresh remote stream state; stale controls remained usable. | Fence reads by stream observation and connection lifetime, abort on disable, retry failed stream construction, and hide unavailable controls. |
| P2 | Delayed catalog reads and player metadata acknowledgements could overwrite newly saved curation in the browser. | Invalidate older reads after acknowledged edits and merge only the player fields that were actually saved. |
| P2 | Unicode tags were deduplicated in Go but SQLite's ASCII-only comparison could duplicate or fail to rename them. | Store indexed Unicode lowercase keys in schema v28. Migration preserves first spelling, timestamps, curation and calibration while merging case variants. |
| P2 | A 500-video bulk edit performed up to 1,000 result reads. | Fetch acknowledged rows and tags in two bounded queries, preserving request order. |
| P2 | At phone width the edit button occupied an extra toolbar row, and millisecond clocks overlapped. | Use a compact two-row watch toolbar and whole-second playback clocks; seek accessibility values retain precision. |

## Validation

- Full `go test ./...`, Windows `go test -race ./...`, `go vet ./...`,
  `golangci-lint v2.12.2` and the pure-Go build passed. Architecture and
  goroutine-lifecycle gates remain enabled.
- `govulncheck v1.8.0` found no reachable vulnerabilities. Three advisories
  exist in required modules without affected packages/symbols used here.
- Frontend typechecking, localization checks, all **731 tests in 96 files**,
  and the canonical production build passed. Existing bundle-size warnings
  remain advisory; their thresholds were not changed.
- Migration tests cover released schema v26 and a v27 preview containing
  duplicate accented tags, including rerunning the migration.
- The isolated review app uses `-simulate-motion`, generated test video and
  funscript files, and the host's available Ollama
  `huihui_ai/granite4.1-abliterated:3b`. The required readiness script completed
  real generation. A text-only request through the application chat returned
  non-empty text without repair, fallback or motion.
- A separate HTTP phone client sent commands to the actual browser executor:
  seek, Script → Off → Script and chat send acknowledged in **124–229 ms**
  on loopback. Simulated script play, a live seek, Script → Chat → Script and
  Emergency Stop also passed; acknowledgements were **127–217 ms** in that run.
  These are local software measurements, not WAN or hardware latency claims.
- The shared engine reported following after a seek, stopped before the new
  source was acknowledged, and stayed stopped after Emergency Stop. The trace
  is retained locally at
  `.scratch/claude-progress-review-20260929/simulated-trace.json`; runtime data
  and recordings are excluded from Git.
- Desktop and 390-pixel phone layouts were inspected. The global Stop remains
  mounted for offline and read-only clients. The fullscreen relocation has a
  component regression; physical browser/phone and real-device acceptance
  remain the existing Phase 18 M3 follow-up, not a claimed hardware test.

The implementation adds no dependency, transport, sampler or motion generator.
Review motion uses the simulator exclusively. No installed-instance data or
credentials were copied, and no user-launched process was replaced.
