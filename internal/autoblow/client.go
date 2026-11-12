// Package autoblow is a client for Autoblow's cloud device API (API V1 at
// developers.autoblow.com): the Autoblow Ultra and the VacuGlide. It speaks
// HTTP and server-sent events only. What a command means for motion belongs
// to the transport owners (ADR 0069).
package autoblow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// API families: the path prefix of each device's commands.
const (
	FamilyUltra     = "autoblow"
	FamilyVacuGlide = "vacuglide"
)

// Device types that the connected request reports.
const (
	DeviceTypeUltra     = "autoblow-ultra"
	DeviceTypeVacuGlide = "vacuglide"
	DeviceTypeVacuPump  = "vacupump"
)

const (
	// DefaultLatencyURL answers which cluster a device joined, from any region.
	DefaultLatencyURL = "https://latency.autoblowapi.com"
	clusterSuffix     = ".autoblowapi.com"
	tokenHeader       = "x-device-token"
	maxResponseBytes  = 1 << 20
	maxTokenLength    = 256
)

var (
	// ErrNoToken means no device token was saved.
	ErrNoToken = errors.New("save the device token first")
	// ErrBadToken rejects a token that cannot be a device token.
	ErrBadToken = errors.New("the device token contains characters a token cannot have")
	// ErrOffline means the device is not connected to Autoblow's cloud.
	ErrOffline = errors.New("the device is not online: turn on its online mode")
	// ErrUnauthorized means Autoblow rejected the token.
	ErrUnauthorized = errors.New("autoblow rejected the device token")
	// ErrRateLimited means Autoblow is limiting requests for this token.
	ErrRateLimited = errors.New("autoblow is limiting requests for this device")
	// ErrBadCluster means the lookup named a server outside Autoblow's cloud.
	ErrBadCluster = errors.New("autoblow reported an unexpected server")
	// ErrNotResolved means a device request was made before the lookup.
	ErrNotResolved = errors.New("look up the device's cluster first")
)

// WrongDeviceError means the token belongs to another kind of device.
type WrongDeviceError struct {
	Want, Got string
}

func (e WrongDeviceError) Error() string {
	return fmt.Sprintf("this token belongs to %s, not %s", deviceName(e.Got), deviceName(e.Want))
}

// APIError is a refused request. Message is Autoblow's own description.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("autoblow refused the request (%d): %s", e.Status, e.Message)
	}
	return fmt.Sprintf("autoblow refused the request (%d)", e.Status)
}

// Options configures a client. The zero value reaches Autoblow's cloud.
type Options struct {
	HTTPClient *http.Client
	// LatencyURL replaces the cluster lookup host, for tests and simulation.
	LatencyURL string
	// ClusterURL maps a reported cluster to its base URL. Nil accepts only
	// *.autoblowapi.com clusters over HTTPS, so the token never leaves
	// Autoblow's servers.
	ClusterURL func(cluster string) (string, error)
}

// Connection is the lookup result.
type Connection struct {
	Connected  bool   `json:"connected"`
	Cluster    string `json:"cluster"`
	DeviceType string `json:"deviceType"`
}

// Info describes the device's firmware.
type Info struct {
	FirmwareStatus  string  `json:"firmwareStatus"`
	FirmwareVersion float64 `json:"firmwareVersion"`
	FirmwareBranch  string  `json:"firmwareBranch"`
	HardwareVersion string  `json:"hardwareVersion"`
	DeviceType      string  `json:"deviceType"`
}

// RateLimit is the last limit the cloud reported, when it reports one.
type RateLimit struct {
	Known     bool
	Limit     int
	Remaining int
	Reset     time.Time
}

// Client talks to one device. It is safe for concurrent use.
type Client struct {
	family     string
	token      string
	http       *http.Client
	latency    string
	clusterURL func(string) (string, error)

	mu    sync.Mutex
	base  string
	limit RateLimit
}

// New returns a client for one device token.
func New(family, token string, options Options) (*Client, error) {
	if family != FamilyUltra && family != FamilyVacuGlide {
		return nil, fmt.Errorf("unknown Autoblow device family %q", family)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrNoToken
	}
	if !ValidToken(token) {
		return nil, ErrBadToken
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	// Device headers are not Authorization headers: Go otherwise forwards
	// them across origins on redirects. SSE also carries the token in its URL.
	// Copy the caller's client, preserving its transport without changing it.
	privateClient := *client
	privateClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	latency := strings.TrimRight(strings.TrimSpace(options.LatencyURL), "/")
	if latency == "" {
		latency = DefaultLatencyURL
	}
	clusterURL := options.ClusterURL
	if clusterURL == nil {
		clusterURL = cloudClusterURL
	}
	return &Client{family: family, token: token, http: &privateClient, latency: latency, clusterURL: clusterURL}, nil
}

// ValidToken accepts printable ASCII without spaces, the shape of a device
// token, so a pasted token cannot inject header or query syntax.
func ValidToken(token string) bool {
	if token == "" || len(token) > maxTokenLength {
		return false
	}
	for _, r := range token {
		if r <= ' ' || r > '~' {
			return false
		}
	}
	return true
}

// cloudClusterURL accepts only Autoblow's own clusters.
func cloudClusterURL(cluster string) (string, error) {
	host := strings.ToLower(strings.TrimSpace(cluster))
	if len(host) <= len(clusterSuffix) || !strings.HasSuffix(host, clusterSuffix) || strings.ContainsAny(host, "/:@?#") {
		return "", ErrBadCluster
	}
	for _, r := range strings.TrimSuffix(host, clusterSuffix) {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return "", ErrBadCluster
		}
	}
	return "https://" + host, nil
}

// Family names the device API family.
func (c *Client) Family() string {
	return c.family
}

// Lookup asks which cluster the device joined and binds later requests to it.
// A device that is offline, or a token for another kind of device, is an
// error, so a connected client always reaches the right device.
func (c *Client) Lookup(ctx context.Context) (Connection, error) {
	var connection Connection
	if err := c.do(ctx, http.MethodGet, c.latency+"/"+c.family+"/connected", nil, &connection); err != nil {
		return connection, err
	}
	if !connection.Connected {
		return connection, ErrOffline
	}
	if want := deviceTypeFor(c.family); connection.DeviceType != "" && connection.DeviceType != want {
		return connection, WrongDeviceError{Want: want, Got: connection.DeviceType}
	}
	base, err := c.clusterURL(connection.Cluster)
	if err != nil {
		return connection, err
	}
	c.mu.Lock()
	c.base = strings.TrimRight(base, "/")
	c.mu.Unlock()
	return connection, nil
}

// Info reads the device's firmware.
func (c *Client) Info(ctx context.Context) (Info, error) {
	var info Info
	return info, c.device(ctx, http.MethodGet, "info", nil, &info)
}

// RateLimit reports the last limit the cloud reported.
func (c *Client) RateLimit() RateLimit {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.limit
}

// device sends one request to the device's cluster.
func (c *Client) device(ctx context.Context, method, path string, body, out any) error {
	c.mu.Lock()
	base := c.base
	c.mu.Unlock()
	if base == "" {
		return ErrNotResolved
	}
	return c.do(ctx, method, base+"/"+c.family+"/"+path, body, out)
}

// do sends a request. A request without a body sends no Content-Type, which
// the API requires. Errors never include the URL or the token.
func (c *Client) do(ctx context.Context, method, target string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return errors.New("autoblow request could not be built")
	}
	request.Header.Set(tokenHeader, c.token)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return c.transportError(method, err)
	}
	defer func() { _ = response.Body.Close() }()
	c.recordLimit(response.Header)
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return c.transportError(method, err)
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return c.statusError(response.StatusCode, payload)
	}
	if out == nil || len(bytes.TrimSpace(payload)) == 0 {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return errors.New("autoblow sent a reply MagicHandy could not read")
	}
	return nil
}

func (c *Client) recordLimit(header http.Header) {
	limit, okLimit := headerInt(header, "x-ratelimit-limit")
	remaining, okRemaining := headerInt(header, "x-ratelimit-remaining")
	if !okLimit && !okRemaining {
		return
	}
	next := RateLimit{Known: true, Limit: limit, Remaining: remaining}
	if reset, ok := headerInt(header, "x-ratelimit-reset"); ok {
		// Some servers send an epoch time, others seconds from now.
		if reset > 1_000_000_000 {
			next.Reset = time.Unix(int64(reset), 0)
		} else {
			next.Reset = time.Now().Add(time.Duration(reset) * time.Second)
		}
	}
	c.mu.Lock()
	c.limit = next
	c.mu.Unlock()
}

func headerInt(header http.Header, name string) (int, bool) {
	value := strings.TrimSpace(header.Get(name))
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	return int(parsed), true
}

// transportError keeps the cause but drops the URL, which can carry the token.
func (c *Client) transportError(method string, err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return fmt.Errorf("autoblow %s request failed: %s", strings.ToLower(method), c.redact(err.Error()))
}

func (c *Client) statusError(status int, payload []byte) error {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrUnauthorized
	case http.StatusTooManyRequests:
		return ErrRateLimited
	case http.StatusBadGateway:
		return ErrOffline
	}
	var decoded struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(payload, &decoded)
	message := decoded.Error.Message
	if message == "" {
		message = decoded.Message
	}
	message = c.redact(message)
	if len(message) > 200 {
		message = message[:200]
	}
	return &APIError{Status: status, Code: c.redact(decoded.Error.Code), Message: message}
}

func (c *Client) redact(message string) string {
	for _, secret := range []string{c.token, url.QueryEscape(c.token), url.PathEscape(c.token)} {
		message = strings.ReplaceAll(message, secret, "[redacted]")
	}
	return message
}

func deviceTypeFor(family string) string {
	if family == FamilyVacuGlide {
		return DeviceTypeVacuGlide
	}
	return DeviceTypeUltra
}

func deviceName(deviceType string) string {
	switch deviceType {
	case DeviceTypeUltra:
		return "an Autoblow Ultra"
	case DeviceTypeVacuGlide:
		return "a VacuGlide"
	case DeviceTypeVacuPump:
		return "a VacuPump, which MagicHandy does not support"
	default:
		return "a different device"
	}
}
