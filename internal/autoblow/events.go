package autoblow

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Button events. While a device is online its buttons stop acting on it and
// arrive here instead, so the app decides what each one does.
const (
	EventConnected = "connected"
	EventHeartbeat = "heartbeat"
	// The Ultra's buttons.
	ButtonUltraPause     = "pause-button-pressed"
	ButtonUltraMode      = "mode-button-pressed"
	ButtonUltraSpeedUp   = "speed-up-button-pressed"
	ButtonUltraSpeedDown = "speed-down-button-pressed"
	// The VacuGlide's buttons.
	ButtonVacuGlideMode       = "mode-button-pressed"
	ButtonVacuGlideSpeedPlus  = "speed-plus-button-pressed"
	ButtonVacuGlideSpeedMinus = "speed-minus-button-pressed"
)

const maxEventLine = 64 << 10

// Event is one server-sent event by name. Its data can carry the device token,
// so it is never kept or logged.
type Event struct {
	Name string
}

// EventStream reads one device's server-sent events.
type EventStream struct {
	body   io.ReadCloser
	reader *bufio.Reader
}

// Events opens the device's event stream on its cluster. The stream takes the
// token in its query, so the request URL must never be logged; errors from
// this package never include it. ctx bounds the stream's life.
func (c *Client) Events(ctx context.Context) (*EventStream, error) {
	c.mu.Lock()
	base := c.base
	c.mu.Unlock()
	if base == "" {
		return nil, ErrNotResolved
	}
	target := base + "/events/stream?deviceToken=" + url.QueryEscape(c.token)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, errors.New("autoblow event request could not be built")
	}
	request.Header.Set("Accept", "text/event-stream")
	response, err := c.http.Do(request)
	if err != nil {
		return nil, c.transportError(http.MethodGet, err)
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		payload, _ := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
		_ = response.Body.Close()
		return nil, c.statusError(response.StatusCode, payload)
	}
	return &EventStream{body: response.Body, reader: bufio.NewReaderSize(response.Body, 4096)}, nil
}

// Next blocks until the next event. A heartbeat is an event too, so a reader
// can tell a quiet stream from a dead one.
func (s *EventStream) Next() (Event, error) {
	name, seen := "", false
	for {
		line, err := s.line()
		if err != nil {
			return Event{}, err
		}
		if line == "" {
			if seen {
				if name == "" {
					name = "message"
				}
				return Event{Name: name}, nil
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			name, seen = value, true
		case "data", "id":
			seen = true
		}
	}
}

// line reads one line, discarding the excess of an oversized one.
func (s *EventStream) line() (string, error) {
	var collected []byte
	for {
		chunk, err := s.reader.ReadSlice('\n')
		if len(collected) < maxEventLine {
			collected = append(collected, chunk[:min(len(chunk), maxEventLine-len(collected))]...)
		}
		switch {
		case err == nil:
			return strings.TrimRight(string(collected), "\r\n"), nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		default:
			return "", err
		}
	}
}

// Close ends the stream.
func (s *EventStream) Close() error {
	return s.body.Close()
}
