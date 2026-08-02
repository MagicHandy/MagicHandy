# ADR 0011: Thin Windows Installer And In-App Setup Wizard

## Status

Accepted. Implementation is Phase 16.

## Context

MagicHandy needs a Windows installation path for non-technical users. Setup must
handle OS integration, LLM runtime and model choices, optional voice providers,
and legacy-data migration without creating a second application or duplicating
the app's existing APIs and React settings surfaces.

Native installer frameworks are good at installing files, shortcuts, and
uninstall metadata but poor at model catalogs, consent, progress, cancellation,
and migration previews. A dedicated Electron, Tauri, or WebView2 installer would
add another runtime and UI stack while still needing to call the MagicHandy API.
The source bootstrap can start on a clean machine by provisioning the tools
needed for the pure-Go app build. Optional managed llama.cpp must not expand
that requirement into a C++/CUDA compiler installation for packaged users.

## Decision

1. Inno Setup is the thin Windows installation shell. It installs the app,
   creates shortcuts and Add/Remove Programs metadata, supports unattended and
   over-install upgrades, asks whether to purge app-owned data on interactive
   uninstall, and launches setup. Silent uninstall purges by default;
   `/KEEPUSERDATA` is the explicit retention override.
2. The embedded React app owns interactive first-run setup at `#/setup`. The
   wizard arranges existing settings and API operations and remains re-runnable
   from Settings. It does not duplicate provider, model, migration, or consent
   logic in installer script.
3. The portable zip and setup binary are built from the same versioned release
   payload. Inno Setup is a build-time dependency only.
4. The wizard exposes checksum-pinned official llama.cpp CPU and CUDA bundles,
   their download/install cost, MIT license, and hardware requirements. The
   managed path requires no compiler or CUDA Toolkit and is tested without Git,
   CMake, or Visual Studio. Existing Ollama, an external server, and skipping
   chat remain explicit alternatives.
5. Every network download remains an explicit user action with visible size,
   license, checksum verification, progress, cancellation, and atomic install.
6. Production signing, auto-update, a WebView2 presentation shell, and LAN/HTTPS
   exposure remain separate Phase 16 decisions.

## Consequences

Positive:

- One interactive UI stack and one implementation of setup operations.
- Installer code stays small and focused on Windows integration.
- Setup capabilities remain useful and testable after first run.
- The setup EXE and portable ZIP remove the source-toolchain requirement for
  the core app without changing the pure-Go boundary.
- Until those releases exist, `install.ps1` and `update.ps1` provide one shared,
  state-aware source workflow that can provision a bare machine without manual
  dependency hunting.

Negative:

- The release workflow must build, license, checksum, publish, and test multiple
  external runtime bundles in addition to the core app.
- Inno Setup becomes a Windows release build dependency.
- The default setup experience remains browser-hosted unless a later decision
  adds a presentation-only app window.

Detailed design and rejected-option analysis live in `docs/gui-installer.md`.
