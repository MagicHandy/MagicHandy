# ADR 0074: Windows icon and installer artwork

## Status

Accepted 2026-11-14 after independent code and native-resource review, under
the owner's instruction to publish the Windows branding integration. Covers the Windows surfaces that
[ADR 0073](0073-brand-mark-and-palettes.md) left without the mark.

## Context

Alpha.87 put the brand mark in the app and the browser tab. On Windows the
installed app still showed the generic executable icon on the Start menu, the
desktop shortcut, the taskbar, the launch console window and the Apps list,
and the installer used Inno Setup's stock icon and images.

A Windows executable takes its icon from a resource linked into the file. The
Go toolchain has no flag for that; it links any `*.syso` object it finds in a
package directory. The usual way to produce one is a third-party resource
compiler.

## Decision

1. **One folder of brand assets.** `assets/brand/` holds the icon, the
   installer images and the SVG masters. Nothing else in the repository
   duplicates them.
2. **The icon.** `magichandy.ico` carries ten sizes from 16 to 256 px in the
   default Steel Azure palette. Sizes of 32 px and above show the full mark;
   16, 20 and 24 px show the star alone, as the browser tab does.
3. **The executable links the icon without a new dependency.**
   `internal/winresource` writes the COFF resource object from the icon file
   using only the standard library, and `cmd/windows-resource` writes it to
   `cmd/magichandy/resource_windows_amd64.syso`. The object is committed, like
   the embedded web build, so `go build` needs no extra step and no tool is
   downloaded at build time. A test fails when the committed object is not
   exactly what the committed icon produces.
4. **Only the core executable is branded.** The voice and Bluetooth workers
   never show a window or a shortcut, so they keep no icon.
5. **The installer shows the mark.** `installer/magichandy.iss` sets
   `SetupIconFile`, `WizardImageFile` and `WizardSmallImageFile` from
   `assets/brand/`, each wizard image at two sizes for display scaling.
   Shortcuts and the Apps list already point at `magichandy.exe`, so they pick
   the icon up from it.
6. **Still not a status light.** The icon is fixed artwork. It does not change
   with device, session or connection state, or with the saved palette.

## Consequences

- `magichandy.exe` grows by the size of the icon: 175,616 bytes on the
  measured build. The measurement is in
  [the goal scorecard](../goal-scorecard.md).
- The resource object is a binary file in the source tree. Its only input is
  the icon, and the generator is deterministic.
- This is a narrow repository-hygiene exception for the single deterministic
  Windows resource link input, not permission to commit executables, runtime
  state or general build output. The icon remains in `assets/brand/`; the
  committed object is checked byte for byte against it by the Go test suite.
- The object's file name limits it to Windows on amd64, the only Windows
  target that ships. Other platforms build exactly as before.
- Changing the icon is two steps: replace `magichandy.ico`, then run
  `go run ./cmd/windows-resource`. The test catches a missed second step.

## Not covered

- Version information in the executable's Properties dialog.
- The README header and a social preview image.
- An application manifest. None is added, so display scaling and elevation
  behave as before.
