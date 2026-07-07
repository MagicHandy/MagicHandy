package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport/intiface"
)

type intifaceRuntime struct {
	client *intiface.Client
	mu     sync.Mutex
}

type deviceTransportRequest struct {
	Transport     string  `json:"transport"`
	ConnectionKey *string `json:"connection_key,omitempty"`
}

type deviceSelectRequest struct {
	DeviceID string `json:"device_id"`
}

type deviceListEntry struct {
	DeviceID  string `json:"device_id"`
	Name      string `json:"name"`
	HasLinear bool   `json:"has_linear"`
}

func newIntifaceRuntime(client *intiface.Client) intifaceRuntime {
	return intifaceRuntime{client: client}
}

func (s *Server) handleDeviceTransportGet(w http.ResponseWriter, _ *http.Request) {
	settings, _ := s.store.Snapshot()
	writeJSON(w, http.StatusOK, s.deviceTransportPayload(settings))
}

func (s *Server) handleDeviceTransportPost(w http.ResponseWriter, r *http.Request) {
	var body deviceTransportRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	current, _ := s.store.Snapshot()
	next := current
	owner, err := mapDeviceTransportToOwner(body.Transport)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	next.Device.HSPDispatchOwner = owner
	if body.ConnectionKey != nil {
		next.Device.HandyConnectionKey = strings.TrimSpace(*body.ConnectionKey)
	}

	saved, err := s.store.Save(next)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("settings could not be saved"))
		return
	}
	s.applySettingsRuntimeTransition(r.Context(), current, next)
	response := map[string]any{
		"ok":                   true,
		"transport":            mapOwnerToDeviceTransport(saved.Device.HSPDispatchOwner),
		"handy_key_configured": saved.Device.HandyConnectionKey != "",
	}
	if saved.Device.HSPDispatchOwner == config.DispatchOwnerIntiface {
		if bootstrap, err := s.bootstrapIntiface(r.Context()); err == nil {
			for key, value := range bootstrap {
				response[key] = value
			}
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleDeviceConnect(w http.ResponseWriter, r *http.Request) {
	result, err := s.bootstrapIntiface(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if result["error"] != nil && result["connected"] != true {
		writeError(w, http.StatusBadGateway, errors.New(stringValue(result["error"])))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"use_mock":     false,
		"connected":    result["connected"],
		"device_label": result["device_label"],
	})
}

func (s *Server) handleDeviceBootstrap(w http.ResponseWriter, r *http.Request) {
	result, err := s.bootstrapIntiface(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if result["error"] != nil && result["connected"] != true {
		writeError(w, http.StatusBadGateway, errors.New(stringValue(result["error"])))
		return
	}
	payload := map[string]any{"ok": true}
	for key, value := range result {
		payload[key] = value
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleDeviceScan(w http.ResponseWriter, r *http.Request) {
	client := s.intifaceClient()
	if client == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("Intiface client is unavailable"))
		return
	}
	devices, err := client.Scan(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"devices": toDeviceListEntries(devices),
	})
}

func (s *Server) handleDeviceSelect(w http.ResponseWriter, r *http.Request) {
	var body deviceSelectRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	client := s.intifaceClient()
	if client == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("Intiface client is unavailable"))
		return
	}
	if err := client.SelectDevice(body.DeviceID); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"device_id": body.DeviceID,
	})
}

func (s *Server) bootstrapIntiface(ctx context.Context) (map[string]any, error) {
	settings, _ := s.store.Snapshot()
	result := map[string]any{
		"use_mock":     false,
		"transport":    mapOwnerToDeviceTransport(settings.Device.HSPDispatchOwner),
		"connected":    false,
		"devices":      []deviceListEntry{},
		"selected":     nil,
		"device_label": nil,
	}
	if settings.Device.HSPDispatchOwner != config.DispatchOwnerIntiface {
		result["error"] = "Intiface dispatch owner is not selected"
		return result, nil
	}
	client := s.intifaceClient()
	if client == nil {
		result["error"] = "Intiface client is unavailable"
		return result, nil
	}
	if err := client.Connect(ctx); err != nil {
		result["error"] = err.Error()
		return result, nil
	}
	result["connected"] = true

	var devices []intiface.DeviceCapabilities
	var scanErr error
	for attempt := 0; attempt < 3; attempt++ {
		devices, scanErr = client.Scan(ctx)
		if scanErr == nil && len(devices) > 0 {
			break
		}
		select {
		case <-ctx.Done():
			scanErr = ctx.Err()
			break
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
	if scanErr != nil {
		result["error"] = scanErr.Error()
		return result, nil
	}
	result["devices"] = toDeviceListEntries(devices)
	if len(devices) == 0 {
		result["error"] = "no Intiface devices found; start scanning in Intiface Central"
		return result, nil
	}
	selected, err := client.SelectPreferredDevice("The Handy")
	if err == nil {
		result["selected"] = selected
		result["device_label"] = client.SelectedDeviceName()
	} else {
		result["error"] = err.Error()
	}
	return result, nil
}

func (s *Server) startIntifaceBootstrapLoop(ctx context.Context) {
	if s.intifaceClient() == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				settings, _ := s.store.Snapshot()
				if settings.Device.HSPDispatchOwner != config.DispatchOwnerIntiface {
					continue
				}
				client := s.intifaceClient()
				if client == nil {
					continue
				}
				if client.Connected() && client.SelectedDeviceID() != "" {
					return
				}
				bootstrapCtx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
				_, _ = s.bootstrapIntiface(bootstrapCtx)
				cancel()
				if client.Connected() && client.SelectedDeviceID() != "" {
					return
				}
			}
		}
	}()
}

func (s *Server) deviceTransportPayload(settings config.Settings) map[string]any {
	client := s.intifaceClient()
	payload := map[string]any{
		"transport":            mapOwnerToDeviceTransport(settings.Device.HSPDispatchOwner),
		"handy_key_configured": settings.Device.HandyConnectionKey != "",
		"intiface_url":         settings.Device.IntifaceURL,
	}
	if client != nil {
		payload["intiface_connected"] = client.Connected()
		payload["intiface_error"] = client.LastError()
		payload["intiface_reconnecting"] = client.Reconnecting()
		payload["device_connected"] = client.Connected() && client.SelectedDeviceID() != ""
		payload["device_label"] = client.SelectedDeviceName()
	}
	return payload
}

func (s *Server) intifaceClient() *intiface.Client {
	s.intiface.mu.Lock()
	defer s.intiface.mu.Unlock()
	return s.intiface.client
}

func (s *Server) ensureIntifaceDeviceForMotion(ctx context.Context) error {
	settings, _ := s.store.Snapshot()
	if settings.Device.HSPDispatchOwner != config.DispatchOwnerIntiface {
		return nil
	}
	client := s.intifaceClient()
	if client == nil {
		return errors.New("Intiface client is unavailable")
	}
	if client.SelectedDeviceID() != "" {
		return nil
	}
	bootstrapCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	result, _ := s.bootstrapIntiface(bootstrapCtx)
	if client.SelectedDeviceID() != "" {
		return nil
	}
	if errText, ok := result["error"].(string); ok && strings.TrimSpace(errText) != "" {
		return errors.New(errText)
	}
	return errors.New("no Intiface device selected")
}

func (s *Server) newIntifaceTransport() (*intiface.Transport, error) {
	settings, _ := s.store.Snapshot()
	if settings.Device.HSPDispatchOwner != config.DispatchOwnerIntiface {
		return nil, errors.New("Intiface dispatch owner is not selected")
	}
	client := s.intifaceClient()
	if client == nil {
		return nil, errors.New("Intiface client is unavailable")
	}
	if !client.Connected() {
		return nil, errors.New("Intiface is not connected")
	}
	if client.SelectedDeviceID() == "" {
		bootstrapCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, _ = s.bootstrapIntiface(bootstrapCtx)
		cancel()
	}
	if client.SelectedDeviceID() == "" {
		return nil, errors.New("No Intiface device selected")
	}
	return intiface.NewTransport(client, intiface.TransportOptions{
		ReverseDirection: settings.Motion.ReverseDirection,
		StrokeMinPercent: settings.Motion.StrokeMinPercent,
		StrokeMaxPercent: settings.Motion.StrokeMaxPercent,
	}), nil
}

func mapDeviceTransportToOwner(transport string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(transport)) {
	case "intiface":
		return config.DispatchOwnerIntiface, nil
	case "handy_cloud", "cloud_rest":
		return config.DispatchOwnerCloudREST, nil
	case "browser_bluetooth":
		return config.DispatchOwnerBrowserBluetooth, nil
	default:
		return "", errors.New("transport must be intiface or handy_cloud")
	}
}

func mapOwnerToDeviceTransport(owner string) string {
	switch owner {
	case config.DispatchOwnerCloudREST:
		return "handy_cloud"
	case config.DispatchOwnerIntiface:
		return "intiface"
	case config.DispatchOwnerBrowserBluetooth:
		return "browser_bluetooth"
	default:
		return owner
	}
}

func toDeviceListEntries(devices []intiface.DeviceCapabilities) []deviceListEntry {
	entries := make([]deviceListEntry, 0, len(devices))
	for _, device := range devices {
		entries = append(entries, deviceListEntry{
			DeviceID:  device.DeviceID,
			Name:      device.Name,
			HasLinear: device.HasLinear,
		})
	}
	return entries
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
