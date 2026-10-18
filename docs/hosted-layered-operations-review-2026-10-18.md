# Hosted Layered operation review — October 18, 2026

## Failure and change

The supplied October 16 diagnostic report came from alpha.63, `5e02daa0`, with
ChatGPT `gpt-6-sol` and Layered motion. A completed, single-call response
supplied both absolute `controls` and relative `change_by` for six of the same
fields. The domain validator correctly rejected it with `cannot set and adjust
the same control: min_percent`. The schema had independently allowed both
groups. Zero deltas were not the whole problem: `anchor_percent` also contained
an absolute zero and a relative -10. Dropping selected fields would guess intent.

Hosted models now choose one operation per control, for example
`"speed_percent":{"op":"adjust","value":-5}`. Each operation's schema has
disjoint `set`/`adjust` branches with the existing bounds. Layer periods use the
same representation to avoid a second absolute/relative conflict. The existing
OpenAI strict-schema adapter makes optional fields nullable and restores them
to omission, then the chat layer translates explicit operations into the
unchanged domain edit. Zero values remain real values. Ambiguity, duplicate
fields, unknown operations, malformed values and out-of-range edits reject
atomically. No keyword matching, response repair or additional provider call
was added.

The provider capability applies consistently to interactive chat, Autopilot
and hosted second-opinion corrections. The latter uses the reviewer's provider
contract while the immediate local path retains its original contract. Local
Ollama/llama.cpp, Labs and Creative v2 keep their original contracts. Only
code-owned contract instructions/examples are translated; user messages,
personas and conversation history are not rewritten. Saved limits, recall,
lifecycle authority, the shared compiler, transport dispatch and Stop remain
the existing authorities.

## Validation

- Regression cases compare complete resulting scores with equivalent legacy
  domain edits, including absolute zero, relative changes, layer periods,
  geometry, hold and empty arrays. Rejections leave the original score intact.
- Tests cover recall, stopped/paused/running authority, Autopilot timing,
  prompt-inspector parity, state-specific examples and reviewer corrections.
- Real Responses adapter tests cover both ChatGPT plan usage and OpenAI API
  mode, including wrapped action unions and strict nullable fields. They
  produce the expected domain score in one call without repair/fallback.
- Independent JSON Schema validation accepts four valid set/adjust examples
  and rejects the archived overlap, competing operations and unknown operation
  names in both the domain and strict wire schemas.
- `go test ./...`, `go test -race -timeout 20m ./...`, `go vet ./...`,
  `golangci-lint run ./...` and the `CGO_ENABLED=0` build passed. Frontend
  typecheck, all 913 tests and build passed; committed dist was unchanged.
- Ten captured local Layered/Creative v2 state fixtures have identical prompt
  and schema hashes before and after. There is no new call on the local path.

## Live ChatGPT and remaining limitations

The opt-in `TestHostedLayeredLiveOperations` uses the existing protected host
sign-in with `gpt-6-sol`, reasoning off, synthetic technical requests, and no
engine or transport. Credentials are never copied into a review profile.

Three retained reports contain seventeen generations: twelve interactive
cases, two reviewer corrections and three Autopilot probes. All seventeen
parsed with the new operations. All twelve interactive intent checks and both
reviewer corrections passed. Cases cover absolute zero, relative pace,
relative timing, alternating ends, explanation without motion, recall plus an
adjustment, and correcting a missed speed reduction. Interactive latency was
2.789–6.240 s, median 3.728 s; this small sample is not a provider benchmark.

Only one of the three retained Autopilot intent checks passed. Two valid
proposals raised a requested fixed 25% pace to 30% and 28%. One failure used
minimal history; the other included the recent human message, acknowledgement
and age. The isolated history-inclusive probe passed, so adding history is not
established as a fix. These failures remain in the reports, visual atlas and
opt-in test result. A preliminary smoke run also rejected an unrelated geometry
conflict; its console log survives, but a relative output-path mistake lost its
raw report, so it is not included in the seventeen fully captured generations.

This change fixes the structural set/adjust contradiction. It does not claim
that every schema-valid edit follows the user's intent, or that geometry
conflicts and autonomous pace drift are solved. Those require a separate
evaluation of planning instructions and constraint representation. Existing
semantic checks remain intact; they are not relaxed to make the probes pass.
No real device connection, physical motion or stop test was needed for this
contract change. The charts describe commanded estimates, not carriage feedback.

## Shared-engine visual review

All seventeen retained generations plus a neutral reproduction of the archived
rejection are in an eighteen-entry atlas. Identical outputs share images; ten
distinct plots and the overview were generated with the shared compiler. The
overview and detailed alternating-end and failed-Autopilot plots were inspected.
There are no added seam discontinuities in those detailed outputs; their
whole-percent samples follow the planned trajectories. The alternating-end
selection retains its two end regions and variable short strokes. The failed
Autopilot selections compile smoothly, but their 30%/28% pace remains an intent
failure, not a visual success. Explanation-only turns plot the unchanged
already-running score and issue no command.

Review artifacts are ignored under `.scratch/layered-contract-review/`:
`live-chatgpt-final.json`, `live-chatgpt-autopilot-history.json`,
`live-chatgpt-complete.json`, `reported-rejection.json`, `atlas-all.json`,
`atlas-all/index.html`, `schema-check.json`, test logs and local hash captures.
The original diagnostic archive and user conversation are not committed.

To repeat the hosted evaluation, set `MAGICHANDY_HOSTED_DATA_DIR` to an existing
signed-in profile, `MAGICHANDY_LIVE_MODEL=gpt-6-sol`, and
`MAGICHANDY_EXPERIMENT_CAPTURE` to an **absolute**, ignored output path. Run
`go test -tags liveeval ./internal/chat -run '^TestHostedLayeredLiveOperations$'
-count=1 -v`. It consumes plan usage and intentionally fails if intent does not
match; do not hide failed cases. Export the retained reports with the documented
`motion-atlas -catalog=false -llm ...` workflow.

The current review app runs in a fresh isolated simulator profile with Layered
selected and an available local Gemma model. `scripts/check-review-llm.ps1` and
one text-only app chat both completed; the chat made one provider call without
repair, fallback or motion. This readiness check is separate from the real
ChatGPT contract evaluation above.
