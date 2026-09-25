package console

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// rgb is a 24-bit terminal colour from the Steel Azure palette in
// docs/ui-design-guidelines.md. The launch console keeps the app's colour
// roles: azure for anything you can act on, green only for running, amber for
// warnings and red only for Stop.
type rgb struct{ r, g, b uint8 }

var (
	colorBackground = rgb{0x0e, 0x0f, 0x11} // --bg
	colorText       = rgb{0xea, 0xe9, 0xe4} // --text
	colorMuted      = rgb{0x98, 0xa0, 0xa9} // --muted
	colorLine       = rgb{0x3d, 0x43, 0x4c} // --line-strong
	colorAccent     = rgb{0x5b, 0x9d, 0xd9} // --accent
	colorAccentDeep = rgb{0x49, 0x7f, 0xb5} // --accent-strong
	colorFocus      = rgb{0x82, 0xb4, 0xe2} // --focus
	colorOK         = rgb{0x4f, 0xc0, 0x6d} // --ok
	colorWarn       = rgb{0xd8, 0xb6, 0x6a} // --warn
	colorDanger     = rgb{0xd1, 0x3f, 0x45} // --danger
)

func (c rgb) foreground() string {
	return "\x1b[38;2;" + strconv.Itoa(int(c.r)) + ";" + strconv.Itoa(int(c.g)) + ";" + strconv.Itoa(int(c.b)) + "m"
}

func (c rgb) background() string {
	return "\x1b[48;2;" + strconv.Itoa(int(c.r)) + ";" + strconv.Itoa(int(c.g)) + ";" + strconv.Itoa(int(c.b)) + "m"
}

// mix blends two colours; t runs from 0 (a) to 1 (b).
func mix(a, b rgb, t float64) rgb {
	blend := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5) }
	return rgb{blend(a.r, b.r), blend(a.g, b.g), blend(a.b, b.b)}
}

// span is a run of text in one style. A link span is an OSC 8 hyperlink,
// which Windows Terminal and most modern terminals open on Ctrl+click.
type span struct {
	text      string
	color     rgb
	bold      bool
	underline bool
	link      string
}

type line []span

func plain(text string, color rgb) span { return span{text: text, color: color} }

func strong(text string, color rgb) span { return span{text: text, color: color, bold: true} }

// width is the number of terminal columns the line occupies. Every glyph the
// console draws is one column wide.
func (l line) width() int {
	total := 0
	for _, s := range l {
		total += utf8.RuneCountInString(s.text)
	}
	return total
}

// frameStart puts a row on the console background with the base text colour.
// Rows never use a full SGR reset, so the background survives every span.
var frameStart = colorBackground.background() + colorText.foreground()

// render encodes the line for a terminal row of the given width, cutting it
// rather than letting the terminal wrap it onto the next row.
func (l line) render(columns int) string {
	var out strings.Builder
	out.WriteString(frameStart)
	used := 0
	for _, s := range l {
		if used >= columns {
			break
		}
		text := s.text
		if n := utf8.RuneCountInString(text); used+n > columns {
			text = truncateRunes(text, columns-used)
		}
		used += utf8.RuneCountInString(text)
		if s.link != "" {
			out.WriteString("\x1b]8;;" + s.link + "\x1b\\")
		}
		out.WriteString(s.color.foreground())
		if s.bold {
			out.WriteString("\x1b[1m")
		}
		if s.underline {
			out.WriteString("\x1b[4m")
		}
		out.WriteString(text)
		if s.underline {
			out.WriteString("\x1b[24m")
		}
		if s.bold {
			out.WriteString("\x1b[22m")
		}
		if s.link != "" {
			out.WriteString("\x1b]8;;\x1b\\")
		}
	}
	return out.String()
}

func truncateRunes(text string, n int) string {
	if n <= 0 {
		return ""
	}
	count := 0
	for i := range text {
		if count == n {
			return text[:i]
		}
		count++
	}
	return text
}

// fit shortens text to at most n columns, marking the cut with an ellipsis.
func fit(text string, n int, ellipsis string) string {
	if utf8.RuneCountInString(text) <= n {
		return text
	}
	cut := n - utf8.RuneCountInString(ellipsis)
	if cut <= 0 {
		return truncateRunes(text, n)
	}
	return truncateRunes(text, cut) + ellipsis
}

// safeText removes control characters, including escape sequences, from text
// the console did not write itself, such as log values.
func safeText(text string) string {
	if strings.IndexFunc(text, isControl) < 0 {
		return text
	}
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return ' '
		}
		if isControl(r) {
			return -1
		}
		return r
	}, text)
}

func isControl(r rune) bool { return r < 0x20 || (r >= 0x7f && r < 0xa0) }
