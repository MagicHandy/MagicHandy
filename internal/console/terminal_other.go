//go:build !windows

package console

// Open reports ErrNotInteractive outside Windows, where MagicHandy keeps its
// structured logs; the launch console is for the window Windows opens with
// the app.
func Open() (Terminal, error) { return nil, ErrNotInteractive }
