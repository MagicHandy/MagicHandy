# User-report issues 1–6: review and implementation

This work continues `codex/asynchronous-motion-review`; it is not merged into
`main`. TTS streaming and macOS installation from the report are outside scope.
The user's direction to avoid deterministic language fixes governs this change.

## Findings and changes

1. **Several Creative v2 control groups in one edit.** The alpha.60 base already
   supports complete combined groups and sends the standard `json_schema`
   response format. Keep that compatibility and its strict bounds. Regression
   tests and real generations exercise combined speed/variation and range/speed
   changes. No prompt-based workaround or parser-invented action is needed.
2. **Missing/invalid live action with thinking.** Experimental examples omitted
   an action while live chat required one. Live composition now gives state-valid
   action examples, preserving the established running-motion geometry examples.
   Paused examples cannot start motion. Live failures also exposed output-budget
   exhaustion: distinguish that from a malformed JSON action and show reasoning
   Off / a larger output budget as choices. Qwen 3.6 35B is not installed here;
   this is not a claim to have reproduced or fixed that exact model's report.
3. **Disappearing errors and model responses.** A collapsed Recent request failures
   view retains the latest 24 failures across chats. Metadata includes model,
   prompt set, reasoning, output budget and provider timing. Explicit inclusion
   reveals the redacted response, any repair, composed system prompt and schema.
   Saved-chat evidence survives reopening the database and never becomes model
   history. Unsaved chats retain their existing temporary lifetime; their records
   disappear when those sessions are cleaned up.
4. **Save versus activate.** The prompt editor retains its selection through save
   and navigation. Default prompt set, Used by the current chat, persona override,
   Use for [persona], and Choose as default make the assignment separate from
   editing. A chosen default still uses Save settings. Custom names are displayed
   in the selector. The report did not establish that saved prompt contents were
   actually lost; the reproducible selection/assignment ambiguity is addressed.
5. **Labs versus live prompt.** The inspector now includes the live action section
   and response schema, plus effective provider, prompt-set source and reasoning.
   Test active chat configuration is available there and in Labs. It uses the
   saved production context and never applies proposals or saves dialogue. This
   permits a useful comparison without pretending the Lab's experimental system
   prompt and a user-editable behavior profile have identical authority.
6. **Repetitive autonomous speech.** Remove contradictory pressure to produce a
   line on every timer. Ask the model what its next line contributes, let it choose
   an explicit empty reply, and honor the ordinary later cadence. No phrase or
   similarity rules suppress, rewrite, or retry model-authored language. A test
   deliberately repeats an earlier reply and verifies it is still published when
   the model chooses to say it.

## Live action evaluation

`internal/chat/report_issues_live_test.go`, `liveeval` build tag, runs on the
installed `igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0` through Ollama.
It retains every result, including incorrect intent and provider errors. No
hardware is connected. There is one provider request per interactive case.

The native-protocol comparison has 48 cases: old versus corrected live examples,
schema on/off, reasoning Off/Auto, and stopped/running/paused state. With reasoning
Off, both variants are valid in 16/16 and meet the exact requested intent in 15/16.
The range error keeps a 95% ceiling instead of the requested 100%; it is retained
as a failed intent case, not counted as success merely because JSON is valid.

| Reasoning Off | Baseline mean | Candidate mean |
|---|---:|---:|
| No response schema | 1,161 ms | 1,035 ms |
| Response schema | 919 ms | 1,011 ms |

These are small serial samples, not a controlled latency claim. Seeds differ
across motion cases. The change fixes a demonstrable prompt/contract contradiction;
it does not establish improved semantic selection accuracy on this batch.

With Auto and 1,536 output tokens, the baseline has 7/8 valid results and the
candidate 6/8. All invalid results exhausted the output budget before producing
visible JSON. They are not silently repaired into motion. Retained diagnostics
make this distinction visible. The compatible-provider follow-up uses the same
model through Ollama's OpenAI-compatible endpoint and the llama.cpp adapter:
8/8 valid and intent-correct with reasoning Off (982 ms mean), versus 2/4 with
Auto (13,953 ms mean). The other two again exhaust 1,536 output tokens before
visible JSON. Results are retained separately in `compatible-actions.json`.

## Speech evaluation and rejected approaches

The final comparison uses three 18-opportunity scenarios per model: an idle
session permitting brief updates, requested updates with an explicit pace change,
and an invited ordinary conversation with a new topic later. These are synthetic
non-explicit conversations, not the reporter's private chat history. Every
opportunity calls the same speech service; published speech alone enters history.

| Model/scenario | Baseline spoken | Final spoken | Baseline mean | Final mean |
|---|---:|---:|---:|---:|
| Gemma 12B, idle | 18/18 | 6/18 | 664 ms | 499 ms |
| Gemma 12B, requested updates | 18/18 | 2/18 | 624 ms | 459 ms |
| Gemma 12B, invited conversation | 18/18 | 18/18 | 677 ms | 679 ms |
| Vanessa, idle | 18/18 | 18/18 | 321 ms | 437 ms |
| Vanessa, requested updates | 18/18 | 18/18 | 302 ms | 318 ms |
| Vanessa, invited conversation | 18/18 | 18/18 | 342 ms | 335 ms |

All 108 final opportunities were valid and used one provider call, without repair.
Gemma acknowledged the pace change at opportunity 10, then let that exchange rest;
its invited conversation continued and answered the new topic. Residual generic
reassurance remains. Vanessa still paraphrases status repeatedly and sometimes
invents sensory details. The idle latency increase on Vanessa is recorded rather
than hidden by aggregate averages. Default Gemma interactive and speech paths
show no sustained regression in these samples; this is not a guarantee across
all local models or long sessions.

The candidate that merely added a quiet boolean kept speaking on every turn.
A stronger prompt then made Gemma quiet even during an invited conversation;
that candidate was rejected. Adding message/phrase ages also missed a requested
acknowledgment and was reverted. A trial wording-similarity filter was removed
following user direction. Only model-authored choices remain. The free-form
speech intent is never matched against a word such as "quiet": explicit empty
reply plus valid non-motion structure is the signal. This avoids repairing an
otherwise valid quiet answer just because its intent used different words.

The new quiet choice covers the independent speech contract (the default
Speech motion authority = None). Creative v2/Layered speech with motion authority
still uses its established continuous edits/reply contract and does not acquire
an incompatible quiet field. Motion-only Autopilot planning is unchanged.

## Reproduction and evidence

Set `MAGICHANDY_REPORT_ISSUES_MODEL` to the installed model name, and
`MAGICHANDY_REPORT_ISSUES_OUTPUT` to an ignored absolute output prefix. Run:

```powershell
go test -tags liveeval ./internal/chat -run '^TestReportIssuesLiveActionContract$' -count=1 -v
go test -tags liveeval ./internal/chat -run '^TestReportIssuesLiveCheckIns$' -count=1 -v
```

Speech accepts model names separated by `|`. `MAGICHANDY_REPORT_ISSUES_QUIET=true`
selects only the candidate speech pass. `MAGICHANDY_REPORT_ISSUES_COMPATIBLE=true`
selects the candidate action cases through the OpenAI-compatible local endpoint.
The historical `guarded` output label means candidate, not a runtime wording filter.

Local evidence is under `.scratch/report-issues/`: `gemma-actions.json`,
`compatible-actions.json`, baseline speech rows in `speech-balanced-checkins.json`,
final `speech-release-checkins.json`, and retained intermediate attempts. Early
speech attempts lacking the actual Flow context are not used as final evidence.
Some earlier attempts include now-rejected suppression decisions; final rows do not.

The atlas uses the shared engine, no transport, and all captured cases including
failures. Adapter copies under `atlas-inputs` rename provider request objects so
they do not collide with the exporter's request-text field; source captures stay
intact. Speech plots depict their supplied motion context, not speech-driven
movement. Likewise a stopped or paused chat case's preserved score is not proof
of playback. Invalid budget results retain cards without motion plots.

All overview sheets and detailed upper-range/directional outliers were
inspected. Planned and whole-percent wire curves align; the selected detailed
figures report zero velocity/acceleration knot jumps. The 50–95 selection is
visibly narrower than the requested 50–100, despite valid structure. Shared-engine
limits and Stop tests remain the safety evidence; no physical comfort or device
telemetry claim is made. The final atlas retains 816 cases, 58 distinct plots and
four overview sheets: `.scratch/report-issues/motion-atlas/index.html`.

## Validation and review app

Full Go tests, full race tests (plus a final changed-package race pass), go vet,
golangci-lint, architecture/lifecycle gates and the pure-Go build pass. Frontend:
112 files / 845 tests, typecheck, localization audit in five locales, production
build, and focused tests after the selector-label refinement. Bundle advisories
remain unchanged. Regression coverage includes bounded persistence, private
content opt-in, credential redaction, prompt/schema parity, explicit quiet choices,
normal cadence, no language filtering, and Stop-cancelable no-motion previews.

The isolated review app is `http://127.0.0.1:50224`. It uses the installed local
Gemma model, a separate data directory and simulated motion. No user credentials
were copied. Prompt save, assignment and navigation were exercised in the browser.
`scripts/check-review-llm.ps1` passed on that exact app. A text-only request through
the actual chat path returned "I am ready to assist you." in 589 ms, one provider
call, no repair/fallback, with motion Off. A subsequent browser preview in
Creative v2 returned action none and empty edits, reported "Motion was not applied",
and left the engine idle. The review tab is left at Diagnostics with the preview
expanded; the app process remains running.
