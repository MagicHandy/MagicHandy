# Connection Artwork: Composition And Refactoring

This note documents the status artwork inside the top-bar connection manager.
It is a maintenance reference for changing the composition without
rediscovering how the SVG geometry, CSS state, and panel layout fit together.

## Production pieces

| Piece | Canonical source | Purpose |
|---|---|---|
| Composition | `web/src/shell/ConnectionManager.tsx`, `ConnectionArtwork` | One 360 x 120 SVG coordinate space containing the signal paths, failed-attempt mark, and Handy-inspired target. |
| State and motion | `web/src/styles/shell.css`, `.connection-*` rules | Shows the correct signal/status treatment for connected, connecting, disconnected, error, and initializing states. |

The artwork is vector-only. Earlier builds composed a generated conductor-hand
bitmap (`conductor-hand-v2.png`, about 444 KB) into a 360 x 260 frame; the
Graphite refactor (2026-10-09) removed it in favor of a flat status strip. The
strip keeps every state signal, scales sharply, follows the active palette
through tokens, and removes the largest single asset from the shipped UI. Do
not reintroduce an illustration here: the panel is an instrument readout.

## How the composition works

`ConnectionArtwork` uses a fixed `viewBox="0 0 360 120"` rendered as a 96px
strip (`padding: 8px 0`) on `--bg-inset` between `--line` rules. The browser
scales the composition uniformly and centers it.

- `SIGNAL_PATHS` contains exactly three quadratic arcs above the device,
  progressing from a narrow inner arc to a wider outer arc:
  `M166 34 Q176.5 25 187 34`, `M157 25 Q176.5 9 196 25`,
  `M148 16 Q176.5 -7 205 16`.
- The target recreates the reference geometry rather than a product drawing: a
  tall 27 x 70 capsule (`x=146 y=42`), a shorter domed body aligned at the
  baseline (`M180 112v-22.5…`), a centered LED (`cx=159.5 cy=77`), and a small
  7 x 7 square marker to the right (`x=216 y=105`).
- The error X (`m167 11 19 19` / `m186 11-19 19`) occupies the signal zone only
  after a connection attempt fails. The target remains visible so the X reads
  as a failed path to the device; an ordinary disconnected state has neither
  arcs nor an X.

Bodies use `--surface-2` with a `--line-strong` outline, so the device reads in
every palette without its own colors.

## State contract

| Phase | Arcs | X | LED | Square | Motion |
|---|---|---|---|---|---|
| `initializing` | Hidden | Hidden | Muted | Muted | None |
| `connected` | `--ok`, visible | Hidden | `--ok` | `--ok` | Static |
| `connecting` | `--accent`, visible | Hidden | `--accent` | `--accent` | Staggered opacity wave |
| `disconnected` | Hidden | Hidden | Muted | `--danger` | None |
| `error` | Hidden | `--danger` | Muted | `--danger` | One brief X shake on entry |

`initializing` lasts only until the first backend snapshot arrives and does not
guess a provider or failure state. `prefers-reduced-motion: reduce` removes both
the signal wave and error shake, leaving static state feedback. The error phase
is entered only from a failed provider connection attempt; a backend-offline or
never-connected state remains disconnected. Backend snapshots and provider
attempt results determine the phase; the artwork does not infer connection
state. Green appears only when the device is connected (running/ok), and red
only as a disconnected or failure mark, never as an action.

## Panel sizing

The panel's title is one line, the current-device row is 44px minimum, and the
Limits heading/grid use compact padding. Cloud REST adds a compact write-only
connection-key form and a visible bundled/developer API v3 ID source readout.
The key is saved through `PUT /api/settings/device/connection-key`; responses
expose only `connection_key_set` and never echo the credential.

The panel remains capped by the viewport. Mobile must retain the reserved Stop
and navigation region defined in `shell.css`; adding content may require
internal scrolling, but must not move or cover Stop.

## Safe ways to change it

- **Rebalance spacing:** edit the SVG coordinates and `viewBox` together. Keep
  every visible element inside the frame and leave at least 6px around the
  target.
- **Change the signal shape:** keep three semantic paths and the existing CSS
  classes so state and reduced-motion behavior remain intact.
- **Change colors:** use tokens only, and keep the state contract above.
- **Bitmaps, canvas, video, or Lottie:** avoid. They add payload and
  accessibility/testing work for behavior SVG and CSS already express.

## Refactor checklist

1. Run `npm test` and `npm run build` from `web/`.
2. Confirm tests still find no bitmap, three signal paths, two target body
   shapes, one LED, one square marker, and the failed-attempt X.
3. Render the open manager at 1280 x 800 and 390 x 844. Check all states, no
   clipping, no horizontal overflow, and access to all four limit sliders.
4. Check reduced motion in browser emulation.
5. Confirm the Cloud key input is present only for Cloud REST, is disabled for a
   read-only/offline client, and no request, toast, log, or response exposes it.
6. Rebuild the embedded `web/dist` output and remeasure browser/binary budgets if
   the bundle size changes.
