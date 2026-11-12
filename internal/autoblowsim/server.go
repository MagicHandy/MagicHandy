// Package autoblowsim serves an in-process imitation of Autoblow's cloud API
// for -simulate-motion builds and tests: one simulated Autoblow Ultra and one
// simulated VacuGlide that accept any token and move nothing. Like the
// Intiface simulator, it exists so review builds never reach a real device.
package autoblowsim

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	familyUltra     = "autoblow"
	familyVacuGlide = "vacuglide"
	// ultraFullSpeed is the Ultra's travel at 100% speed, in percent per second.
	ultraFullSpeed = 400.0
)

// Request is one command the simulator accepted, for tests.
type Request struct {
	Family string
	Path   string
	Body   map[string]any
	At     time.Time
}

// Options tune a simulator for tests.
type Options struct {
	// Heartbeat is the event stream's heartbeat interval; zero means 30 s.
	Heartbeat time.Duration
}

// Server is a running simulator.
type Server struct {
	listener  net.Listener
	server    *http.Server
	heartbeat time.Duration

	mu          sync.Mutex
	closed      bool
	offline     map[string]bool
	failures    map[string][]int
	tokens      map[string]string
	requests    []Request
	ultra       ultraDevice
	vacuglide   vacuglideDevice
	subscribers map[string]map[chan string]struct{}
}

type ultraDevice struct {
	mode      string
	position  float64
	target    float64
	speed     float64
	movedAt   time.Time
	oscillate [3]float64
}

type vacuglideDevice struct {
	mode       string
	speed      float64
	plusOpen   bool
	minusOpen  bool
	strokeAirs int
}

// Start serves a simulator on a loopback port until Close.
func Start() (*Server, error) {
	return StartWith(Options{})
}

// StartWith starts a simulator with test options.
func StartWith(options Options) (*Server, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	heartbeat := options.Heartbeat
	if heartbeat <= 0 {
		heartbeat = 30 * time.Second
	}
	s := &Server{
		listener:    listener,
		heartbeat:   heartbeat,
		offline:     map[string]bool{},
		failures:    map[string][]int{},
		tokens:      map[string]string{},
		ultra:       ultraDevice{mode: "ONLINE_CONNECTED", position: 50, target: 50},
		vacuglide:   vacuglideDevice{mode: "ONLINE_CONNECTED"},
		subscribers: map[string]map[chan string]struct{}{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{family}/connected", s.connected)
	mux.HandleFunc("GET /{family}/info", s.info)
	mux.HandleFunc("GET /{family}/state", s.state)
	mux.HandleFunc("PUT /autoblow/{command...}", s.ultraCommand)
	mux.HandleFunc("PUT /vacuglide/{command...}", s.vacuglideCommand)
	mux.HandleFunc("GET /events/stream", s.events)
	s.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = s.server.Serve(listener) }()
	return s, nil
}

// URL is the simulator's base URL, for both the lookup and the cluster.
func (s *Server) URL() string {
	return "http://" + s.listener.Addr().String()
}

// ClusterURL maps any reported cluster to this simulator.
func (s *Server) ClusterURL(string) (string, error) {
	return s.URL(), nil
}

// Close stops the simulator and ends every event stream.
func (s *Server) Close() error {
	s.mu.Lock()
	s.closed = true
	for _, subscribers := range s.subscribers {
		for channel := range subscribers {
			close(channel)
		}
	}
	s.subscribers = map[string]map[chan string]struct{}{}
	s.mu.Unlock()
	return s.server.Close()
}

// SetOnline turns a simulated device's online mode on or off.
func (s *Server) SetOnline(family string, online bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.offline[family] = !online
}

// FailNext makes the next device commands of family answer with status.
func (s *Server) FailNext(family string, status int, count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for range count {
		s.failures[family] = append(s.failures[family], status)
	}
}

// Press sends a button event to every stream of the device family.
func (s *Server) Press(family, event string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for channel := range s.subscribers[family] {
		select {
		case channel <- event:
		default:
		}
	}
}

// Requests returns the commands accepted so far.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

// UltraPosition is the simulated Ultra's position now.
func (s *Server) UltraPosition() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.advanceUltraLocked(time.Now())
	return s.ultra.position
}

// VacuGlideSpeed is the simulated VacuGlide's motor speed now.
func (s *Server) VacuGlideSpeed() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.vacuglide.speed
}

// VacuGlideValves reports whether each valve is open and how many times
// stroke plus opened, net of stroke minus.
func (s *Server) VacuGlideValves() (plusOpen, minusOpen bool, net int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.vacuglide.plusOpen, s.vacuglide.minusOpen, s.vacuglide.strokeAirs
}

func (s *Server) connected(w http.ResponseWriter, r *http.Request) {
	family := r.PathValue("family")
	token, ok := requireToken(w, r)
	if !ok || !knownFamily(w, family) {
		return
	}
	s.mu.Lock()
	online := !s.offline[family]
	if online {
		s.tokens[token] = family
	}
	s.mu.Unlock()
	if !online {
		writeJSON(w, http.StatusOK, map[string]any{"connected": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connected": true, "cluster": s.listener.Addr().String(), "deviceType": deviceType(family)})
}

func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	family := r.PathValue("family")
	if _, ok := requireToken(w, r); !ok || !knownFamily(w, family) || !s.reachable(w, family) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"firmwareStatus": "UP_TO_DATE", "firmwareVersion": 1.01, "firmwareBranch": "prod",
		"hardwareVersion": strings.TrimPrefix(deviceType(family), "autoblow-"), "mac": "000000000000", "deviceType": deviceType(family)})
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	family := r.PathValue("family")
	if _, ok := requireToken(w, r); !ok || !knownFamily(w, family) || !s.reachable(w, family) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if family == familyUltra {
		writeJSON(w, http.StatusOK, s.ultraStateLocked())
		return
	}
	writeJSON(w, http.StatusOK, s.vacuglideStateLocked())
}

func (s *Server) ultraCommand(w http.ResponseWriter, r *http.Request) {
	command := r.PathValue("command")
	body, ok := s.admit(w, r, familyUltra, command)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.advanceUltraLocked(now)
	switch command {
	case "goto":
		position, speed, valid := number(body, "position"), number(body, "speed"), has(body, "position", "speed")
		if !valid || position < 0 || position > 100 || speed < 0 || speed > 100 {
			writeAPIError(w, http.StatusBadRequest, "position and speed must be 0 to 100")
			return
		}
		s.ultra.mode, s.ultra.target, s.ultra.speed, s.ultra.movedAt = "GO_TO", position, speed, now
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
	case "oscillate":
		speed, low, high := number(body, "speed"), number(body, "minY"), number(body, "maxY")
		if !has(body, "speed", "minY", "maxY") || high-low < 10 || low < 0 || high > 100 {
			writeAPIError(w, http.StatusBadRequest, "minY and maxY must be at least 10 apart")
			return
		}
		s.ultra.mode, s.ultra.oscillate = "OSCILLATOR_PLAYING", [3]float64{speed, low, high}
		writeJSON(w, http.StatusOK, s.ultraStateLocked())
	case "oscillate/start":
		s.ultra.mode = "OSCILLATOR_PLAYING"
		writeJSON(w, http.StatusOK, s.ultraStateLocked())
	case "oscillate/stop":
		s.ultra.mode, s.ultra.target = "OSCILLATOR_PAUSED", s.ultra.position
		writeJSON(w, http.StatusOK, s.ultraStateLocked())
	default:
		writeAPIError(w, http.StatusNotFound, "unknown command")
	}
}

func (s *Server) vacuglideCommand(w http.ResponseWriter, r *http.Request) {
	command := r.PathValue("command")
	body, ok := s.admit(w, r, familyVacuGlide, command)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch command {
	case "target-speed":
		speed := number(body, "targetSpeed")
		if !has(body, "targetSpeed") || speed < 0 || speed > 100 {
			writeAPIError(w, http.StatusBadRequest, "targetSpeed must be 0 to 100")
			return
		}
		s.vacuglide.mode, s.vacuglide.speed = "TARGET_SPEED_PLAYING", speed
	case "target-speed/stop":
		s.vacuglide.mode, s.vacuglide.speed = "TARGET_SPEED_PAUSED", 0
	case "valve/stroke-plus", "valve/stroke-minus":
		open, valid := body["valveState"].(bool)
		if !valid {
			writeAPIError(w, http.StatusBadRequest, "valveState must be true or false")
			return
		}
		if command == "valve/stroke-plus" {
			s.vacuglide.plusOpen = open
			if open {
				s.vacuglide.strokeAirs++
			}
		} else {
			s.vacuglide.minusOpen = open
			if open {
				s.vacuglide.strokeAirs--
			}
		}
	default:
		writeAPIError(w, http.StatusNotFound, "unknown command")
		return
	}
	writeJSON(w, http.StatusOK, s.vacuglideStateLocked())
}

// admit checks the token, online mode, injected failures and the body rules:
// a command with a body sends JSON, and one without sends no Content-Type.
func (s *Server) admit(w http.ResponseWriter, r *http.Request, family, command string) (map[string]any, bool) {
	if _, ok := requireToken(w, r); !ok || !s.reachable(w, family) {
		return nil, false
	}
	payload, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "unreadable body")
		return nil, false
	}
	body := map[string]any{}
	contentType := r.Header.Get("Content-Type")
	switch {
	case len(payload) == 0 && contentType != "":
		writeAPIError(w, http.StatusBadRequest, "a request without a body must not send a Content-Type")
		return nil, false
	case len(payload) > 0 && !strings.HasPrefix(contentType, "application/json"):
		writeAPIError(w, http.StatusBadRequest, "a body must be JSON")
		return nil, false
	case len(payload) > 0:
		if err := json.Unmarshal(payload, &body); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid JSON")
			return nil, false
		}
	}
	s.mu.Lock()
	s.requests = append(s.requests, Request{Family: family, Path: command, Body: body, At: time.Now()})
	if len(s.requests) > 4096 {
		s.requests = s.requests[len(s.requests)-4096:]
	}
	s.mu.Unlock()
	return body, true
}

// reachable answers 502 for a device that left online mode, and any injected
// failure.
func (s *Server) reachable(w http.ResponseWriter, family string) bool {
	s.mu.Lock()
	offline := s.offline[family]
	status := 0
	if queue := s.failures[family]; len(queue) > 0 {
		status, s.failures[family] = queue[0], queue[1:]
	}
	s.mu.Unlock()
	switch {
	case offline:
		writeAPIError(w, http.StatusBadGateway, "device is not connected")
		return false
	case status != 0:
		writeAPIError(w, status, "injected failure")
		return false
	}
	return true
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("deviceToken")
	s.mu.Lock()
	family, known := s.tokens[token]
	closed := s.closed
	s.mu.Unlock()
	if token == "" || !known || closed {
		writeAPIError(w, http.StatusUnauthorized, "unknown device token")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	channel := make(chan string, 8)
	s.mu.Lock()
	if s.subscribers[family] == nil {
		s.subscribers[family] = map[chan string]struct{}{}
	}
	s.subscribers[family][channel] = struct{}{}
	s.mu.Unlock()
	defer s.unsubscribe(family, channel)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "event: connected\ndata: {\"connectionId\":\"simulated\"}\n\n")
	flusher.Flush()
	ticker := time.NewTicker(s.heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			_, _ = fmt.Fprintf(w, "event: heartbeat\ndata: {\"timestamp\":%d}\n\n", time.Now().UnixMilli())
		case event, open := <-channel:
			if !open {
				return
			}
			_, _ = fmt.Fprintf(w, "event: %s\ndata: {\"payload\":{\"type\":%q}}\n\n", event, event)
		}
		flusher.Flush()
	}
}

func (s *Server) unsubscribe(family string, channel chan string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subscribers[family][channel]; ok {
		delete(s.subscribers[family], channel)
		close(channel)
	}
}

// advanceUltraLocked moves the simulated slider toward its target.
func (s *Server) advanceUltraLocked(now time.Time) {
	if s.ultra.mode != "GO_TO" || s.ultra.movedAt.IsZero() {
		return
	}
	step := ultraFullSpeed * s.ultra.speed / 100 * now.Sub(s.ultra.movedAt).Seconds()
	delta := s.ultra.target - s.ultra.position
	if math.Abs(delta) <= step {
		s.ultra.position = s.ultra.target
	} else {
		s.ultra.position += math.Copysign(step, delta)
	}
	s.ultra.movedAt = now
}

func (s *Server) ultraStateLocked() map[string]any {
	return map[string]any{
		"operationalMode": s.ultra.mode, "localScript": 0, "localScriptSpeed": 0, "motorTemperature": 30,
		"oscillatorTargetSpeed": s.ultra.oscillate[0], "oscillatorLowPoint": s.ultra.oscillate[1], "oscillatorHighPoint": s.ultra.oscillate[2],
		"syncScriptCurrentTime": 0, "syncScriptOffsetTime": 0, "syncScriptToken": "", "syncScriptLoop": false,
	}
}

func (s *Server) vacuglideStateLocked() map[string]any {
	return map[string]any{
		"operationalMode": s.vacuglide.mode, "localScript": 0, "targetSpeed": s.vacuglide.speed,
		"strokePlusValve": s.vacuglide.plusOpen, "strokeMinusValve": s.vacuglide.minusOpen,
		"syncScriptCurrentTime": 0, "syncScriptOffsetTime": 0, "syncScriptToken": "", "syncScriptLoop": false,
	}
}

func requireToken(w http.ResponseWriter, r *http.Request) (string, bool) {
	token := strings.TrimSpace(r.Header.Get("x-device-token"))
	if token == "" {
		writeAPIError(w, http.StatusUnauthorized, "missing device token")
		return "", false
	}
	return token, true
}

func knownFamily(w http.ResponseWriter, family string) bool {
	if family != familyUltra && family != familyVacuGlide {
		writeAPIError(w, http.StatusNotFound, "unknown device family")
		return false
	}
	return true
}

func deviceType(family string) string {
	if family == familyVacuGlide {
		return "vacuglide"
	}
	return "autoblow-ultra"
}

func number(body map[string]any, key string) float64 {
	value, _ := body[key].(float64)
	return value
}

func has(body map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := body[key].(float64); !ok {
			return false
		}
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": "Error", "message": message}})
}
