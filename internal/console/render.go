package console

import (
	"log/slog"
	"strings"
	"time"
)

type phase int

const (
	phaseStarting phase = iota
	phaseRunning
	phaseStopping
)

// entry is one line of recent activity. Verbose entries, such as every HTTP
// request, show only while details are on.
type entry struct {
	at      time.Time
	level   slog.Level
	message string
	detail  string
	verbose bool
}

// view is everything one frame shows.
type view struct {
	version     string
	url         string
	access      string
	simulated   bool
	phase       phase
	since       time.Time
	entries     []entry
	details     bool
	canCopy     bool
	notice      string
	noticeColor rgb
	frame       int
	glyphs      glyphs
	clickable   bool
	columns     int
	rows        int
}

const (
	margin       = "  "
	fullColumns  = 64
	fullRows     = 20
	tagline      = "Local-first control for The Handy"
	closeWarning = "Closing this window stops the app."
)

// compose lays out one frame, row by row, sized to the terminal.
func compose(v view) []line {
	var rows []line
	rows = append(rows, header(v)...)
	rows = append(rows, line{})
	rows = append(rows, status(v)...)
	rows = append(rows, line{})
	footer := footer(v)
	room := v.rows - len(rows) - len(footer) - 1
	rows = append(rows, activityHeading(v))
	entries := v.entries
	if room < 0 {
		room = 0
	}
	if len(entries) > room {
		entries = entries[len(entries)-room:]
	}
	for _, e := range entries {
		rows = append(rows, activityLine(v, e))
	}
	for len(rows) < v.rows-len(footer) {
		rows = append(rows, line{})
	}
	rows = append(rows, footer...)
	if len(rows) > v.rows && v.rows > 0 {
		rows = rows[len(rows)-v.rows:]
	}
	return rows
}

// header is the magic wand beside the MAGICHANDY wordmark, or a one-line
// title when the window is too small for it.
func header(v view) []line {
	if v.columns < fullColumns || v.rows < fullRows {
		step := v.frame % len(v.glyphs.twinkle)
		return []line{
			{},
			{plain(margin, colorText), span{text: v.glyphs.star, color: colorText, bold: true},
				span{text: v.glyphs.twinkle[step], color: twinkleColors[step]}, plain(" ", colorText),
				strong("MagicHandy", colorAccent), plain(v.glyphs.separator+versionText(v.version), colorMuted)},
		}
	}
	wand := wandFrame(v.glyphs, v.frame)
	mark := wordmark()
	beside := [wandRows]line{
		nil,
		mark[0],
		mark[1],
		{plain(tagline, colorMuted)},
		{plain(versionText(v.version), colorMuted)},
	}
	rows := []line{{}}
	for i := range wand {
		row := line{plain(margin, colorText)}
		row = append(row, wand[i]...)
		row = append(row, plain("   ", colorText))
		row = append(row, beside[i]...)
		rows = append(rows, row)
	}
	return rows
}

// versionText names the build; source builds without a release version say
// so plainly.
func versionText(version string) string {
	if version == "" || version == "dev" {
		return "Development build"
	}
	return "Version " + version
}

func status(v view) []line {
	var state line
	switch v.phase {
	case phaseRunning:
		state = line{plain(margin, colorText), strong(v.glyphs.running+" Running", colorOK)}
		if !v.since.IsZero() {
			state = append(state, plain(" since "+v.since.Format("15:04"), colorMuted))
		}
	case phaseStopping:
		state = line{plain(margin, colorText), plain(v.glyphs.pending+" Shutting down", colorMuted)}
	default:
		state = line{plain(margin, colorText), plain(v.glyphs.pending+" Starting", colorMuted)}
	}
	rows := []line{state}
	if v.url != "" {
		rows = append(rows, line{plain(margin+"  Open    ", colorMuted),
			span{text: v.url, color: colorAccent, underline: true, link: v.url}})
	}
	if v.access != "" {
		rows = append(rows, line{plain(margin+"  Access  ", colorMuted), plain(v.access, colorText)})
	}
	if v.simulated {
		rows = append(rows, line{plain(margin+"  Motion  ", colorMuted), plain("Simulated; no device moves", colorText)})
	}
	return rows
}

func activityHeading(v view) line {
	title := "Recent activity"
	if v.details {
		title = "Recent activity, including every request"
	}
	row := line{plain(margin, colorText), strong(title, colorText), plain(" ", colorText)}
	if rule := v.columns - row.width() - len(margin); rule > 0 {
		row = append(row, plain(strings.Repeat("─", rule), colorLine))
	}
	return row
}

func activityLine(v view, e entry) line {
	row := line{plain(margin, colorText), plain(e.at.Format("15:04:05")+"  ", colorMuted)}
	message := e.message
	color := colorText
	switch {
	case e.level >= slog.LevelError:
		row = append(row, strong(v.glyphs.errorMark+" ", colorWarn))
		color = colorWarn
	case e.level >= slog.LevelWarn:
		row = append(row, plain(v.glyphs.warn+" ", colorWarn))
		color = colorWarn
	case e.verbose:
		color = colorMuted
	}
	room := v.columns - row.width() - len(margin)
	row = append(row, span{text: fit(message, room, v.glyphs.ellipsis), color: color, bold: e.level >= slog.LevelError})
	if e.detail != "" {
		if room = v.columns - row.width() - len(margin) - 3; room > 4 {
			row = append(row, plain("   "+fit(e.detail, room, v.glyphs.ellipsis), colorMuted))
		}
	}
	return row
}

type key struct {
	letter, label, short string
	color                rgb
}

func footer(v view) []line {
	keys := []key{{"O", "Open in browser", "Open", colorAccent}}
	if v.canCopy {
		keys = append(keys, key{"C", "Copy link", "Copy", colorAccent})
	}
	keys = append(keys, key{"S", "Stop motion", "Stop", colorDanger}, key{"D", "Details", "Details", colorAccent}, key{"Q", "Quit", "Quit", colorAccent})
	keyRow := keyLine(keys, false)
	if keyRow.width() > v.columns-len(margin) {
		keyRow = keyLine(keys, true)
	}
	rule := v.columns - 2*len(margin)
	if rule < 0 {
		rule = 0
	}
	hint := line{plain(margin, colorText)}
	switch {
	case v.notice != "":
		hint = append(hint, plain(v.notice, v.noticeColor))
	case v.clickable:
		hint = append(hint, plain("Ctrl+click the link or press O to open MagicHandy. "+closeWarning, colorMuted))
	default:
		hint = append(hint, plain("Press O to open MagicHandy in your browser. "+closeWarning, colorMuted))
	}
	return []line{
		{plain(margin, colorText), plain(strings.Repeat("─", rule), colorLine)},
		keyRow,
		hint,
	}
}

func keyLine(keys []key, compact bool) line {
	row := line{plain(margin, colorText)}
	for i, k := range keys {
		if i > 0 {
			gap := "   "
			if compact {
				gap = "  "
			}
			row = append(row, plain(gap, colorText))
		}
		label := k.label
		if compact {
			label = k.short
		}
		row = append(row, plain("[", colorMuted), strong(k.letter, k.color), plain("] ", colorMuted), plain(label, colorText))
	}
	return row
}
