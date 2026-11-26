// Package elevenlabs is a small, dependency-free client for the parts of the
// ElevenLabs API MagicHandy uses: streaming text to speech (from the TTS
// worker) and the account, model and voice catalogs (from the core, for the
// Settings voice picker).
//
// The API key is a private credential. It is sent only in the xi-api-key
// header and never appears in errors, logs, or values this package returns.
package elevenlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// DefaultBaseURL is the public ElevenLabs API.
const DefaultBaseURL = "https://api.elevenlabs.io"

const (
	defaultMaxAttempts       = 3
	defaultCatalogTimeout    = 20 * time.Second
	defaultStreamIdleTimeout = 20 * time.Second
	maxErrorBodyBytes        = 64 << 10
	maxJSONBodyBytes         = 8 << 20
	maxBackoff               = 4 * time.Second
	maxRetryAfter            = 10 * time.Second
)

// ErrStreamStalled reports a speech stream that stopped delivering audio.
var ErrStreamStalled = errors.New("ElevenLabs stopped sending audio")

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// Options configure a Client.
type Options struct {
	// BaseURL defaults to DefaultBaseURL. Tests point it at a local server.
	BaseURL string
	// APIKey is the private credential; empty is allowed for public endpoints.
	APIKey string
	// HTTPClient overrides the default transport. It should not set an
	// overall Timeout, which would cut long speech streams short.
	HTTPClient *http.Client
	// UserAgent is sent when set; otherwise Go's default is used.
	UserAgent string
	// MaxAttempts bounds retries of transient failures (default 3).
	MaxAttempts int
	// CatalogTimeout bounds each JSON catalog call (default 20s).
	CatalogTimeout time.Duration
	// StreamIdleTimeout ends a speech stream that delivers no bytes for this
	// long (default 20s).
	StreamIdleTimeout time.Duration
	// Sleep waits between retries; tests replace it.
	Sleep func(context.Context, time.Duration) error
}

// Client calls the ElevenLabs API. It is safe for concurrent use.
type Client struct {
	baseURL        string
	apiKey         string
	http           *http.Client
	userAgent      string
	maxAttempts    int
	catalogTimeout time.Duration
	idleTimeout    time.Duration
	sleep          func(context.Context, time.Duration) error
}

// New validates options and returns a client.
func New(options Options) (*Client, error) {
	base := strings.TrimRight(strings.TrimSpace(options.BaseURL), "/")
	if base == "" {
		base = DefaultBaseURL
	}
	if err := ValidateBaseURL(base); err != nil {
		return nil, err
	}
	if strings.ContainsAny(options.APIKey, "\r\n") {
		return nil, errors.New("the ElevenLabs API key contains a line break")
	}
	client := &Client{
		baseURL:        base,
		apiKey:         strings.TrimSpace(options.APIKey),
		http:           options.HTTPClient,
		userAgent:      options.UserAgent,
		maxAttempts:    options.MaxAttempts,
		catalogTimeout: options.CatalogTimeout,
		idleTimeout:    options.StreamIdleTimeout,
		sleep:          options.Sleep,
	}
	if client.http == nil {
		client.http = NewHTTPClient()
	}
	if client.maxAttempts <= 0 {
		client.maxAttempts = defaultMaxAttempts
	}
	if client.catalogTimeout <= 0 {
		client.catalogTimeout = defaultCatalogTimeout
	}
	if client.idleTimeout <= 0 {
		client.idleTimeout = defaultStreamIdleTimeout
	}
	if client.sleep == nil {
		client.sleep = sleepContext
	}
	return client, nil
}

// NewHTTPClient returns a client with connection-phase timeouts and no
// overall deadline, so a long speech stream is bounded by its idle timeout
// and the caller's context instead of a fixed total duration.
func NewHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     true,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			ExpectContinueTimeout: time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConnsPerHost:   4,
		},
		// Never follow a redirect: it would re-send the key to another URL.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// ValidateBaseURL accepts an absolute http(s) URL without credentials, query
// or fragment.
func ValidateBaseURL(base string) error {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" || !parsed.IsAbs() {
		return errors.New("the ElevenLabs base URL must be an absolute HTTP URL with a host")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("the ElevenLabs base URL scheme must be http or https")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return errors.New("the ElevenLabs base URL must not contain credentials, a query, or a fragment")
	}
	return nil
}

// ValidIdentifier reports whether value looks like an ElevenLabs voice, model
// or owner ID. IDs are interpolated into URL paths, so this is a hard gate.
func ValidIdentifier(value string) bool {
	return identifierPattern.MatchString(value)
}

// HasKey reports whether the client carries an API key.
func (c *Client) HasKey() bool { return c.apiKey != "" }

// Redact removes the API key from text that might reach a log or message.
func (c *Client) Redact(text string) string {
	if c == nil || c.apiKey == "" {
		return text
	}
	return strings.ReplaceAll(text, c.apiKey, "[redacted]")
}

type attempt struct {
	response *http.Response
	cancel   context.CancelFunc
}

// send performs one logical call, retrying transient failures. A retry is only
// safe before any response body has been consumed, which holds here: failures
// are detected from the status line, and successful bodies are returned.
func (c *Client) send(ctx context.Context, operation, method, path string, query url.Values, body []byte, accept string) (attempt, error) {
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	for try := 1; ; try++ {
		attemptCtx, cancel := context.WithCancel(ctx)
		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(body)
		}
		request, err := http.NewRequestWithContext(attemptCtx, method, target, reader)
		if err != nil {
			cancel()
			return attempt{}, fmt.Errorf("build ElevenLabs request: %s", c.Redact(err.Error()))
		}
		if c.apiKey != "" {
			request.Header.Set("xi-api-key", c.apiKey)
		}
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		if accept != "" {
			request.Header.Set("Accept", accept)
		}
		if c.userAgent != "" {
			request.Header.Set("User-Agent", c.userAgent)
		}

		response, err := c.http.Do(request)
		var failure *APIError
		switch {
		case err != nil:
			cancel()
			if ctx.Err() != nil {
				return attempt{}, ctx.Err()
			}
			failure = networkError(operation, errors.New(c.Redact(err.Error())))
		case response.StatusCode >= 200 && response.StatusCode < 300:
			return attempt{response: response, cancel: cancel}, nil
		default:
			detail, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBodyBytes))
			_ = response.Body.Close()
			cancel()
			failure = parseAPIError(operation, response.StatusCode, response.Header, detail)
			failure.Message = c.Redact(failure.Message)
		}
		if !failure.Retryable() || try >= c.maxAttempts {
			return attempt{}, failure
		}
		if err := c.sleep(ctx, backoff(try, failure.RetryAfter)); err != nil {
			return attempt{}, err
		}
	}
}

// doJSON performs one bounded JSON call and decodes the result into out.
func (c *Client) doJSON(ctx context.Context, operation, method, path string, query url.Values, body []byte, out any) error {
	ctx, cancel := context.WithTimeout(ctx, c.catalogTimeout)
	defer cancel()
	result, err := c.send(ctx, operation, method, path, query, body, "application/json")
	if err != nil {
		return err
	}
	defer result.cancel()
	defer func() { _ = result.response.Body.Close() }()
	limited := io.LimitReader(result.response.Body, maxJSONBodyBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return networkError(operation, errors.New(c.Redact(err.Error())))
	}
	if len(data) > maxJSONBodyBytes {
		return &APIError{Operation: operation, kind: KindInvalid, Message: "the response was larger than MagicHandy accepts"}
	}
	if err := json.Unmarshal(data, out); err != nil {
		return &APIError{Operation: operation, kind: KindUnknown, Message: "the response was not in the expected format"}
	}
	return nil
}

func backoff(try int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return min(retryAfter, maxRetryAfter)
	}
	base := 400 * time.Millisecond << (try - 1)
	if base > maxBackoff {
		base = maxBackoff
	}
	// Full jitter keeps simultaneous clients from retrying in lockstep.
	// #nosec G404 -- retry jitter, not a security decision.
	return base/2 + time.Duration(rand.Int64N(int64(base/2)+1))
}

func sleepContext(ctx context.Context, wait time.Duration) error {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// idleBody ends a stream that stops delivering bytes. The timer only cancels
// the request context; it never touches the body concurrently with Read.
type idleBody struct {
	body     io.ReadCloser
	timer    *time.Timer
	timeout  time.Duration
	cancel   context.CancelFunc
	timedOut *atomic.Bool
}

func newIdleBody(body io.ReadCloser, timeout time.Duration, cancel context.CancelFunc) *idleBody {
	timedOut := &atomic.Bool{}
	timer := time.AfterFunc(timeout, func() {
		timedOut.Store(true)
		cancel()
	})
	return &idleBody{body: body, timer: timer, timeout: timeout, cancel: cancel, timedOut: timedOut}
}

func (b *idleBody) Read(p []byte) (int, error) {
	n, err := b.body.Read(p)
	if n > 0 && !b.timedOut.Load() {
		b.timer.Reset(b.timeout)
	}
	if err != nil && !errors.Is(err, io.EOF) && b.timedOut.Load() {
		return n, ErrStreamStalled
	}
	return n, err
}

func (b *idleBody) Close() error {
	b.timer.Stop()
	err := b.body.Close()
	b.cancel()
	return err
}
