// Package console draws MagicHandy's launch console: the window that opens
// with the app on Windows. It shows the brand, where to open the app, recent
// activity in plain language, and a few keys. It never commands motion; its
// Stop key calls the action the caller supplies, which is the app's own
// emergency-stop path.
package console

import "errors"

// ErrNotInteractive reports that input or output is redirected, or that the
// console cannot draw colour and move the cursor. Callers then keep plain
// structured logs.
var ErrNotInteractive = errors.New("console: not an interactive terminal")

// Terminal is the text console the dashboard draws in.
type Terminal interface {
	Write(p []byte) (int, error)
	// Size reports the visible columns and rows.
	Size() (columns, rows int)
	// ReadKey blocks until a key is pressed. A lone Escape is '\x1b'.
	ReadKey() (rune, error)
	// Restore returns input and output to the modes they had before Open.
	Restore() error
	// RichGlyphs reports whether the host renders stars and bullets that
	// console fonts lack. Such hosts also open links on Ctrl+click.
	RichGlyphs() bool
	// SoleOwner reports whether this process alone uses the window, as when
	// it was started from a shortcut; the window then closes when it exits.
	SoleOwner() bool
}
