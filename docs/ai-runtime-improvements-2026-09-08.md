# AI runtime improvements — 2026-09-08

Base: `c1f7b7cbb75b17c04d8bb2702ef2a577a6e96faa` (alpha.43).
Implementation branch: `codex/ai-runtime-improvements`.

The implementation addresses the twelve reproducible or traced findings from
the AI module audit. It prioritizes lifecycle correctness, bounded resources
and progressive delivery across the existing worker boundary. No native model
framework enters the core, and no Go or JavaScript dependency is added.

## Coverage and resulting behavior

| Area | Finding / optimization | Result |
| --- | --- | --- |
| Managed llama.cpp | AI-02, AI-10 | `Close` permanently retires a provider; `Unload` stays reusable. A retired handle cannot restart its runner. Short status/operation deadlines remain effective under longer parent contexts. |
| External llama.cpp and Ollama | AI-10, O-03 | Bounded combined input and phase diagnostics. Reasoning activity is distinguished from visible output without retaining reasoning text. Ollama and llama.cpp timing/token fields are captured when available. |
| Chat, modes, Autopilot and labs | AI-02, AI-03 | Ordinary chat and Autopilot resolve providers after admission; settings changes reject old queued generations and hold new admission through retirement. Status cannot evict an active lab override. Existing shared-engine motion contracts remain intact. |
| Memories, persona/lore and model management | AI-03 | At most 6 KiB of complete memory entries, selected by lexical relevance then recency, shrinking to fit small contexts. Old history can be dropped; system rules and the current/repair exchange are preserved. Oversized essential input fails explicitly. Verified model import/deletion architecture is retained. |
| Parakeet and compatible ASR | AI-08, O-04 | Owned-child exit clears cached readiness. Staged audio is read through a multipart reader with an exact Content-Length, avoiding the worker's whole-file and whole-multipart copies. |
| Qwen3-TTS | AI-05, O-01 | Bounded async bridge replaces the blocking completion sentinel/executor read. Disconnect cancels the producer and skips later model calls. Seed, instruction, conditioning cache and warmup behavior are preserved. |
| Chatterbox | AI-06, AI-12, O-01 | Synthesis leaves the event loop, produces bounded sentence audio and uses only a private speech/health app. The upstream management/upload UI and permissive CORS routes are not exposed. |
| OpenAI-compatible TTS and ElevenLabs | AI-09, AI-11, O-01/O-05 | HTTP audio is forwarded progressively; speech and core retention share an 8 MiB ceiling. OpenAI-compatible synthesis uses a consistent ten-minute budget. An ElevenLabs `/v1/user` 403 no longer prevents a scoped key from reaching speech; 401 and speech errors remain authoritative. |
| Voice supervision, browser playback and VAD | AI-01, AI-07, AI-09, O-01/O-05 | Atomic terminal states; bounded stdin/control writes; cancellation reaches queued adapter jobs. Short noise resets at the silence boundary. PCM/WAV plays before generation completes, in the existing delivery order. Stop, lease/backend loss and stale tokens revoke pending playback. |
| Installation and updates | AI-04, AI-12 | Permanent versioned candidates keep package/source/model writes away from a live worker. Candidate validation precedes settings activation and index promotion; prior runtimes remain available for manual recovery. Shared uv downloads and copied Qwen model seeds reduce repeat download work. |

The shared adapter job tracker also fixes a related bug found during
implementation: unload previously canceled active inference but could leave
queued jobs eligible to run after the model reloaded. Unknown cancel IDs now
consume no retained job state.

The final bug pass found and covered partial stdin writes with errors: these
retire the connection just like a timeout or short write, preventing another
JSON frame from following an incomplete one.

The narrow-browser visual review also caught the expanded diagnostics clipping
inside the chat scroll container. Its tooltip now renders outside that container
and fits within the viewport, with bounded scrolling and the Stop region clear.

## Measured effects

Windows/amd64, Go 1.26.4, Ryzen 9 9950X3D. Baseline and candidate use
`CGO_ENABLED=0 go build -trimpath -ldflags '-s -w'` and their respective shipping
UI. The comparison uses no installed user data or credentials.

| Measurement | Baseline | Candidate |
| --- | ---: | ---: |
| 200 synthetic 1,998-byte memories, same utility-chat system prompt | 401,604 B | 7,604 B |
| Multipart assembly/drain for already-present 32 MiB audio, allocations | 33,566,577–33,566,661 B/op | 35,881–35,884 B/op |
| Same multipart benchmark, three runs | 1.862 / 2.168 / 2.070 ms/op | 0.387 / 0.386 / 0.403 ms/op |
| Stripped Go app | 19,116,032 B | 19,183,104 B (+67,072; 0.35%) |
| Main JS raw / gzip level 9 | 755,229 / 208,618 B | 763,057 / 211,013 B (+7,828 / +2,395) |
| Complete embedded `dist`, raw | 2,011,769 B | 2,020,773 B (+9,004) |

The memory fixture retains stored memories and sends three complete entries;
this is prompt-size reduction, not a tokenizer or inference-speed claim. The
multipart microbenchmark excludes disk/network/recognition and compares the old
buffered builder against the new reader using the same input. Allocation count
is slightly higher (54 versus 44–45) while allocated bytes fall by about 99.9%.

Labs JS raw bytes are unchanged (gzip +1 byte). Each lazy non-English locale adds 310–367 raw bytes
and 142–161 gzip bytes. The added frontend code pays for progressive PCM parsing,
bounded playback and the new diagnostics, without a decoder dependency.

Fresh launch observations were 603.9/588.1 ms for baseline and 610.2/595.6/592.6
ms before the final tooltip correction. An immediate paired rerun was 588.1
versus 592.6 ms. The final UI build observed 585.7 ms and 34,082,816 B working set.
The 500 ms startup target remains unmet on this host.
Windows working-set readings varied substantially between launches: baseline
22,491,136–68,653,056 B; candidate 25,305,088–67,874,816 B. Private commitment was
approximately 57–59 MB. No idle-memory improvement or waiver closure is claimed.
Details and sampling conditions are in [the performance baseline](perf-baseline.md).

## Validation

- Full `go test ./...`, `go test -race ./...`, `go vet ./...`, gofmt,
  golangci-lint (zero issues), architecture checks and motion/transport goleak
  gates pass. A final voice-only race pass covers the partial-write correction.
- Frontend typecheck, localization, all **68 files / 502 tests**, and the
  canonical production build pass. No stale alternate bundle is retained.
- Windows PowerShell **5.1** installer fixtures pass, including exclusive
  candidate creation, unchanged active files, model-copy independence and saved
  choice/plan-only behavior. PowerShell 7 is not the fixture suite's target host.
- Seven lightweight Python adapter tests pass locally. CI runs them on Python
  3.10 and 3.11 using fake engines; no Torch/model download is required.
- Regression coverage includes late canceled responses, blocked stdin and full
  response-queue teardown, dead Parakeet readiness, scoped ElevenLabs keys,
  provider retirement/admission, complete repair-tail preservation, minimum
  managed-context compatibility, Qwen disconnect with a full queue, Chatterbox
  responsiveness/private routes, split WAV headers/samples, and initial audio
  delivery before worker HTTP EOF/browser generation completion.

The first full race run identified a fixture setup race in two newly added
HTTP tests: they replaced a provider while startup autoload was running. The
fixtures now stop autoload before replacing it; the full race gate passes.
No safety or validation gate was relaxed.

## Review app

The isolated current-source app is left at
`http://127.0.0.1:49971/#/chat`, using simulator mode, LLM motion Off and voice
disabled until explicitly configured. Existing user-launched sessions are
preserved. No hardware connection or motion command was issued.

`scripts/check-review-llm.ps1 -BaseUrl http://127.0.0.1:49971` passed against the
available local Ollama model `huihui_ai/granite4.1-abliterated:3b`. A separate real
request through the app's chat path returned “The text-only AI review session
is ready.” One provider call, no repair/fallback, 53 ms to first visible token,
44 ms provider-reported prefill and 122 ms generation (123 ms overall). The response diagnostics
are expanded in the browser. This is one short warm readiness check, not a
general model benchmark.

## Explicit acceptance limits and follow-up

- **O-02 GPU/coexistence tuning remains deferred.** This PR does not change
  offload, KV quantization, batch sizes, model placement or eviction policies
  without representative latency, VRAM and quality measurements. The new phase
  diagnostics and bounded work provide evidence for that next tuning pass.
- No actual Qwen/Chatterbox/Parakeet model generation or listening evaluation,
  paid ElevenLabs call, full multi-GiB installation, shared-GPU contention test,
  or physical-device test was performed for this PR. Fake-engine tests prove
  delivery and lifecycle behavior, not model quality or first-audible latency.
- Chatterbox's per-sentence clipping normalization can differ from the previous
  global normalization. Listen for level changes, joins, truncation and speaker
  drift before release. MP3/Opus still buffer for final encoding.
- A synchronous GPU call must return before producer cancellation can finish.
  Subsequent calls are skipped and overlapping inference remains serialized.
- Staged runtimes consume additional disk and are retained, not automatically
  rolled back or deleted. Candidate preparation verifies native imports and
  model files; a new Qwen installation still needs user-provided conditioning
  before its model-load warmup can validate real generation. A settings/index
  split failure is reported and leaves settings authoritative.
- Conservative byte admission can drop useful history sooner than an exact
  tokenizer would; external servers can have smaller contexts than the default
  application policy. Essential prompts are rejected rather than silently cut.

Architecture is recorded in [ADR 0028](decisions/0028-ai-runtime-lifetimes.md).
Machine-local traces, fixtures, measurements and test environments remain in
ignored `.scratch/ai-module-review/`; they are not release assets.
