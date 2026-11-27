# Alpha.91 Windows release review

The release candidate integrates three individually tested changes on top of
alpha.90 (`4286ae43`). Each feature PR had 17 passing CI checks and two
conditional transport jobs skipped. The integration preserves the three
reviewed heads. The overlapping scorecard additions were retained together.

| Change | Reviewed head | Evidence |
| --- | --- | --- |
| Inkling schemas and Gemma context retention | [#376](https://github.com/MagicHandy/MagicHandy/pull/376) · `392b3da5` | [Gemma and GPT response comparison](inkling-local-evaluation-2026-11-21.md#gemma-and-gpt-regression-review) |
| Earlier Cloud refills and same-chat history lookup | [#380](https://github.com/MagicHandy/MagicHandy/pull/380) · `bad671eb` | [Cloud timing](hsp-v4-invariants.md) · [Saved user evidence](llm-control-surface.md) |
| Managed Xet installation and HTTP retry | [#381](https://github.com/MagicHandy/MagicHandy/pull/381) · `9928623c` | [Voice-module download behavior](voice-tts-modules.md) |

## Checks on the combined candidate

- The production Cloud encoder and scheduler ran against a simulated Original
  Handy in `TestCloudRefillArrivalAndStop`. All 12 repetitions passed. At
  injected delays of 1.0 and 1.4 seconds the refill arrived 504 and 104 ms
  before the previous tail. At 1.8 seconds the peer reported starvation and
  the engine stopped. The suite also checked retargeting, Emergency Stop
  cancellation of a held request, and the subsequent absence of movement writes.
- The normal streaming chat path recovered the initial paces of 25% and 35%
  and the five-point increase on both local Gemma 4 12B Intermediate and
  signed-in GPT-6.1 Terra. Both identified the separate checkpoint and
  baseline, returned unavailable fields for missing evidence, and kept motion
  idle throughout the historical questions. A fresh 42% instruction then
  reached the shared engine and simulated transport. The local test reused the
  catalog Q4_K_M weights on the running b11533 CUDA endpoint. Both tests used
  synthetic saved conversations.
- In disposable Windows voice environments, the integrated downloader installed
  `hf-xet` for Python 3.11 and completed a first model download. With the wheel
  index offline on Python 3.10 it downloaded the model over HTTP. Both downloads
  matched the published SHA-256 for the model weights.
- The stripped Windows amd64 core measured 25,712,128 bytes with Go 1.26.11,
  `CGO_ENABLED=0`, `-buildvcs=false -trimpath` and `-ldflags "-s -w"`.
  The public packaging workflow retains the symbols required for binary
  vulnerability analysis.

The timing measurements describe a simulated peer. Physical travel, real
internet jitter and physical-device acceptance require separate testing.
The voice check covered downloads with a small public model. The release
uses the existing unsigned Windows prerelease policy and requires the
exact-package Defender scan, manifest and SHA-256 verification, executable
checks and installer lifecycle before publication.
