# llama.cpp conversation and Autopilot review

Compared merged Creative v2 (`025c68f8`) with production prompt, history and
Autopilot changes in ordinary full Go builds. Models run on the installed CUDA
llama.cpp b9966 (`c749cb0`), not Ollama. Motion uses `-simulate-motion`, the
normal scheduler and shared engine. No hardware was connected by these tests.
Commanded plots and simulated dispatch cannot establish physical feel.

## Retained experiments and integration

The [ideas review](motion-ideas-review-2026-09-04.md),
[conversation review](lab-conversation-review-2026-09-05.md),
[Layered review](layered-motion-review-2026-09-05.md),
[Creative v2 review](creative-v2-motion-review-2026-09-05.md) and retained local
prompt patches were checked against production.

A read-only comparison of the existing Creative v2 review app's nine saved Lab
prompts against the merged baseline found no additional custom prompt changes
to recover. Hashes are retained in `.scratch/llama-saved-prompt-audit.json`.

| Idea | Disposition |
| --- | --- |
| Direct language and anatomy boundaries | Already integrated in identity/final voice checks; preserved. |
| Named geometry, relative edits, atomic groups | Already integrated; retained instead of reverting to long flat parameter lists. |
| Compound examples | Clarified that geometry can be paired with speed, and fixed width plus drifting location requires both edits. |
| More explanatory Layered v8 guidance | Previously degraded both models; not restored. |
| Ground speech in accepted motion | Added refreshed semantic facts before existing speech generation, without an extra inference pass. |
| Session progress and stochastic sampling | Existing Autopilot controls were being discarded by the continuous adapter; now forwarded. |

## Confirmed defects

- Continuous assistant history was wrapped in legacy `motion.action` JSON,
  and its final guard contradicted the required `edits` form. Under llama.cpp,
  Creative v2 attempted evolution during ordinary conversation. History and
  final guards now use the selected mode's empty edit form, preserve speech,
  and exclude historical actions. The selected voice remains authoritative.
- Two independent 12-message limits ended context after six exchanges. Both
  now share a 64-message limit and a 24,000-byte history budget. This is bounded
  session context, not permanent memory or an automatic summary. Verbose or
  longer sessions can still lose details. Existing 200-message storage retention
  is unchanged.
- Empty Autopilot sessions were always told to refresh only the seed. They can
  now author compatible controls inside saved limits. Human-requested character
  and exact repetition retain preservation rules. Ordinary questions cannot
  crowd out retained motion directions. No fixed rotation or novelty quota was
  introduced.
- Speech called Flow IDs catalog patterns and omitted Dynamic geometry. It now
  gets mode-specific facts even with chat-only authority. Planning derivatives
  stay in planning; controls do not imply instantaneous phase or sensed feedback.
- The running continuous permission matcher missed “increase speed by …”.
  Direct increase/decrease/reduce requests now work. Questions, negatives,
  pauses and stopped motion retain their restrictions.

## Full-build observations

Installed models: Gemma 4 12B Q4_0 and Granite 4.1 3B, imported into disposable
managed inventory. Comparison builds share the same loaded llama.cpp worker
through a loopback recorder and run sequentially. Raw selections, including
rejected actions, are retained in ignored reports. Setup failures are separate.

| Probe | Baseline | Candidate observed |
| --- | --- | --- |
| Gemma Creative v2, 24 conversation turns | 2 accepted; 22 unauthorized evolution attempts rejected | 24 accepted, no repair/fallback or motion |
| Gemma, four delayed recall probes | Original Creative: 2/4; invented a dinner and wine | Creative v2: 4/4 lexical checks; manual review still found a small key-location error |
| Gemma, 16 production control turns | 13 strict intent passes | 14 strict intent passes |
| Granite, 16 production control turns | 8 strict intent passes | 7; a failed Layered start caused subsequent edits to be rejected while stopped |
| Granite, 24 conversation turns | 21 accepted, 0/4 recall checks | 24 accepted, 4/4 recall checks |
| Direct vocabulary, three modes × four turns | Gemma 12/12; Granite 10/12 | Gemma 12/12; Granite 10/12 |

The language probe asks for neutral classification/reproduction of direct
terms, with no scene or act. It checks availability, JSON validity, lack of
masking and contextual recall. It does not establish erotic prose quality.
Explicit's production instructions remain present and tested in both formats;
Warm conversation did not acquire explicit terms.

These are strict probes, not perfect mapping claims. A lower-end focus of 10%
fails the fixture's exact-0 condition despite being a plausible regional choice.
Layered still sometimes claims 30% while omitting its pace edit. Granite remains
weak on compound geometry in the complete production prompt. No Granite-specific
prompt replaced Gemma's prompt, and no unproven compact preset was added.
Single stochastic samples have limited statistical strength.

Non-English and negative directions are treated conservatively rather than
allowing missed English keywords to unlock broader autonomy. This can preserve
more of a score than intended for ambiguous conversation.

## Autonomous output and visual review

Baseline: 85 seconds per mode, no human requests. Both continuous modes used
one character at 25%, changing only its seed. Original Creative used one
geometry with speed 45–54%.

First candidate: 180 seconds per mode. Creative v2 authored 15 distinct
control configurations at 30–55%, with local/broad contrast, focus shifts, unequal direction
timing and rebounds. Layered still refreshed seeds because an unconditional
evolution example outweighed exploration. Narrowing it to continuation of a
requested character produced 10 distinct Layered configurations in 11 accepted
targets in a later 120-second run. Base speed stayed at 25%; layers varied.

Initial check-ins escaped into floors, car cabins and scenery. The first
revision removed these settings but repeated generic steady/smooth claims.
Later revisions used specific features, removed planning-only peak telemetry,
and separated Creative v2 and Layered vocabulary. Models can still misdescribe
phase or imply an action in a chat-only line; that grants no motion authority.

The first atlas has 67 records, 57 distinct plots and four overview sheets.
All four were inspected. Baseline broad/tip groups and Layered envelopes recur;
candidate Creative v2 shows distinct regions, rebound clusters and broad
returns. Small scalar changes in Original Creative remain less distinct.
Finite curves remain replayable even with stochastic authoring.

Some short-stroke Layered selections are strongly slowed by reversal-spacing
limits. Requested speed alone therefore overstates actual travel. These remain
in the atlas; sampler, sanitizer, velocity/acceleration/jerk limits and transport
path were not relaxed. Final captures, raw provider records, traces and further
atlas findings are retained under `.scratch/llama-*`, uncommitted and unembedded.

The subsequent atlas contains 168 records, 123 distinct steady plots and six
captured dispatch timelines. All eight overview sheets were inspected. Distinct
configurations exclude seed and base speed, but are not a perceptual novelty
guarantee. In the 90-second final runs, Gemma used seven Creative v2 and seven
Layered configurations; Granite used eight and four, respectively. Gemma's
Layered base speed stayed at 25%; Granite progressed from 25 to 45%. Original
Creative remained less exploratory, especially with Granite.

The lowest effective-pace outlier requested 25% but compiled to about 13.67
percentage points of travel per second (0.47% effective calibration), limited
by reversal spacing. The highest jerk was about 149,111, below the gesture
limit of 150,000, with an acceleration seam jump about 0.000172. These expose
real limitations of small local selections rather than grounds to raise limits.

A final speech-only context revision describes active semantics in plain
language, omits inactive focus/width/rebound values, and removes stale speed
trends and planning derivatives. Follow-up 65-second runs per mode on both
models were closer to the active phrase: Gemma described broad/local work,
regions, directions and active layers; Granite stopped claiming full-slider
travel in its tested Dynamic run. Language remains repetitive in places and
is not protected by a complete semantic truth validator.

The final speech-run atlas adds 26 records, 23 distinct plots and two inspected
overview sheets. Creative v2 alternates broad returns and regional groups;
Layered shows drifting envelopes and asymmetric local clusters. Some proposals
remain nearly periodic, so additional control values alone do not establish
naturalness. Detailed review of the slowest and highest-jerk outliers shows
smooth planned joins but substantial wire quantization and unequal directional
velocity at high contrast. All six reviewed dispatch timelines were inspected:
they show successive accepted changes and the canceled queue at Stop, using
trace-event timestamps rather than physical position feedback.

Validation includes full Go tests, race tests, vet, lint, architecture and
`CGO_ENABLED=0` builds, plus frontend typecheck, 473 tests and the canonical UI
build. The same engine, Stop and transport ownership gates remain in force.

## Reproduction and update preparation

Use disposable data and a loaded local model. Scripts refuse control unless
the app reports `motion_simulated`; they create test sessions and change that
disposable app's settings. Read replies and curves alongside numerical checks.

For raw autonomous responses, the optional
`scripts/record-llama-evaluation.py --upstream-file .scratch/llama-upstream.json
--output .scratch/provider.jsonl` recorder can sit between a comparison app
and a separately owned local worker. The JSON file contains only `base_url`.
Keep the comparison app in external-provider mode; do not switch the app that
owns the worker away from managed mode while comparing. The recorder accepts
only loopback model endpoints and was smoke-tested with real text generation.

```powershell
python scripts/evaluate-app-conversation.py --base-url http://127.0.0.1:49853 --output .scratch/conversation.json
python scripts/evaluate-app-controls.py --base-url http://127.0.0.1:49853 --output .scratch/controls.json
python scripts/evaluate-app-controls.py --suite language --modes creative_v2,layered,dynamic --base-url http://127.0.0.1:49853 --output .scratch/language.json
python scripts/evaluate-app-autopilot.py --base-url http://127.0.0.1:49853 --seconds 180 --output .scratch/autopilot.json
go run -tags magichandy_labs ./cmd/motion-atlas -catalog=false -sessions .scratch/autopilot.json -llm .scratch/controls.json -output .scratch/atlas.json
python scripts/render-motion-atlas.py .scratch/atlas.json .scratch/atlas
```

Alpha.40 notes and package preparation accompany the review. Publication still
requires the normal reviewed PR, green gates, a merged main commit and explicit
release authorization.
