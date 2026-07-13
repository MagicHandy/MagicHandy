# Performance Baseline

This file records the baseline evidence required by
`docs/goals-and-guardrails.md`. Measurements must exclude browser, test runner,
Ollama, llama.cpp, CUDA, TTS, ASR, and other worker/model processes from the Go
core number.

## Environment

- Date: 2026-06-30 (Go idle), 2026-07-01 (Python baseline),
  2026-07-02 (Go active Cloud REST short run and one-hour soak; Browser
  Bluetooth UI/chat hardware run), 2026-07-06 (Phase 11B SQLite persistence),
  2026-07-11 (Phase 14 pattern library, LLM model manager, managed llama.cpp,
  Phase 14B Intiface owner, Phase 14C floating connection manager, and rendered UI)
- OS and architecture: Windows / amd64
- Go toolchain: Go 1.26.3 for earlier Go rows; Go 1.26.4 for Phase 11B and
  Phase 14 measurements
- Python runtime: CPython 3.11 in the StrokeGPT-ReVibed `.venv`

## Measurements

| App | Commit | Command | Browser UI Opened | Child Worker/Model Processes Excluded | Steady RSS After Warmup | Peak RSS | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- |
| StrokeGPT-ReVibed core idle | `6c56985` (2026-06-29) | `.venv\Scripts\python.exe app.py` with `STROKEGPT_OPEN_BROWSER=0`, `STROKEGPT_PORT=5017` | No, `Invoke-WebRequest` loaded `/` once (HTTP 200) | Yes — the 4.3 MB `.venv` shim process was excluded; the spawned real interpreter was measured; no LLM/TTS/ASR model loaded, no motion, no voice | 524.75-524.81 MB across 3 samples | Not measured separately | The idle number includes ML libraries imported at startup (Torch et al.), which is precisely the core-install-path overhead the rewrite removes. |
| MagicHandy Go core idle | pending Phase 1 working tree | `CGO_ENABLED=0 go build -o $env:TEMP\magichandy-phase1.exe ./cmd/magichandy`, then run built binary with `-addr 127.0.0.1:49718` | No, `Invoke-WebRequest` loaded `/` once | Yes | 8.96 MB (9,392,128 bytes) across 3 samples | Not measured separately | `/healthz` returned `ok`; `/` returned HTTP 200. |
| MagicHandy Go core active, Cloud REST short run | Phase 9B controller/SSE working tree | temp `CGO_ENABLED=0` binary, Cloud REST configured with real Handy, `POST /api/motion/start` at 25%, `GET /api/motion/events` held open, deterministic chat `stop` | No browser window; HTTP API exercised the app endpoints and SSE stream | Yes; no Ollama/llama.cpp/voice worker loaded | 16.75-16.76 MB (17,563,648-17,571,840 bytes) across 3 samples | Not measured separately | Real Cloud REST check returned OK/HSP available; motion SSE showed running `stroke` at 25%; chat `stop` returned `Stopping motion.` and cleanup Stop was sent. This is a short safety run, not the one-hour soak. |
| MagicHandy Go core active, Cloud REST one-hour soak | Phase 9B soak-evidence working tree | temp `CGO_ENABLED=0` binary under `.tmp-phase9b-soak`, Cloud REST configured with real Handy, `POST /api/motion/start` at 25%, `GET /api/motion/events` held open, one sample per minute, deterministic chat `stop` cleanup | No browser window; HTTP API exercised the app endpoints and SSE stream | Yes; measured only the `magichandy` PID, excluding the PowerShell supervisor/SSE reader and direct-stop cleanup helper | 18.41-20.16 MB (19,300,352-21,139,456 bytes) across 56 warmed samples from 302s through 3600s | 20.16 MB (21,139,456 bytes) | 61 total samples from 2s through 3600s; all samples reported `running=true` at 25%; SSE log recorded 28,800 lines with 14,392 running events; warmed RSS range grew 9.53%, within the +20% Phase 9B gate; chat `stop`, motion stop, Cloud stop, and direct cleanup Stop all completed. |
| MagicHandy Go core active, Browser Bluetooth UI/chat short run | Phase 9B Browser Bluetooth readiness/play patch working tree | temp binary under `.tmp-phase9b-manual`, running on `127.0.0.1:49736` with dispatch owner `browser_bluetooth`; Edge Web Bluetooth selected `OHD_hw0_29b3243120f4`; visible UI Start at 28%, deterministic chat `stop`, then a repeat UI Start/Stop for RSS samples | Yes; the user's running Edge profile owned the BLE GATT link | Yes; measured only the `magichandy` PID, excluding Edge, Codex, and automation helpers | First run active sample 17.23 MB (18,063,360 bytes; post-chat-stop 18,071,552 bytes). Repeat active RSS 17.52-17.53 MB (18,374,656-18,378,752 bytes) across 3 samples | Not measured separately | Visible Check connection returned `Connected: HSP ready / Unknown / 0 ms` without queuing `hsp/state`. First run: UI Start sent `stroke_window` 97 ms, `hsp_add` 236 ms, `hsp_play` 176 ms, all `browser_ack`; chat `stop` returned `Stopping motion.` and Stop ACKed in 163 ms. Repeat run: `stroke_window` 80 ms, `hsp_add` 235 ms, `hsp_play` 116 ms, UI Stop 71 ms. Speed remained 28%, below the 40% automated-test cap. |
| MagicHandy Go core idle/API-read, SQLite persistence | Phase 11B SQLite working tree | `CGO_ENABLED=0 go build` under `%TEMP%\magichandy-phase11b-budget-*`; stripped binary run with `-addr 127.0.0.1:49750 -data-dir %TEMP%\magichandy-phase11b-budget-*\data-stripped`; `/healthz` for idle, then `/api/state`, `/api/settings`, `/api/memory`, `/api/prompt-sets` for DB-backed reads | No browser window; HTTP API exercised by `Invoke-WebRequest` | Yes; measured only the `magichandy-stripped` PID | Idle after `/healthz`: 54.13 MB (54,132,736 bytes) across 3 warmed samples. After DB-backed API reads: 54.36 MB (54,362,112 bytes) across 3 samples | Not measured separately | Binary size re-measured separately: 17.92 MB plain (17,916,928 bytes) / 12.32 MB stripped (12,319,744 bytes). Stripped binary remains under the <30 MB size budget; RSS exceeds the original <40 MB idle target and is recorded as the Phase 11B SQLite waiver in `docs/goal-scorecard.md`. |
| MagicHandy Go core idle/library reads, Phase 14 | Phase 14 review working tree | plain and `-ldflags "-s -w"` builds under `%TEMP%\MagicHandy-phase14-budget`; fresh stripped binary with isolated data dir; `/healthz` for idle, then five `GET /api/library` reads | No browser window; HTTP API exercised by `Invoke-WebRequest` | Yes; measured only the stripped MagicHandy PID | Idle after `/healthz`: 52.49 MiB (55,042,048 bytes) across 3 equal samples. After library reads: 52.99 MiB (55,562,240 bytes) across 3 equal samples | Not measured separately | Plain binary 18,464,256 bytes; stripped binary 12,766,208 bytes. Embedded JS is 250,740 bytes / 74,321 gzip; CSS 26,797 / 6,212 gzip; combined gzip 80,533 bytes (+8,174, +11.3% from Phase 13). No model or voice worker loaded. |
| MagicHandy Go core idle/model-manager reads | Managed llama.cpp runtime working tree | plain and `-ldflags "-s -w"` `CGO_ENABLED=0` builds; fresh stripped binary on `127.0.0.1:49735` with isolated data; `/healthz` for idle, then twenty `GET /api/llm/models` reads | No browser window for RSS; rendered UI measured separately | Yes; no Ollama/llama.cpp/voice worker included | Idle after `/healthz`: 52.73 MiB (55,296,000 bytes) across 3 equal samples. After model-manager reads: 53.40 MiB (55,992,320 bytes) across 3 equal samples | Not measured separately | Plain binary 18,822,656 bytes; stripped binary 13,031,936 bytes. Embedded JS is 266,268 bytes / 78,339 gzip; CSS 31,934 / 7,091 gzip; HTML 454 / 288 gzip; combined gzip 85,718 bytes (+5,185 / 6.4% from Phase 14). Cold starts measured 556/534/533 ms with the client-side PowerShell probe. No model bytes or runner process were loaded. |

| MagicHandy Go core idle, Phase 14B Intiface owner | Final Phase 14B working tree | plain and `-ldflags "-s -w"` `CGO_ENABLED=0` builds; fresh stripped binary on `127.0.0.1:49764` with isolated data; `/healthz`, two-second warmup, then three one-second samples | No browser window for RSS | Yes; no Intiface Central, device, model, or voice worker included | 53.20 MiB (55,779,328 bytes) across 3 equal samples | Not measured separately | Plain binary 19,205,632 bytes; stripped binary 13,309,952 bytes. Embedded JS is 270,506 bytes / 79,435 gzip; CSS 32,443 / 7,177 gzip; HTML 454 / 281 gzip; combined gzip 86,893 bytes (+1,175 / 1.4%). The pure-Go websocket owner and unconditional Stop hardening add 278,016 stripped bytes (+2.1%) over the managed-runtime baseline. |
| MagicHandy Go core idle, Phase 14C compacted connection manager | Final Phase 14C working tree | plain and `-ldflags "-s -w"` `CGO_ENABLED=0` builds under `%TEMP%\MagicHandy-connection-final`; fresh stripped binary on `127.0.0.1:49781` with isolated data; `/healthz`, two-second warmup, then three one-second samples | No browser window for RSS; rendered UI checked separately | Yes; no device, model, or voice worker included | 53.47 MiB (56,066,048 bytes) across 3 equal samples | Not measured separately | Final artwork/visualizer refinement: plain binary 19,675,648 bytes; stripped binary 13,779,968 bytes. JS 281,698 / 82,637 gzip; CSS 41,123 / 8,653 gzip; HTML 454 / 286 gzip; isolated PNG 444,236 / 437,427 gzip; total embedded browser payload 767,511 raw / 529,003 gzip. RSS is retained from the preceding Phase 14C sample because only embedded browser assets changed. |
| MagicHandy source-installer/compiler bootstrap | Source-installer follow-up working tree | plain and `-ldflags "-s -w"` `CGO_ENABLED=0` builds under `%TEMP%`; clean pinned llama.cpp CPU build in ignored scratch data, followed by manifest and `--version` probes | No browser window; no app process needed for script-only runtime change | Yes; the external CPU runner was built/probed, then scratch data was removed | Retained 53.47 MiB Phase 14C idle sample; the changed embedded script runs only during an explicit managed-runtime build | Not measured separately | Plain binary 19,677,696 bytes; stripped binary 13,782,016 bytes (+2,048 each). Browser payload is unchanged. CMake selected Visual Studio 18/MSVC 19.51 plus Windows SDK 10.0.28000.0; the clean build completed in 70.8 s and the runner reported pinned commit `c749cb0`. |

Core idle result: the pre-SQLite Go core idled at roughly **1/58th** of the
Python core (8.96 MB vs ~525 MB) on the same machine. After the Phase 11B
pure-Go SQLite dependency, the stripped Go core idles around **54 MB**, still
about **1/10th** of the Python core but above the original <40 MB idle target.

Still required (Phase 9B):

- none for the current real-device app-path gate. Cloud REST has the one-hour
  soak, and Browser Bluetooth now has a full short UI/chat hardware run with
  active RSS samples. A longer Browser Bluetooth soak can be scheduled later if
  BLE link endurance becomes a release criterion.

## Full App Path Evidence

- 2026-07-02 Cloud REST browser UI/chat path: launched the embedded UI in the
  in-app browser against a real Handy with the motion envelope capped at 10-35%.
  The visible connection-check button reported `Connected: HSP ready / 540 ms`;
  the visible Start motion button started `Stroke` at 23%; the SSE-driven
  visualizer reported `Running` with `Stroke - speed 23%`; the chat form sent
  `stop`, received the deterministic `Stopping motion.` reply, and the motion
  UI returned to `Idle`. An explicit Cloud stop cleanup succeeded afterward.
- 2026-07-02 Browser Bluetooth browser UI attempt: switched the visible UI to
  `browser_bluetooth`, saved the dispatch owner, observed the Bluetooth panel,
  `Browser: Available`, and an enabled Connect button. Automated browser clicks
  could not complete `requestDevice`; Chromium returned `Must be handling a user
  gesture to show a permission request.` Browser Bluetooth full motion/chat
  validation remains open until a human selects the device in the chooser.
- 2026-07-02 Browser Bluetooth Edge/Windows attempt with the device in
  Bluetooth mode: launched an isolated Edge profile against a local
  `browser_bluetooth` MagicHandy server with speed capped at 35%. Edge exposed
  Web Bluetooth and DevTools `DeviceAccess` on the page target, but the chooser
  event returned an empty device list. Additional `requestDevice` probes using
  the reported `OHD` device name and `OHD` prefix also returned empty device
  lists. After disabling DevTools chooser interception, a visible manual Edge
  page remained `Bluetooth disconnected` for four minutes. Windows PnP did not
  list an `OHD` Bluetooth device, and a Windows BLE advertisement watcher saw
  zero advertisements while running. The UI discovery filter was widened to
  include `OHD`/Handy name prefixes, but hardware app-path validation remains
  open until the OS/browser can see and select the device.
- 2026-07-02 Browser Bluetooth connected Edge follow-up: launched a local
  `browser_bluetooth` server with motion capped at 20-35%, opened it in the
  user's running Edge profile, and selected `OHD_hw0_29b3243120f4` in the real
  chooser. The bridge reported `connected=true`, `ready=true`, protocol
  `hsp_ble`, and `motion.available=true`. A non-moving Stop command ACKed via
  the browser bridge in 102 ms with no pending or inflight commands afterward.
  The first capped `23%` app-path start found a transport bug: motion engine
  stream ID `motion-000001` was rejected by the Browser Bluetooth BLE path,
  which required a numeric stream ID. That is fixed by mapping semantic stream
  IDs to numeric BLE stream IDs while retaining semantic diagnostics. Further
  retests found two UI/bridge recovery bugs, also fixed: command long-poll now
  survives backend restarts, and Bluetooth command consumers now use per-tab IDs
  so stale tabs cannot consume commands for the connected tab. The remaining
  blocker was live link stability: after reconnect, `hsp/state` timed out
  and/or the GATT server disconnected before the capped start sequence could
  complete. That failure led to treating the Browser Bluetooth connection check
  as bridge readiness instead of a state probe, and treating `hsp/play` as a
  write-ack command.
- 2026-07-02 Browser Bluetooth patched Edge validation: rebuilt the embedded
  app, reloaded the user's running Edge profile, reconnected the paired
  `OHD_hw0_29b3243120f4` device, and verified that the visible Check connection
  control reported `Connected: HSP ready / Unknown / 0 ms` with `/api/traces`
  still empty. A visible Start motion at 28% produced `stroke_window`,
  `hsp_add`, and `hsp_play` traces with `browser_ack`; `/api/motion/state`
  reported `running=true`, source `manual_ui`, pattern `stroke`, speed 28, and
  `last_result.kind=hsp_play`. The chat form then sent `stop`, received the
  deterministic `Stopping motion.` reply, returned the UI to `Idle`, and traced
  Stop as `browser_ack`. A repeat visible Start/Stop captured the three-sample
  active RSS range above. Logs were written under `.tmp-phase9b-manual/` for
  the local validation session; they are not committed because they are run
  artifacts.

## Phase 14B Intiface Rendered UI Evidence

- The production embedded build rendered Settings > Device with `intiface`
  selected at 1280×720 and 390×844. Both viewports had equal body/client and
  workspace/client widths, no horizontal overflow, and all server, connection,
  status, and Save controls remained reachable above the fixed Stop bar.
- The first desktop pass exposed a `devices: null` startup crash before any
  Intiface session existed. The backend now emits a non-nil empty device list
  and the React panel also tolerates older null snapshots; a regression test
  covers the API shape.
- Unsaved owner/address edits visibly disable Connect. After Save, Connect is
  enabled; with no listener on port 12345 the app remains disconnected and
  reports an actionable availability error without enabling motion.

## Phase 14B Intiface Hardware Evidence

- On 2026-07-12, Intiface Central at `127.0.0.1:12345` exposed
  `The Handy (FW4+)` as one 100-step position actuator. An isolated Phase 14B
  binary selected it without using a Handy connection key.
- Settings capped the run to 10–20% speed. The shared stroke pattern ran at
  20% in a 20–80% window for two seconds, paused with a successful
  `StopDeviceCmd`, resumed with phase preserved, then applied a 30–70% reverse
  refresh while running before a successful final Stop.
- The exported `motion_trace.v2` envelope had 19 rows. All transport results
  were successful, command kinds were neutral `points_add`/`points_play`, no
  starvation occurred, final playback was idle, and queue depth was zero.
  Local command latency rounded to 0 ms at the current millisecond resolution.
- A rebuilt final binary then ran the pattern at 20% for one second. Active Stop
  `intiface-000005` and repeated idle Stop `intiface-000006` were distinct,
  successful commands. Disconnect recorded another successful close-time Stop.
- The same Handy then ran the matched workflow over Cloud REST: 20% cap,
  20–80% initial window, Pause/Resume, live 30–70% reverse refresh, active Stop,
  and repeated-idle Stop. The 23-row trace contained 19 successful transport
  results and no starvation. Pause, active Stop, and idle Stop latency measured
  317, 311, and 310 ms; a non-moving HSP check and preflight Stop succeeded at
  676 and 105 ms.
- The test process was stopped and runtime trace/log files stayed under `%TEMP%`;
  no generated evidence or credential entered the repository. No non-Handy
  linear device was available. Subjective matched feel remains open.

## Phase 14C Floating Connection Manager Evidence

- The embedded production build was rendered at 1280×800 and 390×844. The
  trigger sits at the far right of the 48px top bar and the 360×646px panel
  opens at y=56 with no document, panel, or internal overflow; all four
  immediate limits remain visible.
- At 390×844 the complete panel ends at y=702 while the always-mounted Stop bar
  begins at y=748, preserving a 46px safety gap and clear bottom navigation.
- The artwork uses one 444,236-byte transparent hand isolation generated from
  the reviewed reference. It renders directly at a fixed square source ratio,
  with no SVG mask, filter, or clip. The scaled frame contains the hand, exactly
  three signal paths in the lower half, and the reference's tall capsule,
  shorter domed body, LED, and square marker. Disconnected hides both signal
  and X while keeping the square red; connected makes the square green; only a
  failed connection attempt shows the two-stroke red X, which shakes once for
  420 ms. Reduced-motion leaves the X static.
- React tests cover route persistence, provider-scoped Cloud/Bluetooth/Intiface
  controls, the scoped redacted connection-key save, active API v3 ID source,
  immediate semantic limit updates, focus restoration, three signal paths, and
  connected/disconnected/error artwork states. The one shared motion visualizer
  now renders a Handy chassis, physical rail, configured stroke envelope, and
  backend-positioned carriage in both top-bar and detailed forms; the detailed
  390px rendering remained inside its 314.4px container with all telemetry
  visible. Reduced-motion disables both position transitions and connection
  animations in CSS.

## Phase 14 Rendered UI Evidence

- A temporary isolated-data server exercised the real embedded React build with
  fake transport at 1280 px and 390×844. Browse enablement/weights, Program
  import/player layout, freehand Author drawing/edit/save, backend preview, and
  Training rate/undo/auto-disable interactions were all operated in the DOM.
- Freehand drawing produced 13 source points, 14 saved knots including closure,
  and 246 backend preview samples. The rendered curve was nonblank and the saved
  pattern appeared in Browse/Training.
- Training changed a weight by +0.15 and exact undo restored 1.00 with the ledger
  visibly marked undone. Auto-disable was switched on and back off.
- The first 390 px pass found that `.library-shell` could flex-shrink inside the
  route and clip the Training preference column. Making the route panel content-
  sized fixed it; all tabs then had no horizontal overflow, their final controls
  were reachable above the fixed footer, and the browser console was clean.
- This pass proves UI behavior and backend-sampler agreement over fake transport.
  It does not replace the capped real-device feel check for the generated
  routine floor or imported content.
- 2026-07-11 hardware follow-up: the final embedded build was opened in the
  user's Edge window with an isolated schema-v8 data directory, Browser
  Bluetooth selected, and `speed_max_percent` capped at 35. Edge's chooser
  scanned for five seconds but reported no compatible device advertisement, so
  it was cancelled and **no motion command was sent**. The same build's Browse
  view rendered cleanly at 1027×702; accessibility exposed a named tab list,
  four selectable tabs, a Browse tab panel, labeled toggles, and backend-sampled
  curve graphics. Real-device Phase 14 feel evidence remains open until the
  device advertises again.

## Model Manager Rendered UI Evidence

- The embedded production build rendered Settings > Model at 1280×800 and
  390×844. App-owned runtime version/backend/build controls, provider-scoped
  fields, managed model actions, and both import disclosures remained reachable
  with no horizontal overflow or browser console warnings.
- Desktop and mobile DOM checks exposed labeled Provider/Model controls,
  controller-gated Load/Unload and runtime build/cancel actions, an accessible
  Import from Ollama button, path input, scan action, filter, compatibility
  rows, and stable model actions. Managed mode exposes no executable or model
  path fields. Pending settings now replace stale provider health with an honest
  save-to-check state.
- A real Windows Ollama library at the platform default path contained 16
  manifests. The bounded filesystem scanner marked all 16 compatible, while
  the live daemon `/api/tags` list independently reported 16 models. Selecting
  and saving one of those daemon models reported ready without starting a model
  copy or managed llama.cpp process.
- The pinned helper performed a clean Windows/amd64 CPU source build in 54.2
  seconds, installed 18,432,916 bytes of runner/DLL/license/manifest files, and
  reported commit `c749cb0`. A second helper run and an in-app build request
  reused the validated install without changing its original build timestamp.
- Fixture-backed backend/API tests cover the destructive side of the flow:
  manifest/config/license parsing, unsupported projector rejection, atomic
  SHA-256-verified copy, deduplication, selected-model deletion protection,
  standalone GGUF import, concurrency limits, and controller enforcement.

## LLM Latency Controls (Unmeasured Runtime Delta)

- Source inspection found no output-token cap in the mainline provider request,
  implicit provider/model reasoning behavior, and redundant managed llama.cpp
  health/model-list probes before every warm initial and repair call.
- Requests now default to 256 maximum output tokens, expose reviewed
  128/256/512/1024 choices, and map explicit `auto`/`off` reasoning behavior to
  llama.cpp and Ollama native fields. A zero-temperature repair is now sent as
  zero rather than omitted.
- Managed llama.cpp still performs readiness/model checks during cold load and
  explicit status actions, but reuses successful readiness for warm calls.
- Go request-shape tests prove exact provider payloads; frontend tests prove the
  controls, tradeoff warnings, and persisted values.
- A 2026-07-13 regression probe reused the exact latest failed conversation and
  prompt against the loaded managed llama.cpp `b9966` CUDA runtime with a Gemma
  4 12B Q4 model. Automatic reasoning with `max_tokens=256` ended `length` after
  4.14 seconds with 897 reasoning characters and zero visible content; raising
  the cap to 512 repeated the failure after 6.33 seconds with 1,943 reasoning
  characters. `enable_thinking=false` at 256 produced valid 112-character JSON
  in 1.05 seconds. Keeping automatic reasoning but setting
  `thinking_budget_tokens=128` produced valid 112-character JSON in 2.35
  seconds. These are diagnostic single-request observations, not a general
  model-speed benchmark.
- The resulting policy defaults reasoning off, bounds the current pinned managed
  automatic path to half the selected total budget, recognizes provider length
  finishes, and makes repair retain context while requesting reasoning off. Broader
  malformed/repair-rate and quality A/B runs across fixed small models remain
  required before claiming a general speedup.

## Procedure

For Windows local measurements:

```powershell
$env:CGO_ENABLED = "0"
go build -o $env:TEMP\magichandy.exe ./cmd/magichandy
$proc = Start-Process -FilePath $env:TEMP\magichandy.exe -ArgumentList "-addr", "127.0.0.1:49718" -PassThru -WindowStyle Hidden
Start-Sleep -Seconds 2
1..3 | ForEach-Object {
  Get-Process -Id $proc.Id | Select-Object Id, ProcessName, WorkingSet64
  Start-Sleep -Seconds 1
}
Stop-Process -Id $proc.Id
```
