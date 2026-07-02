# Performance Baseline

This file records the baseline evidence required by
`docs/goals-and-guardrails.md`. Measurements must exclude browser, test runner,
Ollama, llama.cpp, CUDA, TTS, ASR, and other worker/model processes from the Go
core number.

## Environment

- Date: 2026-06-30 (Go idle), 2026-07-01 (Python baseline),
  2026-07-02 (Go active Cloud REST short run and one-hour soak)
- OS and architecture: Windows / amd64
- Go toolchain: Go 1.26.3
- Python runtime: CPython 3.11 in the StrokeGPT-ReVibed `.venv`

## Measurements

| App | Commit | Command | Browser UI Opened | Child Worker/Model Processes Excluded | Steady RSS After Warmup | Peak RSS | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- |
| StrokeGPT-ReVibed core idle | `6c56985` (2026-06-29) | `.venv\Scripts\python.exe app.py` with `STROKEGPT_OPEN_BROWSER=0`, `STROKEGPT_PORT=5017` | No, `Invoke-WebRequest` loaded `/` once (HTTP 200) | Yes — the 4.3 MB `.venv` shim process was excluded; the spawned real interpreter was measured; no LLM/TTS/ASR model loaded, no motion, no voice | 524.75-524.81 MB across 3 samples | Not measured separately | The idle number includes ML libraries imported at startup (Torch et al.), which is precisely the core-install-path overhead the rewrite removes. |
| MagicHandy Go core idle | pending Phase 1 working tree | `CGO_ENABLED=0 go build -o $env:TEMP\magichandy-phase1.exe ./cmd/magichandy`, then run built binary with `-addr 127.0.0.1:49718` | No, `Invoke-WebRequest` loaded `/` once | Yes | 8.96 MB (9,392,128 bytes) across 3 samples | Not measured separately | `/healthz` returned `ok`; `/` returned HTTP 200. |
| MagicHandy Go core active, Cloud REST short run | Phase 9B controller/SSE working tree | temp `CGO_ENABLED=0` binary, Cloud REST configured with real Handy, `POST /api/motion/start` at 25%, `GET /api/motion/events` held open, deterministic chat `stop` | No browser window; HTTP API exercised the app endpoints and SSE stream | Yes; no Ollama/llama.cpp/voice worker loaded | 16.75-16.76 MB (17,563,648-17,571,840 bytes) across 3 samples | Not measured separately | Real Cloud REST check returned OK/HSP available; motion SSE showed running `stroke` at 25%; chat `stop` returned `Stopping motion.` and cleanup Stop was sent. This is a short safety run, not the one-hour soak. |
| MagicHandy Go core active, Cloud REST one-hour soak | Phase 9B soak-evidence working tree | temp `CGO_ENABLED=0` binary under `.tmp-phase9b-soak`, Cloud REST configured with real Handy, `POST /api/motion/start` at 25%, `GET /api/motion/events` held open, one sample per minute, deterministic chat `stop` cleanup | No browser window; HTTP API exercised the app endpoints and SSE stream | Yes; measured only the `magichandy` PID, excluding the PowerShell supervisor/SSE reader and direct-stop cleanup helper | 18.41-20.16 MB (19,300,352-21,139,456 bytes) across 56 warmed samples from 302s through 3600s | 20.16 MB (21,139,456 bytes) | 61 total samples from 2s through 3600s; all samples reported `running=true` at 25%; SSE log recorded 28,800 lines with 14,392 running events; warmed RSS range grew 9.53%, within the +20% Phase 9B gate; chat `stop`, motion stop, Cloud stop, and direct cleanup Stop all completed. |

Core idle result: the Go core idles at roughly **1/58th** of the Python core
(8.96 MB vs ~525 MB) on the same machine.

Still required (Phase 9B):

- full Browser Bluetooth hardware validation with UI/chat path. The blocker has
  moved past discovery: Edge can now select `OHD_hw0_29b3243120f4`, the browser
  bridge can become ready, and a non-moving Stop command ACKed over Bluetooth in
  102 ms. Full motion/chat validation is still open because the live GATT link
  dropped or reported `hsp/state` timeout before a capped app-path start could
  complete. No successful Browser Bluetooth motion command has been recorded.

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
  blocker is live link stability: after reconnect, `hsp/state` timed out and/or
  the GATT server disconnected before the capped start sequence could complete.
  Logs were written under `.tmp-phase9b-manual/` for the local validation
  session; they are not committed because they are run artifacts.

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
