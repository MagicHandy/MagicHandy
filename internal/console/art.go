package console

import "strings"

// glyphs holds the characters the console draws. Windows Terminal and other
// ConPTY hosts fall back to fonts that carry stars and bullets; the classic
// console host shows only its own font, where they would appear as boxes, so
// it gets plain ASCII sparkles instead. Block elements and the horizontal
// rule are in every Windows console font.
type glyphs struct {
	star      string
	shaft     string
	twinkle   []string
	running   string
	pending   string
	separator string
	ellipsis  string
	warn      string
	errorMark string
}

var richGlyphs = glyphs{
	star:      "★",
	shaft:     "╱",
	twinkle:   []string{"·", "✧", "✦", "✧", "·", " "},
	running:   "●",
	pending:   "◌",
	separator: " · ",
	ellipsis:  "…",
	warn:      "▲",
	errorMark: "✕",
}

var basicGlyphs = glyphs{
	star:      "*",
	shaft:     "/",
	twinkle:   []string{".", "+", "*", "+", ".", " "},
	running:   "*",
	pending:   "-",
	separator: " - ",
	ellipsis:  "...",
	warn:      "!",
	errorMark: "x",
}

// twinkleColors pairs with glyphs.twinkle: a sparkle brightens as it grows
// and fades as it shrinks.
var twinkleColors = []rgb{colorAccentDeep, colorFocus, colorText, colorFocus, colorAccentDeep, colorBackground}

// wordmarkLetters spell MAGICHANDY two rows high in block elements.
var wordmarkLetters = [][2]string{
	{"█▀▄▀█", "█ ▀ █"},
	{"▄▀█", "█▀█"},
	{"█▀▀", "█▄█"},
	{"█", "█"},
	{"█▀▀", "█▄▄"},
	{"█ █", "█▀█"},
	{"▄▀█", "█▀█"},
	{"█▄ █", "█ ▀█"},
	{"█▀▄", "█▄▀"},
	{"█▄█", " █ "},
}

const (
	wandWidth = 13
	wandRows  = 5
)

// sparkle is one twinkling point around the wand's star. Each starts at its
// own phase, so the points brighten and fade at different moments.
type sparkle struct{ row, col, phase int }

var sparkles = []sparkle{{0, 10, 0}, {0, 5, 3}, {1, 2, 1}, {1, 12, 4}, {2, 11, 2}, {3, 9, 5}}

type wandCell struct {
	glyph string
	color rgb
	bold  bool
}

// wandFrame draws the magic wand for one animation frame: a diagonal shaft
// with a white tip, a star that pulses gently, and sparkles that twinkle.
func wandFrame(g glyphs, frame int) [wandRows]line {
	var grid [wandRows][wandWidth]wandCell
	for row := range grid {
		for col := range grid[row] {
			grid[row][col] = wandCell{glyph: " ", color: colorText}
		}
	}
	grid[2][7] = wandCell{glyph: g.shaft, color: colorText, bold: true}
	grid[3][6] = wandCell{glyph: g.shaft, color: colorAccent}
	grid[4][5] = wandCell{glyph: g.shaft, color: colorAccentDeep}
	starColor := colorText
	if frame%6 >= 3 {
		starColor = colorFocus
	}
	grid[1][8] = wandCell{glyph: g.star, color: starColor, bold: true}
	for _, s := range sparkles {
		step := (frame + s.phase) % len(g.twinkle)
		grid[s.row][s.col] = wandCell{glyph: g.twinkle[step], color: twinkleColors[step], bold: step == 2}
	}
	var rows [wandRows]line
	for row := range grid {
		for _, cell := range grid[row] {
			rows[row] = append(rows[row], span{text: cell.glyph, color: cell.color, bold: cell.bold})
		}
	}
	return rows
}

// wordmark returns the two rows of the MAGICHANDY block wordmark, shaded from
// light azure on the left to deep azure on the right.
func wordmark() [2]line {
	var parts [2][]string
	for _, letter := range wordmarkLetters {
		parts[0] = append(parts[0], letter[0])
		parts[1] = append(parts[1], letter[1])
	}
	var rows [2]line
	for row := range rows {
		runes := []rune(strings.Join(parts[row], " "))
		for i, r := range runes {
			t := float64(i) / float64(len(runes)-1)
			rows[row] = append(rows[row], span{text: string(r), color: mix(colorFocus, colorAccentDeep, t)})
		}
	}
	return rows
}
