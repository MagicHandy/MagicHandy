# ADR 0011: Thin Windows Installer And In-App Setup Wizard

## Status

Accepted for planning. Implementation is Phase 16.

## Context

MagicHandy needs a Windows installation path for non-technical users. Setup must
handle OS integration, LLM runtime and model choices, optional voice providers,
and legacy-data migration without creating a second application or duplicating
the app's existing APIs and React settings surfaces.

Native installer frameworks are good at installing files, shortcuts, and
uninstall metadata but poor at model catalogs, consent, progress, cancellation,
and migration previews. A dedicated Electron, Tauri, or WebView2 installer would
add another runtime and UI stack while still needing to call the MagicHandy API.
The source-build path can now start on a clean machine by provisioning Go, Git,
CMake, and Visual Studio C++ Build Tools itself. That improves bootstrap parity,
but it still cannot satisfy a no-toolchain end-user claim because those tools
are installed on the machine and consume several GB.

## Decision

1. Inno Setup is the thin Windows installation shell. It installs the app,
   creates shortcuts and Add/Remove Programs metadata, supports unattended and
   over-install upgrades, leaves user data on uninstall, and launches setup.
2. The embedded React app owns interactive first-run setup at `#/setup`. The
   wizard arranges existing settings and API operations and remains re-runnable
   from Settings. It does not duplicate provider, model, migration, or consent
   logic in installer script.
3. The portable zip and setup binary are built from the same versioned release
   payload. Inno Setup is a build-time dependency only.
4. Phase 16 publishes checksum-pinned CPU and CUDA llama.cpp runtime bundles
   with manifests, size, license, and backend information. The wizard uses these
   prebuilt bundles by default; source build is an advanced/developer fallback.
   The app cannot claim a no-toolchain-footprint setup path until this is
   implemented and tested on a machine without installing Go, Git, CMake, or
   Visual Studio.
5. Every network download remains an explicit user action with visible size,
   license, checksum verification, progress, cancellation, and atomic install.
6. Production signing, auto-update, a WebView2 presentation shell, and LAN/HTTPS
   exposure remain separate Phase 16 decisions.

## Consequences

Positive:

- One interactive UI stack and one implementation of setup operations.
- Installer code stays small and focused on Windows integration.
- Setup capabilities remain useful and testable after first run.
- Prebuilt managed runtimes remove the source-toolchain requirement for release
  users without changing the pure-Go core boundary.
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
