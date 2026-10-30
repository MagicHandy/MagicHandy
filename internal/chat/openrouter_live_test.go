//go:build liveeval

package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// openRouterLiveProvider runs the hosted suites through the production
// OpenRouter adapter with a saved connection's defaults: automatic structured
// output, no fallbacks and data collection denied. The evaluation key is read
// from the environment for each request and never logged or saved.
// MAGICHANDY_LIVE_OPENROUTER_ONLY optionally pins comma-separated providers and
// MAGICHANDY_LIVE_OPENROUTER_SORT sets a connection's host order.
// MAGICHANDY_LIVE_OPENROUTER_TRACE appends one JSON line per completion with
// the status, serving provider and any error code, never headers or messages.
func openRouterLiveProvider(t *testing.T) (llm.Provider, string, bool) {
	t.Helper()
	if os.Getenv("MAGICHANDY_LIVE_OPENROUTER_KEY") == "" {
		return nil, "", false
	}
	model := os.Getenv("MAGICHANDY_LIVE_MODEL")
	if model == "" {
		t.Skip("set MAGICHANDY_LIVE_MODEL to an OpenRouter model ID")
	}
	effort := os.Getenv("MAGICHANDY_LIVE_REASONING")
	if effort == "" {
		effort = "none"
	}
	if effort == "default" {
		effort = ""
	}
	var only []string
	for _, name := range strings.Split(os.Getenv("MAGICHANDY_LIVE_OPENROUTER_ONLY"), ",") {
		if name = strings.TrimSpace(name); name != "" {
			only = append(only, name)
		}
	}
	client := &http.Client{Transport: openRouterTrace{path: os.Getenv("MAGICHANDY_LIVE_OPENROUTER_TRACE"), next: http.DefaultTransport}}
	provider, err := llm.NewCompatibleProvider(llm.CompatibleOptions{
		HTTPProviderOptions: llm.HTTPProviderOptions{BaseURL: "https://openrouter.ai/api/v1", Model: model, ContextSize: 65536, Timeout: 60 * time.Second, Client: client},
		Provider:            "openrouter", ReasoningEffort: effort, OutputMode: "auto", AllowedProviders: only, DataCollection: "deny",
		ProviderSort: os.Getenv("MAGICHANDY_LIVE_OPENROUTER_SORT"),
		Token: func(context.Context) (string, error) {
			if key := os.Getenv("MAGICHANDY_LIVE_OPENROUTER_KEY"); key != "" {
				return key, nil
			}
			return "", errors.New("the OpenRouter evaluation key is unavailable")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return provider, model, true
}

type openRouterTrace struct {
	path string
	next http.RoundTripper
}

var openRouterTraceMu sync.Mutex

func (o openRouterTrace) RoundTrip(request *http.Request) (*http.Response, error) {
	started := time.Now()
	response, err := o.next.RoundTrip(request)
	if o.path == "" || !strings.HasSuffix(request.URL.Path, "/chat/completions") {
		return response, err
	}
	record := map[string]any{"at": started.Format(time.RFC3339Nano), "test": os.Getenv("MAGICHANDY_EXPERIMENT_CAPTURE")}
	if err != nil {
		record["transport_error"] = true
		o.write(record)
		return response, err
	}
	record["status"] = response.StatusCode
	response.Body = &openRouterTraceBody{ReadCloser: response.Body, trace: o, record: record, started: started}
	return response, nil
}

func (o openRouterTrace) write(record map[string]any) {
	encoded, err := json.Marshal(record)
	if err != nil {
		return
	}
	openRouterTraceMu.Lock()
	defer openRouterTraceMu.Unlock()
	file, err := os.OpenFile(o.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	_, _ = file.Write(append(encoded, '\n'))
}

// The first bytes carry the serving provider and generation ID; an upstream
// error can also arrive as a streamed chunk after a 200 status.
type openRouterTraceBody struct {
	io.ReadCloser
	trace   openRouterTrace
	record  map[string]any
	started time.Time
	head    bytes.Buffer
	once    sync.Once
}

func (b *openRouterTraceBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if b.head.Len() < 1<<18 {
		b.head.Write(p[:min(n, 1<<18-b.head.Len())])
	}
	if err != nil {
		b.finish()
	}
	return n, err
}

func (b *openRouterTraceBody) Close() error {
	b.finish()
	return b.ReadCloser.Close()
}

var (
	openRouterTraceProvider = regexp.MustCompile(`"provider"\s*:\s*"([^"]+)"`)
	openRouterTraceID       = regexp.MustCompile(`"id"\s*:\s*"(gen-[^"]+)"`)
	openRouterTraceFinish   = regexp.MustCompile(`"finish_reason"\s*:\s*"([^"]+)"`)
	openRouterTraceUsage    = regexp.MustCompile(`"usage"\s*:\s*(\{[^{}]*(\{[^{}]*\}[^{}]*)*\})`)
)

func (b *openRouterTraceBody) finish() {
	b.once.Do(func() {
		head := b.head.String()
		b.record["elapsed_ms"] = time.Since(b.started).Milliseconds()
		if match := openRouterTraceProvider.FindStringSubmatch(head); match != nil {
			b.record["provider"] = match[1]
		}
		if match := openRouterTraceID.FindStringSubmatch(head); match != nil {
			b.record["generation"] = match[1]
		}
		if code, present := openRouterTraceErrorCode(head); present {
			b.record["error_code"] = code
		}
		if matches := openRouterTraceFinish.FindAllStringSubmatch(head, -1); len(matches) > 0 {
			b.record["finish_reason"] = matches[len(matches)-1][1]
		}
		if matches := openRouterTraceUsage.FindAllStringSubmatch(head, -1); len(matches) > 0 {
			b.record["usage"] = matches[len(matches)-1][1]
		}
		b.trace.write(b.record)
	})
}

// Provider error prose can echo credentials or conversation text. Retain only
// a numeric status code, including for errors arriving after HTTP 200.
func openRouterTraceErrorCode(body string) (any, bool) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "data:"))
		var packet struct {
			Error *struct {
				Code json.RawMessage `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(line), &packet) != nil || packet.Error == nil {
			continue
		}
		var code int
		if json.Unmarshal(packet.Error.Code, &code) == nil && code >= 100 && code <= 599 {
			return code, true
		}
		return "unknown", true
	}
	return nil, false
}

func TestOpenRouterTraceKeepsErrorTextPrivate(t *testing.T) {
	for _, body := range []string{
		`{"error":{"message":"private conversation and secret-key","code":429}}`,
		"data: {\"error\":{\"code\":429,\"message\":\"private conversation and secret-key\"}}\n\n",
	} {
		if code, present := openRouterTraceErrorCode(body); !present || code != 429 {
			t.Fatalf("lost status-only error: %v, %t", code, present)
		}
	}
	if code, present := openRouterTraceErrorCode(`{"error":{"code":"secret-key","message":"private conversation"}}`); !present || code != "unknown" {
		t.Fatalf("unsafe error code retained: %v, %t", code, present)
	}
}
