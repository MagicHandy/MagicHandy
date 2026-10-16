// Package buttplugsim is a minimal in-process Buttplug message spec v3
// server for -simulate-motion review builds. It advertises one simulated
// stroker and one simulated vibrator, accepts every command and never
// touches a radio. Nothing here is used outside simulation.
package buttplugsim

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// pingMillis is the advertised keepalive. A nonzero value exercises the same
// watchdog path as a configured Intiface Central.
const pingMillis = 1000

// Server is one simulated Intiface Central endpoint on loopback.
type Server struct {
	listener net.Listener
	server   *http.Server
	mu       sync.Mutex
	conns    map[*websocket.Conn]struct{}
}

// Start listens on an ephemeral loopback port.
func Start() (*Server, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	sim := &Server{listener: listener, conns: map[*websocket.Conn]struct{}{}}
	sim.server = &http.Server{Handler: http.HandlerFunc(sim.serve), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = sim.server.Serve(listener) }()
	return sim, nil
}

// Address is the ws:// URL clients connect to.
func (s *Server) Address() string {
	return "ws://" + s.listener.Addr().String()
}

// Close stops the server and drops every client.
func (s *Server) Close() error {
	s.mu.Lock()
	for conn := range s.conns {
		_ = conn.CloseNow()
	}
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.conns[conn] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
		_ = conn.CloseNow()
	}()
	for {
		_, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		replies, err := respond(data)
		if err != nil {
			return
		}
		writeCtx, cancel := context.WithTimeout(r.Context(), time.Second)
		err = conn.Write(writeCtx, websocket.MessageText, replies)
		cancel()
		if err != nil {
			return
		}
	}
}

// respond answers one frame of client messages in order.
func respond(data []byte) ([]byte, error) {
	var envelopes []map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelopes); err != nil {
		return nil, err
	}
	var replies []map[string]any
	for _, envelope := range envelopes {
		for kind, raw := range envelope {
			var header struct {
				ID uint32 `json:"Id"`
			}
			if err := json.Unmarshal(raw, &header); err != nil {
				return nil, err
			}
			replies = append(replies, reply(kind, header.ID)...)
		}
	}
	if len(replies) == 0 {
		return nil, errors.New("empty Buttplug frame")
	}
	return json.Marshal(replies)
}

func reply(kind string, id uint32) []map[string]any {
	switch kind {
	case "RequestServerInfo":
		return []map[string]any{{"ServerInfo": map[string]any{"Id": id, "ServerName": "MagicHandy simulator", "MessageVersion": 3, "MaxPingTime": pingMillis}}}
	case "RequestDeviceList":
		return []map[string]any{{"DeviceList": map[string]any{"Id": id, "Devices": devices()}}}
	case "StartScanning":
		return []map[string]any{{"Ok": map[string]any{"Id": id}}, {"ScanningFinished": map[string]any{"Id": 0}}}
	default:
		return []map[string]any{{"Ok": map[string]any{"Id": id}}}
	}
}

func devices() []map[string]any {
	return []map[string]any{
		{
			"DeviceIndex": 0, "DeviceName": "Simulated Stroker", "DeviceMessageTimingGap": 0,
			"DeviceMessages": map[string]any{
				"LinearCmd":     []map[string]any{{"FeatureDescriptor": "Position", "ActuatorType": "Position", "StepCount": 1000}},
				"StopDeviceCmd": map[string]any{},
			},
		},
		{
			"DeviceIndex": 1, "DeviceName": "Simulated Vibrator", "DeviceMessageTimingGap": 0,
			"DeviceMessages": map[string]any{
				"ScalarCmd": []map[string]any{
					{"FeatureDescriptor": "Internal motor", "ActuatorType": "Vibrate", "StepCount": 20},
					{"FeatureDescriptor": "External motor", "ActuatorType": "Vibrate", "StepCount": 20},
				},
				"StopDeviceCmd": map[string]any{},
			},
		},
	}
}
