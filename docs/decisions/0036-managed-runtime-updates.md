# ADR 0036: Managed llama.cpp updates follow the release pin

Date: 2026-09-30

Status: Accepted

## Context

MagicHandy installs one pinned llama.cpp build (ADR 0005). Each release names
the tag, commit, asset sizes and SHA-256 digests, and the installer verifies
every download before it replaces the active runtime. The pin had not moved
since `b9966` (2026-07-10). Newer models then needed architectures that build
lacked; Spark-X2.5, for example, arrived in llama.cpp on 2026-09-06.

Moving the pin was easy. The gap was what happened next on an existing install.
The app reported the runtime as `outdated` in Settings and kept using the old
build until someone pressed "Install / switch runtime". Nothing told the user,
and automatic reasoning budgets were off while the runtime was outdated.

## Decision

- The pin moves to `b11149` (commit `d2e54583`, the build for llama.cpp's
  `v0.5.0` release of 2026-09-23). It uses the same official Windows CPU and
  CUDA 12.4 archives, and the CUDA runtime archive is byte-identical to
  `b9966`'s. The E4B, 12B and 12B QAT catalog models measured the same on
  `b11149` as on `b9966`.
- Updates follow the release pin only. MagicHandy never installs an upstream
  build that no release has reviewed, so "automatic" still means a
  SHA-256-verified, release-pinned runtime.
- A new preference, `ui.runtime_update_mode`, is `automatic` (default) or
  `manual`. It sits in Settings > General > Updates beside the app's own release
  check.
  - **Automatic**: at startup, once setup is complete and managed llama.cpp is
    the chat engine, if the installed runtime is older than the pin, MagicHandy
    starts the verified install with the backend already in use (CPU or CUDA).
    A runtime left installed while chat uses Ollama or an external server is
    not downloaded again until managed mode is selected. The previous runtime stays active until
    the replacement is verified. Model autoload waits for the install and then
    runs as usual. Chat is unavailable for the minute or two the install takes.
  - **Manual**: nothing installs by itself. The notification and the Updates
    panel offer **Update now**.
- The state snapshot reports the installed and pinned versions and whether an
  install is running. The notification center posts "llama.cpp update
  available", "Updating llama.cpp" and "llama.cpp updated", each once per pinned
  version.
- The model test (ADR 0035) also reports an outdated runtime.

## Consequences

- A MagicHandy update that needs a newer runtime brings it along without a trip
  to Settings, unless the user chose manual updates.
- Upgrading from a release with an older pin costs one ~615 MiB CUDA download
  (18 MiB CPU) at the first start after the update.
- After a verified install, MagicHandy removes superseded install directories,
  so updates do not accumulate about 1.1 GiB per CUDA version. A directory still
  locked by an exiting runner is left for the next install.
- Tests replace the runtime installer (`Server.runtimeUpdater`), so the suite
  never downloads a runtime.
