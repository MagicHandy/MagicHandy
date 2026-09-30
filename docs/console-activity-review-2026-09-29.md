# Terminal activity retention — 2026-09-29

## Diagnosis

The installed version on the review host is alpha.50. Its terminal and the
current development version both retained visible messages and hidden HTTP
request records in one 400-entry history. Filtering happened only when drawing.
Enough successful browser polls therefore evicted the startup messages, warnings
and other visible activity. Every remaining entry was hidden in the normal view,
so “Recent activity” became blank even though the logger was still receiving
records. The installed process was not running during this investigation; the
failure was reproduced with the same code in an isolated test.

The old handler also classified every HTTP request as details-only, including
failed requests and records explicitly logged at warning/error severity.

## Change

- Keep two bounded histories: the last 400 activity entries and the last 400
  records of the full stream. Successful background requests cannot evict
  activity. Details shows the full stream; switching back restores activity.
- Store them as rings. Adding to a full history overwrites one slot instead of
  allocating and copying a new 400-entry slice for each record. Strings are
  shared between histories; the histories do not grow over a long session.
- Display HTTP failures in the normal view: 4xx as warnings and 5xx as errors,
  preserving a higher explicit severity. The configured logging threshold still
  applies. Ordinary successful requests remain details-only.
- Display “No activity to display yet.” when the selected view has no entries.

The logging source, request fields, redaction and plain JSON output remain the
same. The change is confined to terminal retention, filtering and rendering.

## Evidence

The new tests reproduced the blank activity view, hidden failures and missing
empty-state explanation before the fix. They now pass:

- 1,200 successful polls preserve a startup message and warning across Details
  on/off toggles. The test checks the currently drawn frame, not stale bytes in
  the terminal's output history.
- HTTP 403/503 and explicit warning/error records stay visible at the expected
  severity. Both histories retain exactly their bounded, ordered suffixes after
  repeated wraparound. Full-history insertion performs zero allocations.
- The real app passed the Windows ConPTY harness with isolated data and simulated
  motion. After 450 HTTP health requests, D shows request details; D again
  repaints the startup activity. A subsequent 404 appears without Details. The
  same run checks the app link, S-to-Stop and Q/Q-to-quit behavior.
- Full Go tests, full Windows race tests, vet, default lint, lint with the
  `consoleharness` tag and the pure-Go build pass. No frontend source or embedded
  asset changed.

Reproduce the Windows integration test:

```powershell
go test -tags consoleharness ./cmd/magichandy -run '^TestLaunchConsoleInPseudoConsole$' -count=1 -v
```

Local logs and the raw ConPTY capture are retained under
`.scratch/console-activity-20260929/`, outside version control. The source-matching
review app runs at `http://127.0.0.1:50265/#/chat`, with fresh data, a simulated
device and LLM motion Off. Its configured Gemma worker passed the real-generation
review readiness script. The installed application and its data were not changed.
