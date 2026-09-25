package console

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// requestMessage is the per-request log line. It is shown only with details
// on, because the browser polls the app several times a second.
const requestMessage = "http request"

// Handler returns a slog handler that shows log records as recent activity.
// Handle never blocks on the terminal.
func (d *Dashboard) Handler(level slog.Leveler) slog.Handler {
	return &activityHandler{dashboard: d, level: level}
}

type activityHandler struct {
	dashboard *Dashboard
	level     slog.Leveler
	attrs     []slog.Attr
	prefix    string
}

func (h *activityHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *activityHandler) Handle(_ context.Context, record slog.Record) error {
	fields := make([][2]string, 0, len(h.attrs)+record.NumAttrs())
	for _, attr := range h.attrs {
		fields = appendAttr(fields, "", attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		fields = appendAttr(fields, h.prefix, attr)
		return true
	})
	e := entry{at: record.Time, level: record.Level}
	if e.at.IsZero() {
		e.at = time.Now()
	}
	if record.Message == requestMessage {
		e.verbose = true
		e.message = requestSummary(fields)
	} else {
		e.message = sentence(safeText(record.Message))
		e.detail = joinFields(fields)
	}
	h.dashboard.post(func(s *dashState) { s.add(e) })
	return nil
}

func (h *activityHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = append(append([]slog.Attr(nil), h.attrs...), prefixed(h.prefix, attrs)...)
	return &next
}

func (h *activityHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := *h
	next.prefix = h.prefix + name + "."
	return &next
}

func prefixed(prefix string, attrs []slog.Attr) []slog.Attr {
	if prefix == "" {
		return attrs
	}
	out := make([]slog.Attr, len(attrs))
	for i, attr := range attrs {
		out[i] = slog.Attr{Key: prefix + attr.Key, Value: attr.Value}
	}
	return out
}

func appendAttr(fields [][2]string, prefix string, attr slog.Attr) [][2]string {
	value := attr.Value.Resolve()
	if value.Kind() == slog.KindGroup {
		name := prefix
		if attr.Key != "" {
			name += attr.Key + "."
		}
		for _, member := range value.Group() {
			fields = appendAttr(fields, name, member)
		}
		return fields
	}
	if attr.Key == "" {
		return fields
	}
	return append(fields, [2]string{prefix + attr.Key, safeText(value.String())})
}

func joinFields(fields [][2]string) string {
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		if field[1] == "" {
			continue
		}
		parts = append(parts, field[0]+"="+field[1])
	}
	return strings.Join(parts, "  ")
}

// requestSummary turns a request log into "GET /api/state 200 (3 ms)".
func requestSummary(fields [][2]string) string {
	values := map[string]string{}
	for _, field := range fields {
		values[field[0]] = field[1]
	}
	summary := strings.TrimSpace(values["method"] + " " + values["path"] + " " + values["status"])
	if ms := values["duration_ms"]; ms != "" {
		summary += " (" + ms + " ms)"
	}
	return summary
}

// sentence capitalises a log message, so "server starting" reads "Server
// starting".
func sentence(message string) string {
	first, size := utf8.DecodeRuneInString(message)
	if first == utf8.RuneError {
		return message
	}
	return string(unicode.ToUpper(first)) + message[size:]
}
