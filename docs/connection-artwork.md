# Connection Artwork: Provenance, Composition, And Refactoring

This note documents the artwork inside the top-bar connection manager. It is a
maintenance reference for changing the composition without rediscovering how a
generated bitmap, SVG geometry, CSS state, and panel layout fit together.

## Production pieces

| Piece | Canonical source | Purpose |
|---|---|---|
| Isolated hand | `web/src/assets/conductor-hand-v2.png` | Transparent 1280 x 1280 bitmap preserving the generated graphite/starfield hand. |
| Composition | `web/src/shell/ConnectionManager.tsx`, `ConnectionArtwork` | One 360 x 260 SVG coordinate space containing the bitmap, signal paths, failed-attempt mark, and Handy-inspired target. |
| State and motion | `web/src/styles/shell.css`, `.connection-*` rules | Shows the correct signal/status treatment for connected, connecting, disconnected, and error states. |
| Embedded output | `web/dist/assets/` | Vite-hashed production copies served by the Go binary. Do not edit these by hand. |

There is one source bitmap and one generated `dist` copy. The original concept
sheet lived under the ignored `.scratch/branding-review/` workspace and is not a
shipping dependency.

## How the source image was generated

The original square conductor poster was generated from this design brief:

> Flat 2D graphic design poster, vertical portrait format. Two black rounded
> shapes, one tall capsule and one shorter arch, sit side by side like simplified
> Handy device silhouettes at the bottom. Above them, one graphite-black open
> hand hovers palm down. Three thin steel-azure arcs connect the hand and device.
> Include one small green dot and one small red square. Use an off-white paper
> background and a flat screen-print or risograph treatment. No text, gradients,
> 3D, or photorealism.

The production hand was then made as a hand-only transparent isolation. The
important edit constraints were:

1. Preserve the hand's silhouette, finger spacing, wrist angle, paper grain, and
   white star speckles from the reviewed poster.
2. Remove the paper background, signal arcs, device shapes, LED, and square.
3. Make every non-hand pixel transparent; do not substitute a dark background.
4. Keep a square canvas and enough transparent margin that the wrist and fingers
   are not clipped when scaled with `preserveAspectRatio="xMidYMid meet"`.
5. Inspect the exported alpha edge at 100% before replacing the production PNG.

This isolation avoids the earlier runtime luminance mask and approximate clip,
which distorted the fingers and made the result sensitive to background color.
If the hand is regenerated, use an image editor or image-generation edit with
the reviewed source as a reference; do not ask a new generation to redraw it
from text alone unless a deliberately different hand is desired.

## How the window composition works

`ConnectionArtwork` uses a fixed `viewBox="0 0 360 260"`. The browser scales the
whole composition uniformly to the panel width.

- The 1280-square hand is placed as an SVG `<image>` at `x=30`, `y=-77`,
  `width=300`, `height=300`. Its transparent margins make the visible hand end
  above the signals even though the image box overlaps them.
- `SIGNAL_PATHS` contains exactly three quadratic paths. They progress from a
  narrow inner arc to a wider outer arc and cascade downward toward the target.
- The target recreates the reference geometry rather than a product drawing: a
  tall 27 x 70 capsule, a shorter domed body aligned at the baseline, a centered
  LED, and a small square marker to the right.
- The error X occupies the signal zone only after a connection attempt fails.
  The target remains visible so the X reads as a failed path between the hand
  and device; an ordinary disconnected state has neither arcs nor an X.

The target is intentionally vector geometry. It stays sharp, costs little, and
can be proportioned independently of the generated hand. The square marker is
red while disconnected or errored, blue while connecting, and green only when
connected. The X uses red only as failure feedback, never as idle decoration.

## State contract

| Phase | Arcs | X | LED | Square | Motion |
|---|---|---|---|---|---|
| `initializing` | Hidden | Hidden | Muted | Muted | None |
| `connected` | Intense blue, visible | Hidden | Green | Green | Static |
| `connecting` | Intense blue, visible | Hidden | Blue | Blue | Staggered opacity wave |
| `disconnected` | Hidden | Hidden | Muted | Red | None |
| `error` | Hidden | Red | Muted | Red | One brief X shake on entry |

`initializing` lasts only until the first backend snapshot arrives and does not
guess a provider or failure state. `prefers-reduced-motion: reduce` removes both
the signal wave and error shake, leaving static state feedback. The error phase
is entered only from a failed provider connection attempt; a backend-offline or
never-connected state remains disconnected. Backend snapshots and provider
attempt results determine the phase; the artwork does not infer connection
state.

## Panel sizing

The artwork gained vertical room by compacting non-visual chrome, not by hiding
controls. The title is one line, the current-device row is 44px minimum, provider
spacing is reduced, and the Limits heading/grid use tighter padding. Cloud REST
adds a compact write-only connection-key form and a visible bundled/developer
API v3 ID source readout. The key is saved through
`PUT /api/settings/device/connection-key`; responses expose only
`connection_key_set` and never echo the credential.

The panel remains capped by the viewport. Mobile must retain the reserved Stop
and navigation region defined in `shell.css`; adding content may require internal
scrolling, but must not move or cover Stop.

## Safe ways to change it

- **Rebalance spacing:** edit the SVG coordinates and `viewBox` together. Keep
  every visible element inside the frame and leave at least 6px around the target.
- **Replace the hand:** keep a transparent square PNG, update the import, then
  tune only the `<image>` rectangle. Do not add a second source copy.
- **Change the signal shape:** keep three semantic paths and the existing CSS
  classes so state and reduced-motion behavior remain intact.
- **Use a full bitmap composition:** acceptable only if the target never needs
  state-aware colors or animation. It gives up sharp scaling and usually adds
  another large asset.
- **Use canvas, video, or Lottie:** avoid unless interaction becomes materially
  more complex. They add payload and accessibility/testing work for behavior SVG
  and CSS already express.

## Refactor checklist

1. Run `npm test -- --run` and `npm run build` from `web/`.
2. Confirm tests still find one hand image, three signal paths, two target body
   shapes, one LED, one square marker, and the failed-attempt X.
3. Render the open manager at 1280 x 800 and 390 x 844. Check all four states,
   no clipping, no horizontal overflow, and access to all four limit sliders.
4. Check reduced motion in browser emulation.
5. Confirm the Cloud key input is present only for Cloud REST, is disabled for a
   read-only/offline client, and no request, toast, log, or response exposes it.
6. Rebuild the embedded `web/dist` output and remeasure browser/binary budgets if
   the bitmap or bundle size changes.
