# Managed TTS update prompt and alpha.44 review

The app now detects when the selected managed Qwen3-TTS or Chatterbox adapter,
streaming helper, or pinned source revision differs from the bundled release.
The backend caches local bounded file comparisons for 30 seconds and supplies
one update identity. Notifications use that identity for deduplication and link
to Voice settings. The user explicitly starts the staged installer after seeing
the download/disk implications. Existing local Qwen model directories bypass
model downloads. Activation changes only the runtime root; a changed TTS
selection rejects activation, and cancellation targets the displayed job ID.

Validation on Windows x64, Go 1.26.4:

- Full Go tests, vet, lint, race suite, import boundaries and lifecycle gates pass.
- Frontend typecheck, localization (1,976 keys in five locales), 507 tests in 69
  files, and the canonical production build pass.
- Windows PowerShell 5.1 installer fixtures pass, including local-model selection,
  staged-runtime isolation, and the exact alpha.44 unsigned-policy allowlist.
- Seven lightweight Python speech-adapter tests pass. Publication also gates
  them on Python 3.10 and 3.11 in CI.
- HTTP regression tests verify stale-update rejection, controller ownership,
  cancellation identity, and settings preservation. A Windows fixture installer
  runs through the real update endpoint, job process, candidate validation and
  activation without downloading models or running inference.
- In-app browser review follows the one-time notification to Voice settings,
  displays the update action and download explanation at a narrow viewport, and
  keeps Emergency Stop visible. An isolated old-adapter fixture deliberately
  contains no working TTS runtime; no speech installation is triggered by review.
- The current build passes `check-review-llm.ps1` against local Ollama with
  `huihui_ai/granite4.1-abliterated:3b`. A real text-only app chat returns
  "The text-only release review is ready." with one generation, no repair or
  fallback, motion disabled, and voice off (121 ms total; 53 ms first token).

The stripped app is 19,218,432 bytes, 35,328 bytes above merged PR #261. Main JS
is 766,507 bytes raw / 211,775 gzip (level 9); the embedded tree is 2,028,522
bytes. This adds 762 bytes gzip to the main JS and 7,749 raw bytes across all
assets. Measurements use `CGO_ENABLED=0`, `-trimpath`, and `-s -w`, with release
version metadata. No new Go, JavaScript, or Python dependency is introduced.
Fresh isolated simulator startup is 583.4 ms; three idle working-set samples are
34,283,520 bytes, private memory 57,245,696 bytes. Host variation prevents a
whole-app speed or RSS improvement claim; existing budget waivers remain.

Real speech listening, sentence-join/loudness acceptance, shared-GPU coexistence,
and a full multi-GiB module download/update remain alpha acceptance work. This
change does not modify motion generation, transport dispatch, or the installed
user instance. The release follows ADR 0014's explicit alpha.44 amendment;
signature/Defender, artifact hashes, provenance and installer gates are retained.
