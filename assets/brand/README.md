# Brand assets

The one place for MagicHandy's brand artwork outside the web app. See
[ADR 0073](../../docs/decisions/0073-brand-mark-and-palettes.md) for the mark
and [ADR 0074](../../docs/decisions/0074-windows-icon-and-installer-artwork.md)
for how Windows uses these files.

| File | Used by |
| --- | --- |
| `magichandy.ico` | The executable's icon and the installer's icon |
| `installer-side.png`, `installer-side-2x.png` | The tall panel on the installer's first and last pages |
| `installer-corner.png`, `installer-corner-2x.png` | The mark in the installer's header |
| `mark.svg` | Master drawing of the full mark on its tile |
| `star.svg` | Master drawing of the small-size glyph |

All of them use the default Steel Azure palette: tile `#1d2024`, hand
`#e9e7e2`, star and streak `#5b9dd9`. An icon file cannot follow the palette
saved in the app, so it stays on the default.

## The icon

`magichandy.ico` holds ten sizes. The 32 px and larger images show the full
mark. The 16, 20 and 24 px images show the star alone, because the hand is not
legible that small.

The executable gets the icon from
`cmd/magichandy/resource_windows_amd64.syso`, a resource object the Go linker
picks up by its file name. After replacing the icon, rebuild that object from
the repository root:

```
go run ./cmd/windows-resource
```

`internal/winresource` has a test that fails when the committed object and the
committed icon no longer match.

## The drawing

The geometry in `mark.svg` and `star.svg` is the same geometry the web app
draws from `web/src/brand/paths.ts`. Change them together.
