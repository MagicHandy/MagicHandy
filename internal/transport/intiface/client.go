// Package intiface implements a Buttplug websocket client for Intiface Central.
package intiface

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const defaultClientName = "magichandy"

// DeviceCapabilities describes one Intiface device entry.
type DeviceCapabilities struct {
	DeviceIndex int    `json:"device_index"`
	DeviceID    string `json:"device_id"`
	Name        string `json:"name"`
	HasLinear   bool   `json:"has_linear"`
	HasVibrate  bool   `json:"has_vibrate"`
	HasRotate   bool   `json:"has_rotate"`
}

type linearItem struct {
	deviceIndex int
	position    float64
	durationMS  int
}

// Client is a Buttplug v3 websocket client for Intiface Central.
type Client struct {
	mu sync.Mutex

	logger          *slog.Logger
	serverURL       string
	clientName      string
	messageGap      time.Duration
	conn            *websocket.Conn
	messageID       int
	devices         []DeviceCapabilities
	selectedIndex   *int
	lastError       string
	reconnecting    bool
	pending         map[int]chan map[string]any
	readerDone      chan struct{}
	linearQueue     chan linearItem
	linearDone      chan struct{}
	lastLinearSent  time.Time
	reconnectCancel context.CancelFunc
}

// ClientOptions configures an Intiface websocket client.
type ClientOptions struct {
	ServerURL  string
	ClientName string
	MessageGap time.Duration
	Logger     *slog.Logger
}

// NewClient returns an Intiface client with defaults matching LSO.
func NewClient(options ClientOptions) *Client {
	url := strings.TrimSpace(options.ServerURL)
	if url == "" {
		url = "ws://127.0.0.1:12345"
	}
	name := strings.TrimSpace(options.ClientName)
	if name == "" {
		name = defaultClientName
	}
	gap := options.MessageGap
	if gap < 0 {
		gap = 0
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		logger:     logger,
		serverURL:  url,
		clientName: name,
		messageGap: gap,
		pending:    make(map[int]chan map[string]any),
	}
}

// ServerURL returns the configured websocket URL.
func (c *Client) ServerURL() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.serverURL
}

// Connected reports whether the websocket is open.
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil
}

// Reconnecting reports whether a background reconnect is active.
func (c *Client) Reconnecting() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reconnecting
}

// LastError returns the last connection or command error.
func (c *Client) LastError() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastError
}

// Devices returns a copy of the last scanned device list.
func (c *Client) Devices() []DeviceCapabilities {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]DeviceCapabilities(nil), c.devices...)
}

// SelectedDeviceID returns the selected device identifier, if any.
func (c *Client) SelectedDeviceID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.selectedIndex == nil || *c.selectedIndex < 0 || *c.selectedIndex >= len(c.devices) {
		return ""
	}
	return c.devices[*c.selectedIndex].DeviceID
}

// SelectedDeviceName returns the selected device name, if any.
func (c *Client) SelectedDeviceName() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.selectedIndex == nil || *c.selectedIndex < 0 || *c.selectedIndex >= len(c.devices) {
		return ""
	}
	return c.devices[*c.selectedIndex].Name
}

// Connect opens the websocket and negotiates server info.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.conn != nil {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	dialer := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 5 * time.Second,
	}
	conn, _, err := dialer.DialContext(ctx, c.serverURL, nil)
	if err != nil {
		c.setLastError(err)
		return err
	}

	c.mu.Lock()
	c.conn = conn
	c.readerDone = make(chan struct{})
	c.linearQueue = make(chan linearItem, 256)
	c.linearDone = make(chan struct{})
	c.lastError = ""
	c.mu.Unlock()

	go c.readerLoop(conn)
	go c.linearWorker()

	if _, err := c.requestServerInfo(ctx); err != nil {
		_ = c.Disconnect(context.Background(), false)
		c.setLastError(err)
		return err
	}
	return nil
}

// Disconnect closes the websocket and optional device selection.
func (c *Client) Disconnect(_ context.Context, clearSelection bool) error {
	c.stopReconnectLoop()
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	if clearSelection {
		c.selectedIndex = nil
		c.devices = nil
	}
	c.failPending(errors.New("not connected to Intiface"))
	c.mu.Unlock()

	if conn != nil {
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(2*time.Second),
		)
		_ = conn.Close()
	}
	c.waitReader()
	c.stopLinearWorker()
	return nil
}

// EnsureConnected reconnects when needed.
func (c *Client) EnsureConnected(ctx context.Context) error {
	if c.Connected() {
		return nil
	}
	return c.Connect(ctx)
}

// Scan requests the current device list from Intiface.
func (c *Client) Scan(ctx context.Context) ([]DeviceCapabilities, error) {
	if err := c.EnsureConnected(ctx); err != nil {
		return nil, err
	}
	devices, err := c.requestDeviceList(ctx)
	if err != nil {
		c.setLastError(err)
		return nil, err
	}
	if len(devices) == 0 {
		_, _ = c.send(ctx, "StartScanning", map[string]any{})
		deadline := time.Now().Add(10 * time.Second)
		for len(devices) == 0 && time.Now().Before(deadline) {
			select {
			case <-ctx.Done():
				_, _ = c.send(context.Background(), "StopScanning", map[string]any{})
				return nil, ctx.Err()
			case <-time.After(500 * time.Millisecond):
				devices, err = c.requestDeviceList(ctx)
				if err != nil {
					_, _ = c.send(context.Background(), "StopScanning", map[string]any{})
					c.setLastError(err)
					return nil, err
				}
				if len(devices) == 0 {
					c.mu.Lock()
					devices = append([]DeviceCapabilities(nil), c.devices...)
					c.mu.Unlock()
				}
			}
		}
		_, _ = c.send(ctx, "StopScanning", map[string]any{})
	}
	c.mu.Lock()
	c.devices = devices
	c.lastError = ""
	c.mu.Unlock()
	return append([]DeviceCapabilities(nil), devices...), nil
}

func (c *Client) requestDeviceList(ctx context.Context) ([]DeviceCapabilities, error) {
	response, err := c.sendAndWait(ctx, "RequestDeviceList", map[string]any{}, "DeviceList")
	if err != nil {
		return nil, err
	}
	return parseDevices(response), nil
}

// SelectDevice chooses a linear-capable device by id or name.
func (c *Client) SelectDevice(deviceID string) error {
	needle := strings.TrimSpace(deviceID)
	if needle == "" {
		return errors.New("device id is required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for index, device := range c.devices {
		if device.DeviceID == needle || device.Name == needle {
			if !device.HasLinear {
				return fmt.Errorf("device %s has no linear actuator", needle)
			}
			selected := index
			c.selectedIndex = &selected
			return nil
		}
	}
	return fmt.Errorf("device not found: %s", needle)
}

// SelectPreferredDevice picks the first linear device matching preferredName.
func (c *Client) SelectPreferredDevice(preferredName string) (string, error) {
	needle := strings.ToLower(strings.TrimSpace(preferredName))
	linear := make([]DeviceCapabilities, 0, len(c.devices))
	for _, device := range c.devices {
		if device.HasLinear {
			linear = append(linear, device)
		}
	}
	if len(linear) == 0 {
		return "", errors.New("no linear devices found")
	}
	for _, device := range linear {
		if needle != "" && strings.Contains(strings.ToLower(device.Name), needle) {
			if err := c.SelectDevice(device.DeviceID); err != nil {
				return "", err
			}
			return device.DeviceID, nil
		}
	}
	if err := c.SelectDevice(linear[0].DeviceID); err != nil {
		return "", err
	}
	return linear[0].DeviceID, nil
}

// MoveTo queues a linear move in 0..1 space.
func (c *Client) MoveTo(ctx context.Context, position float64, durationMS int) error {
	if err := c.EnsureConnected(ctx); err != nil {
		return err
	}
	deviceIndex, err := c.selectedDeviceIndex()
	if err != nil {
		return err
	}
	clamped := clamp(position, 0, 1)
	if durationMS < 1 {
		durationMS = 1
	}
	item := linearItem{
		deviceIndex: deviceIndex,
		position:    clamped,
		durationMS:  durationMS,
	}
	c.mu.Lock()
	queue := c.linearQueue
	c.mu.Unlock()
	if queue == nil {
		return errors.New("not connected to Intiface")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case queue <- item:
		return nil
	}
}

// StopAllDevices sends StopAllDevices when connected.
func (c *Client) StopAllDevices(ctx context.Context) error {
	if !c.Connected() {
		return nil
	}
	_, err := c.send(ctx, "StopAllDevices", map[string]any{})
	if err != nil {
		c.setLastError(err)
	}
	return err
}

// StartReconnectLoop keeps Intiface connected in the background.
func (c *Client) StartReconnectLoop(ctx context.Context, active func() bool) {
	c.stopReconnectLoop()
	loopCtx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.reconnectCancel = cancel
	c.mu.Unlock()

	go func() {
		backoff := 2 * time.Second
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-loopCtx.Done():
				return
			case <-ticker.C:
				if active != nil && !active() {
					c.setReconnecting(false)
					backoff = 2 * time.Second
					continue
				}
				if c.Connected() && c.SelectedDeviceID() != "" {
					c.setReconnecting(false)
					backoff = 2 * time.Second
					continue
				}
				c.setReconnecting(true)
				connectCtx, connectCancel := context.WithTimeout(loopCtx, 8*time.Second)
				err := c.Connect(connectCtx)
				if err == nil {
					scanCtx, scanCancel := context.WithTimeout(loopCtx, 8*time.Second)
					_, scanErr := c.Scan(scanCtx)
					scanCancel()
					if scanErr == nil {
						_, _ = c.SelectPreferredDevice("The Handy")
					}
				}
				connectCancel()
				if err == nil && c.SelectedDeviceID() != "" {
					c.setReconnecting(false)
					backoff = 2 * time.Second
					continue
				}
				timer := time.NewTimer(backoff)
				select {
				case <-loopCtx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				if backoff < 60*time.Second {
					backoff *= 2
				}
			}
		}
	}()
}

func (c *Client) stopReconnectLoop() {
	c.mu.Lock()
	cancel := c.reconnectCancel
	c.reconnectCancel = nil
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (c *Client) selectedDeviceIndex() (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.selectedIndex == nil {
		return 0, errors.New("no device selected")
	}
	if *c.selectedIndex < 0 || *c.selectedIndex >= len(c.devices) {
		return 0, errors.New("invalid selected device index")
	}
	return *c.selectedIndex, nil
}

func (c *Client) requestServerInfo(ctx context.Context) (map[string]any, error) {
	return c.sendAndWait(ctx, "RequestServerInfo", map[string]any{
		"ClientName":     c.clientName,
		"MessageVersion": 3,
	}, "ServerInfo")
}

func (c *Client) send(ctx context.Context, messageType string, body map[string]any) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return 0, errors.New("not connected to Intiface")
	}
	id := c.nextIDLocked()
	envelope := []map[string]any{
		{messageType: mergeBody(body, id)},
	}
	if messageType == "LinearCmd" {
		c.awaitLinearGapLocked()
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return 0, err
	}
	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		c.invalidateConnectionLocked(err)
		return 0, err
	}
	if messageType == "LinearCmd" {
		c.lastLinearSent = time.Now()
	}
	select {
	case <-ctx.Done():
		return id, ctx.Err()
	default:
		return id, nil
	}
}

func (c *Client) sendAndWait(
	ctx context.Context,
	messageType string,
	body map[string]any,
	expected string,
) (map[string]any, error) {
	id, err := c.send(ctx, messageType, body)
	if err != nil {
		return nil, err
	}
	responseCh := make(chan map[string]any, 1)
	c.mu.Lock()
	c.pending[id] = responseCh
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case response := <-responseCh:
		if response == nil {
			return nil, errors.New("connection closed waiting for Intiface response")
		}
		if _, ok := response[expected]; ok {
			return response, nil
		}
		if errValue, ok := response["Error"]; ok {
			return nil, fmt.Errorf("%v", errValue)
		}
		return nil, fmt.Errorf("timed out waiting for %s", expected)
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timed out waiting for %s", expected)
	}
}

func (c *Client) readerLoop(conn *websocket.Conn) {
	defer close(c.readerDone)
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if !isConnectionClosed(err) {
				c.logger.Debug("intiface reader error", "error", err)
			}
			c.invalidateConnection(err)
			return
		}
		for _, message := range parseMessages(raw) {
			c.dispatchIncoming(message)
		}
	}
}

func (c *Client) dispatchIncoming(message map[string]any) {
	if added, ok := message["DeviceAdded"].(map[string]any); ok {
		c.mu.Lock()
		device := deviceFromEntry(len(c.devices), added)
		replaced := false
		for index, existing := range c.devices {
			if existing.DeviceID == device.DeviceID || existing.DeviceIndex == device.DeviceIndex {
				c.devices[index] = device
				replaced = true
				break
			}
		}
		if !replaced {
			c.devices = append(c.devices, device)
		}
		c.mu.Unlock()
		return
	}
	if removedIndex, ok := message["DeviceRemoved"].(float64); ok {
		index := int(removedIndex)
		c.mu.Lock()
		filtered := make([]DeviceCapabilities, 0, len(c.devices))
		for _, device := range c.devices {
			if device.DeviceIndex != index {
				filtered = append(filtered, device)
			}
		}
		c.devices = filtered
		if c.selectedIndex != nil && (*c.selectedIndex == index || *c.selectedIndex >= len(c.devices)) {
			c.selectedIndex = nil
		}
		c.mu.Unlock()
		return
	}
	if errValue, ok := message["Error"]; ok {
		id, _ := message["Id"].(float64)
		c.failResponse(int(id), fmt.Errorf("%v", errValue))
		return
	}
	idValue, ok := message["Id"].(float64)
	if !ok {
		return
	}
	id := int(idValue)
	c.mu.Lock()
	ch := c.pending[id]
	delete(c.pending, id)
	c.mu.Unlock()
	if ch != nil {
		ch <- message
	}
}

func (c *Client) linearWorker() {
	for item := range c.linearQueue {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		payload := map[string]any{
			"DeviceIndex": item.deviceIndex,
			"Vectors": []map[string]any{
				{
					"Index":    0,
					"Position": item.position,
					"Duration": item.durationMS,
				},
			},
		}
		_, err := c.send(ctx, "LinearCmd", payload)
		cancel()
		if err != nil {
			c.logger.Warn("intiface linear command failed", "error", err)
		}
	}
	close(c.linearDone)
}

func (c *Client) stopLinearWorker() {
	c.mu.Lock()
	queue := c.linearQueue
	done := c.linearDone
	c.linearQueue = nil
	c.linearDone = nil
	c.mu.Unlock()
	if queue != nil {
		close(queue)
	}
	if done != nil {
		<-done
	}
}

func (c *Client) waitReader() {
	c.mu.Lock()
	done := c.readerDone
	c.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (c *Client) invalidateConnection(err error) {
	c.mu.Lock()
	c.invalidateConnectionLocked(err)
	c.mu.Unlock()
}

func (c *Client) invalidateConnectionLocked(err error) {
	if err != nil {
		c.lastError = err.Error()
	}
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
	c.failPending(errors.New("not connected to Intiface"))
}

func (c *Client) failPending(err error) {
	pending := c.pending
	c.pending = make(map[int]chan map[string]any)
	for _, ch := range pending {
		select {
		case ch <- nil:
		default:
		}
		_ = err
	}
}

func (c *Client) failResponse(id int, err error) {
	c.mu.Lock()
	ch := c.pending[id]
	delete(c.pending, id)
	c.mu.Unlock()
	if ch != nil {
		select {
		case ch <- map[string]any{"Error": err.Error(), "Id": float64(id)}:
		default:
		}
	}
}

func (c *Client) nextIDLocked() int {
	c.messageID++
	return c.messageID
}

func (c *Client) awaitLinearGapLocked() {
	if c.messageGap <= 0 || c.lastLinearSent.IsZero() {
		return
	}
	elapsed := time.Since(c.lastLinearSent)
	if remaining := c.messageGap - elapsed; remaining > 0 {
		time.Sleep(remaining)
	}
}

func (c *Client) setLastError(err error) {
	if err == nil {
		return
	}
	c.mu.Lock()
	c.lastError = err.Error()
	c.mu.Unlock()
}

func (c *Client) setReconnecting(value bool) {
	c.mu.Lock()
	c.reconnecting = value
	c.mu.Unlock()
}

func mergeBody(body map[string]any, id int) map[string]any {
	merged := make(map[string]any, len(body)+1)
	for key, value := range body {
		merged[key] = value
	}
	merged["Id"] = id
	return merged
}

func clamp(value, lo, hi float64) float64 {
	if value < lo {
		return lo
	}
	if value > hi {
		return hi
	}
	return value
}

func isConnectionClosed(err error) bool {
	if err == nil {
		return false
	}
	if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "close") ||
		strings.Contains(msg, "not connected") ||
		strings.Contains(msg, "use of closed network connection")
}
