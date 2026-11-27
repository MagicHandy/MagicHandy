package elevenlabs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const fixtureKey = "fixture-key-0123456789"

func newTestClient(t *testing.T, handler http.Handler, options Options) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	options.BaseURL = server.URL
	if options.APIKey == "" {
		options.APIKey = fixtureKey
	}
	if options.Sleep == nil {
		options.Sleep = func(context.Context, time.Duration) error { return nil }
	}
	client, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	return client, server
}

func TestNewValidatesConfiguration(t *testing.T) {
	for _, base := range []string{"ftp://x", "relative/path", "https://user:pw@host", "https://host?x=1", "https://host#frag"} {
		if _, err := New(Options{BaseURL: base}); err == nil {
			t.Errorf("base %q should be rejected", base)
		}
	}
	if _, err := New(Options{APIKey: "line\nbreak"}); err == nil {
		t.Error("a key with a line break would allow header injection")
	}
	client, err := New(Options{APIKey: "  padded  "})
	if err != nil || client.baseURL != DefaultBaseURL || client.apiKey != "padded" {
		t.Fatalf("defaults = %+v, %v", client, err)
	}
}

func TestRequestsCarryTheKeyOnlyInItsHeader(t *testing.T) {
	var seen atomic.Int32
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Add(1)
		if r.Header.Get("xi-api-key") != fixtureKey || strings.Contains(r.URL.String(), fixtureKey) {
			t.Errorf("unexpected request: %s %v", r.URL, r.Header)
		}
		_, _ = io.WriteString(w, `[]`)
	}), Options{})
	if _, err := client.Models(t.Context()); err != nil || seen.Load() != 1 {
		t.Fatalf("models: %v (%d calls)", err, seen.Load())
	}
}

func TestTransientFailuresRetryWithBackoffAndRetryAfter(t *testing.T) {
	var calls atomic.Int32
	var waits []time.Duration
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"detail":{"type":"rate_limit_error","code":"rate_limit_exceeded"}}`)
		case 2:
			w.WriteHeader(http.StatusBadGateway)
		default:
			_, _ = io.WriteString(w, `[{"model_id":"eleven_v4","can_do_text_to_speech":true}]`)
		}
	}), Options{Sleep: func(_ context.Context, wait time.Duration) error {
		waits = append(waits, wait)
		return nil
	}})
	models, err := client.Models(t.Context())
	if err != nil || len(models) != 1 || models[0].ID != "eleven_v4" {
		t.Fatalf("models = %+v, %v", models, err)
	}
	if calls.Load() != 3 || len(waits) != 2 || waits[0] != 2*time.Second {
		t.Fatalf("calls = %d, waits = %v", calls.Load(), waits)
	}
}

func TestRetriesStopAtTheAttemptLimit(t *testing.T) {
	var calls atomic.Int32
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}), Options{MaxAttempts: 2})
	_, err := client.Models(t.Context())
	if !IsKind(err, KindUnavailable) || calls.Load() != 2 {
		t.Fatalf("err = %v after %d calls", err, calls.Load())
	}
}

func TestCancellationIsNotAnAPIError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	client, _ := newTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		cancel()
		<-r.Context().Done()
	}), Options{})
	_, err := client.Models(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestNetworkErrorsAreRedacted(t *testing.T) {
	client, err := New(Options{APIKey: fixtureKey, BaseURL: "http://127.0.0.1:1", MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Models(t.Context())
	if !IsKind(err, KindNetwork) || strings.Contains(err.Error(), fixtureKey) {
		t.Fatalf("err = %v", err)
	}
}

func TestOversizedAndMalformedBodiesFailCleanly(t *testing.T) {
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_, _ = io.WriteString(w, `{"not":"a list"}`)
			return
		}
		_, _ = w.Write([]byte(`{"voices":[` + strings.Repeat(" ", maxJSONBodyBytes) + `]}`))
	}), Options{})
	if _, err := client.Models(t.Context()); err == nil {
		t.Fatal("malformed body should fail")
	}
	if _, err := client.Voices(t.Context(), VoiceQuery{}); err == nil {
		t.Fatal("oversized body should fail")
	}
}

func TestStreamSpeechReturnsHeadersAndEndsStalledStreams(t *testing.T) {
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Query().Get("output_format") != "pcm_24000" || body["model_id"] != "eleven_v4" || body["text"] != "Hi." {
			t.Errorf("unexpected request %s %v", r.URL, body)
		}
		if ids, _ := body["previous_request_ids"].([]any); len(ids) != 3 || ids[0] != "b" {
			t.Errorf("previous_request_ids should keep the last three: %v", body["previous_request_ids"])
		}
		w.Header().Set("request-id", "req-9")
		w.Header().Set("character-cost", "3")
		_, _ = w.Write([]byte("PCM"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}), Options{StreamIdleTimeout: 100 * time.Millisecond})
	speech, err := client.StreamSpeech(t.Context(), "voice1", SpeechRequest{
		Text: "Hi.", ModelID: "eleven_v4", OutputFormat: "pcm_24000", PreviousRequestIDs: []string{"a", "b", "c", "d"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = speech.Body.Close() }()
	if speech.RequestID != "req-9" || speech.CharacterCost != 3 {
		t.Fatalf("speech = %+v", speech)
	}
	start := time.Now()
	data, err := io.ReadAll(speech.Body)
	if string(data) != "PCM" || !errors.Is(err, ErrStreamStalled) || time.Since(start) > 3*time.Second {
		t.Fatalf("read %q, err %v after %s", data, err, time.Since(start))
	}
}

func TestRedactRemovesTheKey(t *testing.T) {
	client, err := New(Options{APIKey: fixtureKey})
	if err != nil {
		t.Fatal(err)
	}
	if got := client.Redact("a " + fixtureKey + " b"); strings.Contains(got, fixtureKey) {
		t.Fatalf("redact = %q", got)
	}
	var nilClient *Client
	if nilClient.Redact("x") != "x" {
		t.Fatal("nil client must pass text through")
	}
}
