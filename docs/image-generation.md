# Image Generation — Analysis And Recommendations

Status: **proposal, nothing implemented.** This evaluates adding generated
images to chat, and recommends a shape. It is written to be argued with; the
decisions it reaches should become an ADR (next free number: 0014) before code.

## Summary Of Recommendations

1. **Treat images as a third worker role, not a new subsystem.** Slow, optional,
   GPU-heavy, out-of-core, and paired with one chat message is exactly the TTS
   problem, which this codebase has already solved once.
2. **ComfyUI first, behind a provider interface, driven by an app-owned pinned
   workflow template** — not by user-supplied workflow graphs.
3. **Do not ship a managed ComfyUI installer.** Start with "point at your
   existing ComfyUI", the same posture as "Use my existing Ollama". This is the
   entire answer to the bloat concern.
4. **The persona owns a structured base appearance as tags. The model never
   writes a diffusion prompt.** It contributes a short scene intent; the backend
   composes the real prompt. Reference images are a later, optional consistency
   layer on top — not the foundation.
5. **The model requests, the backend decides.** Rate limiting, cooldown, and
   suppression are backend-owned, matching the existing motion-authorization
   posture.
6. **Queue depth 1 with coalescing, never a backlog.** An image that arrives
   four messages late is worse than no image.
7. **Assume single-GPU contention is the hard problem**, because it is.

## Why This Is The TTS Problem Again

Speech already is: an optional, out-of-core, GPU-heavy generator whose output
must be attached to one specific chat message, must never delay that message,
and must never break chat when it fails. That machinery exists:

- `internal/voice/protocol/protocol.go` — versioned NDJSON-over-stdio frames, a
  leaf package workers import without touching core internals. Roles are a
  closed enum (`RoleTTS`, `RoleASR`) with `ParseRole` at the API edge.
- `internal/httpapi/chat.go:864` `enqueueSpeech` — submitted **strictly after**
  the chat log append, and its failures "stay in the voice request log — never
  in chat".
- `internal/httpapi/chat.go:887` `enqueueSpeechAt` — orders submission against
  Emergency Stop without letting Stop wait on it, via a stop epoch checked on
  both sides of the submit.
- `internal/chat/log.go:79` `SpeechRequestID` — how a late artifact finds its
  message.

An image feature that reinvents any of this is a mistake. It should reuse the
worker protocol, the job/cancel lifecycle, the "append first, enqueue second"
ordering, and the "optional feature failures never reach chat" rule verbatim.

### The three places images genuinely differ

**1. Images are durable; audio is not.** `SpeechRequestID` is not a database
column. It is a process-local map, `chatSpeechRequests map[int64]string`
(`internal/httpapi/server.go:106`), pruned in
`internal/httpapi/autopilot.go:331`, and decorated onto messages at read time
(`internal/httpapi/chat.go:970`). That is correct for play-once audio and wrong
for images: a user will scroll back a week later and expect the picture to still
be there. **Image correlation must be persisted** — a column on the chat message
row, or a small `chat_images` table keyed by session and seq.

**2. Images are an order of magnitude slower.** TTS is seconds. SDXL is roughly
5–20 s on a mid-range card, Flux considerably more. The reply cannot wait, so
the message must render immediately with an inline placeholder that fills in
later. This is a UI requirement, not just a backend one.

**3. Images contend with the LLM for the same VRAM.** Discussed below; this is
the biggest practical risk and the easiest one to under-plan.

## Provider Evaluation

### ComfyUI (recommended first provider)

Good: it is the de-facto local standard, the user's dev already suggested it,
many users already run it, it has a plain HTTP API (`POST /prompt`, poll
`/history/{id}`, fetch `/view`), a websocket for progress, and an enormous model
and extension ecosystem. Critically, **all image complexity stays in someone
else's process** — the same reason managed llama.cpp and the voice modules live
outside the core.

Bad, and the part that needs designing around: **ComfyUI's API is a workflow
graph, not a stable contract.** You POST a JSON graph whose node IDs are
positional and whose node types depend on which custom nodes that install has.
A workflow exported from a different ComfyUI version, or one referencing a
missing custom node, fails opaquely. Passing user-supplied workflow JSON
straight through would make every support question unanswerable.

The fix is the same trick used elsewhere in this repo — ship a known-good
default and make the escape hatch explicit:

> **MagicHandy ships its own minimal workflow template**, embedded in the
> binary, using only stock ComfyUI nodes. It declares a documented set of
> substitution points: positive prompt, negative prompt, seed, width/height,
> steps/cfg, and checkpoint name. On connect, the app validates that the target
> ComfyUI has the node types the template needs, and reports precisely what is
> missing if not.

An advanced path can accept a user workflow, but only if the user marks the
substitution points (by node title, e.g. a node titled `MH_POSITIVE`). That is a
clearly-labeled advanced feature, mirroring "External llama.cpp server —
MagicHandy will not install or own that process."

### Alternatives considered

**Stable Diffusion WebUI (A1111/Forge) `--api`** — genuinely simpler:
`POST /sdapi/v1/txt2img` with a flat JSON body, no graph. If the only goal were
"get a picture", this is the least-effort integration by a wide margin. Against
it: a smaller and shrinking share of local installs, less active maintenance,
and much weaker support for the reference-image techniques (IP-Adapter etc.)
that the persona-consistency story eventually wants. **Worth supporting as a
second provider precisely because it is trivial once the interface exists** —
it is a good forcing function to keep the interface honest.

**stable-diffusion.cpp** — would mirror managed llama.cpp exactly: a pinned,
app-owned, GGUF-based binary with no Python. Architecturally the most elegant
fit. Against it today: immature relative to ComfyUI, slower, narrower model
support. Worth revisiting if a managed image path is ever wanted, because it
avoids the entire Python-provisioning problem that Phase 16 is still wrestling
with.

**Cloud APIs (Replicate, Fal, Stability, OpenAI)** — fast, no VRAM contention,
no install. Two hard objections. First, it breaks the product's privacy posture:
this app keeps the connection key off the wire, binds to loopback, and runs the
LLM locally; shipping the user's intimate scene descriptions to a third party is
a different product. Second, and more practically, **most of these providers'
terms prohibit explicit content**, so the primary use case would violate them.
If offered at all it should be opt-in, clearly labeled as off-machine, and
never the default.

## What The Model Actually Emits

The contract is already capability-gated and composed in code:
`contractInstructions(capabilities)` in `internal/chat/prompts.go` assembles
`contractBase` plus `contractPatternSection`, `contractAreaSection`, and the
mood section only when those capabilities are on. Images slot in the same way —
add `Images bool` to `Capabilities`, compose a `contractImageSection` only when
a provider is configured *and* the user enabled it.

**The model must not write the diffusion prompt.** Two reasons. A local 7B
asked for SDXL-style prompt syntax produces mush, and it inflates every reply's
token cost — a live concern in this app, where the prompt was already cut from
~15,900 to ~4,400 tokens for latency. Instead:

```json
{
  "reply": "...",
  "image": { "scene": "kneeling on the bed, looking up", "framing": "close" }
}
```

Bounded, cheap, and in the model's actual competence: *what is happening*, not
*how to spell it for a diffusion model*. The backend composes the real prompt:

```
[persona base tags] + [scene] + [framing preset] + [style preset] + [negatives]
```

Every part except `scene` is backend- or user-owned and stable, which is what
makes successive images look like the same character.

`framing` should be a closed enum (`close`, `portrait`, `full`, `wide`) mapped
to composition tags and aspect ratio in code — not free text.

## Persona Base Design: Tags, Reference Images, Or Both

This was the direct question. The answer is **tags as the foundation, reference
images as an optional layer**, and the reasoning is about failure modes.

**Tags (recommended, mandatory layer).** Cheap, deterministic, human-editable,
and — the decisive property — **portable**. A tag list survives swapping the
checkpoint, changing provider, or a ComfyUI upgrade. A reference-image pipeline
does not: it pins the persona to a specific set of custom nodes and models. Tags
also degrade gracefully; a missing node is fatal, a slightly-off tag is not.

Store as a structured field on the persona, beside the existing character sheet:

- `appearance_tags` — the identity anchor (hair, eyes, build, age framing,
  defining features, outfit default)
- `negative_tags` — persona-specific exclusions
- `style_preset` — a shared, user-editable look (photoreal / illustrated / anime)
  so several personas can share one visual language
- later, without migration pain: `lora_name` + `lora_trigger`, `reference_mode`

Note this is deliberately **separate from `Description`**. The description is
prose the LLM reads to act the character
(`internal/persona/store.go:67`); appearance tags are never sent to the LLM at
all. Keeping them apart avoids both bloating the chat prompt and having the
model paraphrase the appearance into drift.

**Reference images (recommended as a later, optional layer).** IP-Adapter or
reference-only ControlNet gives markedly better identity consistency than tags
alone. Two things make this attractive *later* rather than now: it needs extra
models and custom nodes in the user's ComfyUI (so it cannot be the default
path), and **the persona already has a portrait** — `HasPortrait` /
`PortraitUpdatedAt` (`internal/persona/store.go:74`), stored as bounded JPEG
under the data directory (`internal/persona/portrait.go`, 2 MiB / 1024 px cap).
That existing portrait is the natural reference image, already validated, already
purgeable, already copied on persona duplication. The schema should anticipate
it; the first slice should not depend on it.

**LoRA is the strongest identity mechanism** and the design should not preclude
naming one per persona — but training is firmly out of scope.

**"A series of example images"** — worth naming as the option to *avoid* for
now. Multi-image identity conditioning is either LoRA training (out of scope) or
an averaged IP-Adapter embedding (fiddly, node-dependent, hard to explain when
it goes wrong). One reference image plus good tags gets most of the benefit for
a fraction of the surface area.

## Queueing, Pairing, And Pacing

### Flow

1. Model returns the reply, optionally with an `image` intent.
2. Reply is appended to the chat log and committed. **Reply latency is
   unchanged** — this is non-negotiable and mirrors `enqueueSpeech`.
3. If images are enabled and the rate policy allows, the backend composes the
   prompt, enqueues a job, and **persists** `image_id` against that message seq.
4. The client renders the message immediately with an inline placeholder — a
   fixed-aspect box with progress and a cancel control, so the transcript does
   not reflow when the image lands.
5. On completion the bytes are written under the data directory and the row is
   updated; the placeholder fills in via the existing SSE/poll path.
6. On failure the placeholder becomes a quiet, dismissible "image failed" note
   with a retry. **The chat message itself is never affected.**

### Queue policy

- **Depth 1, plus at most one waiting job; coalesce rather than accumulate.** If
  a third request arrives, drop the *waiting* one and keep the newest. Late
  images are actively bad — an image of a moment four exchanges ago is confusing,
  not delightful.
- **Cancel on Emergency Stop**, using the same epoch discipline as
  `enqueueSpeechAt`. An image is not a physical risk, but consistency is cheap
  and "everything stops when I hit Stop" is worth more than one salvaged
  render.
- **Cancel on session switch**; the job is meaningless in another conversation.

### Pacing — the model requests, the backend decides

Let the model ask freely and it will ask constantly. Rate policy belongs in the
backend, matching how motion authorization already works here:

- a cooldown (start ~60 s) between generated images
- a per-session cap
- suppression while an image is already running
- an explicit user control: Off / On request / Automatic

The model is told only that an image *may* be requested and that most turns
should not request one. It is never told whether the request was honoured — that
would invite it to complain about being rate limited in the reply text.

A manual "generate an image of this moment" affordance on a message is probably
the highest-value-per-effort version of the whole feature, and should exist
regardless of whether automatic requests do.

## The Hard Problem: Single-GPU Contention

This deserves more weight than it usually gets in a design like this.

A managed llama.cpp with a 7B Q4 model holds roughly 5 GB. SDXL wants ~7 GB,
Flux considerably more. Faster Qwen3-TTS is CUDA-only and wants its own. On a
16 GB card with all three enabled, the realistic outcomes are an out-of-memory
failure or heavy swapping that makes chat latency far worse than the images are
worth — and chat latency is already a live complaint in this project.

Options, least to most invasive:

1. **Document and default off.** If managed llama.cpp is on CUDA and the card is
   under ~16 GB, images default to off with a clear explanation. Cheap, honest,
   and probably right for v1.
2. **Prefer a remote ComfyUI.** Because the recommended integration is "point at
   an existing ComfyUI", pointing at another machine already works for free.
   This should be surfaced as the recommended configuration for single-GPU
   users, not buried.
3. **A GPU lease** that serialises image generation against managed-llama model
   *load* so the two never initialise concurrently. Bounded work, real benefit.
4. **Unload the LLM around each image.** Correct in principle, awful in practice
   — multi-second reload per image. Not recommended.

The hardware probe needed for option 1 is arriving anyway: the Phase 16 setup
work in PR #195 adds `/api/setup`, which already reports `nvidia`, `gpu_name`,
and `vram_mib`. If that lands first, option 1 is nearly free.

## Bloat Assessment

The concern that a more integrated approach bloats the core is right in general
and avoidable here. What the core would actually gain:

- a provider interface plus one HTTP client (ComfyUI's API is plain JSON)
- a job queue and cancel path — largely a reuse of the voice lifecycle
- a persisted correlation row and a file-serving endpoint
- settings, a contract section, and a prompt composer

What the core would **not** gain, and must not: any image decoding, encoding, or
scaling. Portraits already establish this rule explicitly — scaling happens in
the browser because "server-side scaling would need a new image-scaling
dependency or FFmpeg, and FFmpeg is deliberately optional. A portrait must not
be what makes it mandatory" (`internal/persona/portrait.go:27`). Generated
images inherit that: bytes arrive from the provider, get bounds-checked, and are
written to disk. `CGO_ENABLED=0` and the <30 MB binary budget are unaffected.

The multi-gigabyte weight — ComfyUI, Python, checkpoints — stays in the user's
own install, exactly as Ollama does.

## Suggested Slices

- **17.x.0 — provider plumbing.** Settings (provider, base URL, enable), a
  connection check that validates required node types, the pinned workflow
  template, prompt composition, and a manual "generate image" action on a
  message. No LLM involvement at all. This alone is shippable and useful.
- **17.x.1 — persona appearance.** `appearance_tags`, `negative_tags`,
  `style_preset` on the persona, editor UI, applied to composition.
- **17.x.2 — chat integration.** Contract section behind a capability gate,
  persisted correlation, inline placeholder, queue with coalescing, Stop
  cancellation, backend rate policy.
- **17.x.3 — retention.** Per-session and total-size caps, purge with chat
  history, disclosure of the storage path. Images accumulate fast; this cannot
  be an afterthought.
- **17.x.4 — optional consistency layer.** Reference-image conditioning using
  the existing persona portrait, gated on the user's ComfyUI actually having the
  nodes.
- **Later, if wanted:** a second provider (A1111-style REST) to prove the
  interface; `stable-diffusion.cpp` as a managed path.

## Open Questions

- **Retention default.** Keep every image forever, or cap and prune oldest?
  Leaning cap-by-total-size with an explicit setting, since these are large and
  the data directory is already the thing uninstall deliberately preserves.
- **Do generated images belong in the media library** alongside videos, or only
  inline in chat? Reusing the media catalog is tempting but it is built around
  playable video with funscript offsets; a separate lightweight store is
  probably cleaner.
- **Real-person likeness.** Tags or a reference image can describe a real
  person. Worth a stated policy line before the reference-image slice, which is
  the one that makes it materially easier.
- **Should a failed image be visible at all?** A quiet inline note is proposed
  above, but "silently nothing" is defensible and less noisy.
