# Startup recovery review — 2026-10-20

Claude Opus 5.5 (`claude-opus-5-5`) reviewed the complete first-pass startup
implementation through Claude Code CLI at **high** effort. The review was
read-only and covered the new startup package, command startup, API admission,
Diagnostics UI, source launcher and related installer tools. A focused follow-up
uses the same model at **medium** effort. The author reproduced and adjudicated
findings before changing code.

The focused follow-up confirmed the fixes and reported no remaining High or
Medium issues. Its Low finding (**F1**) concerned callers retrying an already
running, unverified restart. Startup timeout now emits a distinct
`MagicHandy.StartedUnverified` error only while its child is still alive.
Language/TTS callers preserve that process and report a warning; language
recovery also avoids retrying while a checkout process is alive. A control-flow
regression checks that an unverified restart produces one warning and one
launch attempt. The simulator fixture waits for the expected PID before testing
launcher reuse, removing the follow-up's noted test-startup race.

## Findings and decisions

- **H1, saved network deployments:** a source launcher's local `auto` request
  could prevent a saved HTTPS/proxy policy from starting. Saved hosted policies
  now stay fixed when there are no explicit network/TLS overrides. Explicit
  incompatible flags still fail. Regression cases cover both saved hosted modes
  and an explicit proxy override. Hosted readiness is outside the local source
  launcher's discovery protocol; timeout leaves the process alive and explains
  use of the configured public URL. The previous HTTP-only launcher could not
  verify these deployments either, and killed its process on timeout.
- **M1, simulation:** duplicate startup could silently ignore
  `-simulate-motion`. That request is now rejected before reuse, with guidance
  to close the existing instance or use an isolated profile. The failing
  regression was reproduced before the fix. No live transport can be silently
  substituted for requested simulation.
- **M2, installer consumers:** language and TTS changes now pass the running
  profile to Stop; language changes also discover the actual port before their
  initial state query. Install/provision passes the **previous** data directory
  when reconfiguration selects a new profile. The integration suite changes
  languages against an actual recovered simulator and confirms the preferred
  port remains unchanged.
- **L1, transient Windows replacement failure:** private atomic writes now retry
  sharing/access denial at most five times over 150 ms. Other errors fail
  immediately. Injected tests cover recovery and the retry bound. Persistent
  publication failure still fails startup rather than advertising readiness
  without the required instance record.
- **L3, locked/hosted startup guidance:** nonlocal readiness records fail local
  discovery promptly with guidance to use the configured URL. The launcher only
  attributes a failure report to its launch if the report is fresh; duplicate
  failures no longer point to missing or stale retained failure reports. Timeout
  identifies the process left running and the startup report directory.
- **L4, older executables:** discovery suppresses unsupported-flag stderr, and
  fixed source launches rely on their already-strict explicit `-addr`, avoiding
  a needless new flag. Automatic discovery/recovery still requires rebuilding
  older executables; launcher guidance names `update.ps1 -NoPull`.
- **M3, insecure or unsupported coordination storage:** the suggested
  permission downgrade was not adopted. Instance verification depends on a
  private token; the app requires a directory it can secure, explains use of a
  user-owned local data directory, and retains a credential-free failure report
  in private temporary storage. A separate coordination fallback for unsupported
  filesystems is deferred rather than silently weakening this boundary.
- **L2, recovery classification:** retained intentionally. A failed cached bind
  followed by automatic success is a recovery event even when the preference
  becomes available again. The report records the latest event and its attempts.
  Tests cover hint → preference and hint → preference → OS-assigned retry order.
- **L5, setup route:** intentional and documented. `-Setup` opens
  `#/setup/reconfigure`, matching the executable's explicit setup behavior.

The author's review also caught a fixed-port reuse edge case: only the remote
listener could have recovered while the main port stayed preferred. Instance
metadata now retains the preferred remote URL. Both executable and launcher
reject fixed reuse of a moved or unverifiable remote endpoint. A real remote
port conflict regression exercises the executable; launcher policy has a
corresponding assertion.

No HNS reset, firewall change, elevation, process-ownership relaxation, new
dependency, motion path or frontend asset was added by these review fixes.

## Validation

The startup/cmd regressions, full Go and Windows race tests, vet/lint, and
installer suites under both Windows PowerShell 5.1 and PowerShell 7 pass.
The current-source simulator/LLM handoff accompanies the budget sample in the
[goal scorecard](goal-scorecard.md). The original frontend's 836 tests, localization audit,
typecheck and canonical embedded build passed; the review fixes do not change
frontend source or its embedded assets.
