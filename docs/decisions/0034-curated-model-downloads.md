# ADR 0034: Curated model downloads in guided setup

Date: 2026-09-29

Status: Accepted

## Context

Chat and Autopilot need a local model, and MagicHandy ships none. Guided setup
could only import a GGUF file the user already had or copy one from an Ollama
library. A new user without a model reached the Model step with no way forward:
setup gave no recommendation, and skipping the step also dropped the managed
runtime install (the wizard reset its runtime choice to `skip`). The
[model suitability review](../model-suitability.md) had already named a tested
default, and Phase 16 listed curated checksum-pinned downloads and
hardware-fit recommendations as open work.

The pieces were already present: a verified, resumable Hugging Face download
for Parakeet, a managed model store with SHA-256 checks and import jobs, a
sequential setup install queue, and `nvidia-smi` GPU memory detection that the
wizard never used.

## Decision

MagicHandy ships a small curated catalog inside the binary
(`internal/llm/model_catalog.go`). Each entry pins a display name, source page,
license and license link, byte size, SHA-256, measured graphics memory need and
a minimum graphics memory. The catalog changes only with a release; nothing
fetches or updates it at runtime.

Entries are content-addressed Ollama registry blobs
(`https://registry.ollama.ai/v2/<repository>/blobs/sha256:<digest>`). The two
tested 12B models from the suitability review are only published there, and the
digest in the URL is the SHA-256 of the bytes, so the source cannot change what
a pinned entry downloads. The download follows the registry's redirect, resumes
with HTTP ranges, retries three times, cancels an attempt that stalls for 90
seconds, rejects any size other than the pinned one, and hashes the whole file
before it can enter the store. A partial download is kept across cancellation,
failure and restart, and startup deletes only partials whose digest the current
catalog no longer lists. A finished download goes through the same GGUF
validation and store commit as an import; a model already in the store with the
same digest completes without touching the network. Records keep the `ollama`
source, so no schema migration is needed.

HTTP handlers accept only a catalog ID (`POST /api/llm/imports/catalog`); a URL is
never accepted. Before any bytes move, the backend checks free disk space for the
remaining size plus 512 MiB. `GET /api/llm/catalog` rates every entry against the
detected GPU: `recommended` for the default on a card with at least the minimum
memory, `supported` for other entries that fit, `below_minimum`, `unknown_vram`,
or `no_gpu`. Only measured memory needs are used, so a smaller card is told the
model will not fit rather than offered an untested recommendation.

Guided setup merges the runtime and model steps into one Chat AI step. The
recommended download is preselected when the GPU holds it, but downloading is
still an explicit plan item: nothing starts until the user continues past the
Voice step, and the plan shows the license, source page and size first. The
setup install plan runs the runtime, then the model, then voice, then Parakeet,
and selects the downloaded model once it is verified. "Add a model later" keeps
the runtime install; Skip skips chat entirely. Without an NVIDIA GPU the step
starts on Skip and says that CPU inference is too slow for live chat.

## Measurements

On the managed CUDA runtime `b9966`, the default model
(`n0404n0404/gemma-4-12b-it-heretic-b9a462`, 7,381,381,696 bytes) used 8,144 MiB
of dedicated GPU memory at a 16,384-token context, read from the Windows
`GPU Process Memory` counter on an RTX 5070 Ti. Its GGUF metadata shows full
attention on 8 of 48 layers with one 512-wide KV head each; the other 40 layers
use a 1,024-token sliding window. The default 32,768-token context therefore adds
16,384 × 8 layers × 2 × 512 × 2 bytes = 256 MiB of f16 KV cache, for about
8,400 MiB. The alternative
(`igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic`) shares the architecture
and has 387 MiB smaller weights, about 8,013 MiB. The 10,240 MiB minimum leaves
room for the desktop. Cards below it, including 8 GB cards, are not offered a
recommendation.

A live download of the default entry through the setup plan completed from the
real registry: cancelled at 2.83 GiB, resumed with a range request, verified,
committed and selected.

## Consequences

- A first run can go from install to chat without leaving the app.
- The catalog depends on third-party repositories. Pinning cannot stop a
  repository from disappearing; that surfaces as a download error, and GGUF
  import remains the fallback.
- Catalog entries are community fine-tunes of Gemma 4. Their license is shown as
  the base model's Apache-2.0 terms with a link, and the source page is shown
  before download.
- Adding or replacing an entry is a reviewed code change with a new size,
  SHA-256 and measured memory figure.
- Parakeet keeps its own downloader for now; both follow the same verify-then-
  activate pattern.

## Amendment (2026-09-30): three entries, Hugging Face pins, per-card recommendation

A second evaluation (see [model suitability](../model-suitability.md), "2026-09-30
evaluation") replaced the alternative and added a small-card entry. The catalog
is now, in preference order:

1. `n0404n0404/gemma-4-12b-it-heretic-b9a462` (Ollama blob, unchanged default).
2. `OS-Software/gemma-4-12B-it-qat-q4_0-uncensored-heretic-v3` Q4_0, 6,716,356,480
   bytes, 7,766 MiB at 32,768 tokens. It matched the default's contract results
   with no structural violations, in about 0.6 GB less memory. It replaces the
   igorls QAT build.
3. `HTNZ555/gemma-4-E4B-it-heretic-QAT` (UD-Q4_K_XL file, Q4_0 weights),
   4,255,261,568 bytes, 3,446 MiB. It is the fastest tested model (about 0.65 s
   per turn) and gives 6 to 8 GB cards a working recommendation. Its chat template
   needs the Gemma 4 thinking fix from ADR 0035, which the runner applies
   automatically for this file.

Entries may also be Hugging Face files pinned to one repository commit
(`https://huggingface.co/<owner>/<repo>/resolve/<40-hex commit>/<file>.gguf`).
The file's bytes cannot change without the commit changing, and the download is
still verified against the pinned SHA-256. The catalog test rejects any other
URL shape. Hugging Face entries are recorded with the `gguf` source; Ollama
blobs keep `ollama`.

`recommended` now means the first entry in catalog order that the detected GPU
holds, instead of always the default. A 16 GB card is recommended the 12B
default, and an 8 GB card the E4B. When the GPU's memory cannot be read, setup
still preselects the tuned default.
