# Chat model settings review — October 25, 2026

Decision: [ADR 0058, amended](decisions/0058-model-capability-levels.md#amendment-every-model-has-a-level-2026-10-25).
Base: `06f934da`.

## Findings

The owner found Settings > Chat > Model fragmented and too complicated, and
asked for the capability choice on external providers too, especially for LM
Studio and similar servers. The review used a fresh-data build of `06f934da`,
with a local Ollama model and Ollama's OpenAI-compatible endpoint standing in for
LM Studio:

- **Capability followed the transport.** Every external connection resolved to
  Advanced. A compatible connection on 127.0.0.1 serving Gemma 12B saved with
  independent vibration, Auto timing and response repair, the contract that model
  passes 4/20 times. Its budget started at 64K and its editor had no capability
  choice. Easy setup offers the same server under "API / Cloud".
- **Options moved with routing.** Reply length and the LLM motion mode lived in
  the local model's editor. They vanished when Chat and Autopilot used a
  provider, and reappeared nested under "Configure local retry model", together
  with the model store and three download cards, once local retry was on.
- **Two homes, or three.** LLM motion was on the Chat page and in Settings.
  Reply options spanned the Model tab, Prompts & memory and personas. Context had
  three controls in two formats ("65536 tokens", "64K — standard"), and
  "capability" named three different things.
- **Local-only clutter.** The model store and catalog showed while the runtime
  was Ollama. An empty second-opinion card preceded the provider list it needed.
  A draft raised three separate unsaved notices.
- **Defects.** Compatible and OpenRouter editors had an empty bordered row. The
  chat hint described the saved route beside a different draft choice.

## Change

- **A level for every model.** Connections carry `capability_level`, with the
  same Simple / Intermediate / Advanced floors as the local model (16K, 32K,
  64K). A new compatible server starts Simple; hosted providers and connections
  saved earlier keep Advanced. Features, prompts and history follow the level.
  The second opinion reviews any role below Advanced, whatever the transport.
- **Model roles.** Chat, Autopilot (with context sharing when relevant), Second
  opinion as a role row, response repair for Advanced chat, and the
  declined-request retry. Each role names its draft capability, and a changed
  chat route reads "Not saved yet." before the saved route.
- **Models.** One list: the local model first, then each provider, with status,
  capability and the roles that use it. Provider and llama.cpp mode merge into
  one Runtime choice. The model store is a section of the local editor, shown
  only for managed llama.cpp. Capability and Context are one component in one
  format everywhere; managed llama.cpp always shows its context, and a server
  MagicHandy does not run is asked to load at least the budget.
- **One home each.** Reply length joins Chat voice in Prompts & memory. The LLM
  motion mode and the Pattern library's Area focus and Experimental patterns
  live on the Chat page's motion card; the motion-mode endpoint accepts the two
  options and stops Autopilot only when the mode itself changes.
- **Smaller fixes.** The empty row is gone, "Provider capabilities and routing"
  is now "Request format and routing", and the vibrator notice says independent
  vibration needs an Advanced model, such as a hosted one.
- The Model tab has two cards instead of up to seven. 23 strings were added and
  30 retired in each of the five languages.

## Verification

Go: gofmt, vet, `golangci-lint` (0 issues), every package and the
`CGO_ENABLED=0` build pass. New tests cover per-connection resolution, budgets
and validation, routing features, the compact contract and 16K budget of a
Simple connection, reviewer eligibility and the motion card's options. The race
detector needs a C toolchain this host lacks, so `go test -race` runs in CI.
Web: 1,002 tests, the localization audit and the type check pass.

Review app (fresh data, simulated motion, 127.0.0.1:5131), desktop and 390 px
wide, with no horizontal overflow and Stop in view:

- the compatible connection saved by the earlier build showed Advanced, its
  legacy default. Choosing Simple at 16K saved `capability_level: "simple"` and
  `context_window: 16384`, and both roles' features reported Simple, with Auto
  timing, preparation, repair and independent vibration off;
- `check-review-llm.ps1` passed against that connection, serving Granite 4.1 8B
  through Ollama's OpenAI-compatible endpoint;
- a text-only chat in Layered mode replied "The sum of 2 and 5 is 7." in one
  provider call (648 ms, 9,956 of 11,633 prompt bytes), without repair, fallback
  or motion.

## Limits

- The thinking Gemma 4 12B build fails the readiness probe through Ollama's
  OpenAI-compatible endpoint at every level, as it did before this change: the
  adapter sends no reasoning control, so the server must switch reasoning off
  ([provider review](provider-settings-review-2026-10-09.md#local-compatible-endpoints-amendment)).
  Use it through the Ollama runtime instead.
- Simple and Intermediate on a connection reuse the local contracts and their
  evidence ([capability review](model-capability-review-2026-10-25.md)); over a
  connection they were checked with one text-only chat, not the motion suites.
- Connections saved earlier keep Advanced until changed, so an LM Studio
  connection made before this change still needs its level chosen.
- No hardware was connected.
