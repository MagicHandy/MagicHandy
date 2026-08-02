package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestVersionComparison(t *testing.T) {
	t.Parallel()
	tests := []struct {
		left  string
		right string
		want  int
	}{
		{left: "1.2.3", right: "1.2.3", want: 0},
		{left: "v1.3.0", right: "1.2.9", want: 1},
		{left: "1.0.0-rc.2", right: "1.0.0-rc.10", want: -1},
		{left: "1.0.0-999999999999999999999999", right: "1.0.0-1000000000000000000000000", want: -1},
		{left: "1.0.0-rc.1", right: "1.0.0", want: -1},
		{left: "2.0.0+build.4", right: "2.0.0+build.9", want: 0},
	}
	for _, test := range tests {
		left, leftOK := parseVersion(test.left)
		right, rightOK := parseVersion(test.right)
		if !leftOK || !rightOK {
			t.Fatalf("parseVersion(%q, %q) failed", test.left, test.right)
		}
		if got := left.Compare(right); got != test.want {
			t.Errorf("%q.Compare(%q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
	for _, invalid := range []string{"", "dev", "1", "1.2", "01.2.3", "1.2.3-", "1.2.3-01", "1.2.3_bad", "1.2.3+", "1.2.3+bad_value"} {
		if _, ok := parseVersion(invalid); ok {
			t.Errorf("parseVersion(%q) unexpectedly succeeded", invalid)
		}
	}
}

func TestCheckerCachesAndRevalidatesLatestRelease(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Accept") != "application/vnd.github+json" || r.Header.Get("X-GitHub-Api-Version") == "" {
			t.Error("GitHub API headers were not sent")
		}
		w.Header().Set("ETag", `"release-1"`)
		if r.Header.Get("If-None-Match") == `"release-1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"v1.4.0","name":"MagicHandy 1.4","published_at":"2026-08-01T12:00:00Z"}`))
	}))
	t.Cleanup(server.Close)

	now := time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC)
	checker := New(Options{
		CurrentVersion: "1.3.0",
		Endpoint:       server.URL,
		HTTPClient:     server.Client(),
		Clock:          func() time.Time { return now },
	})

	first := checker.Check(context.Background(), false)
	if first.State != StateAvailable || first.Latest == nil || first.Latest.Version != "1.4.0" {
		t.Fatalf("first status = %+v", first)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests after first check = %d, want 1", got)
	}
	second := checker.Check(context.Background(), false)
	if second.State != StateAvailable || requests.Load() != 1 {
		t.Fatalf("cached check = %+v, requests = %d", second, requests.Load())
	}
	now = now.Add(time.Minute)
	refreshed := checker.Check(context.Background(), true)
	if refreshed.State != StateAvailable || refreshed.CheckedAt != now || refreshed.Stale {
		t.Fatalf("revalidated status = %+v", refreshed)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests after refresh = %d, want 2", got)
	}
}

func TestCheckerHandlesDevelopmentNoReleaseAndFailure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		status  int
		body    string
		current string
		want    State
	}{
		{name: "development", status: http.StatusOK, body: `{"tag_name":"v1.0.0"}`, current: "dev", want: StateDevelopment},
		{name: "no release", status: http.StatusNotFound, current: "1.0.0", want: StateNoRelease},
		{name: "rate limited", status: http.StatusForbidden, current: "1.0.0", want: StateError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			t.Cleanup(server.Close)
			status := New(Options{CurrentVersion: test.current, Endpoint: server.URL, HTTPClient: server.Client()}).Check(context.Background(), false)
			if status.State != test.want {
				t.Fatalf("state = %q, want %q (%+v)", status.State, test.want, status)
			}
		})
	}
}

func TestFailedRefreshRetainsLastResult(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		if fail.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"v2.0.0"}`))
	}))
	t.Cleanup(server.Close)
	checker := New(Options{CurrentVersion: "1.0.0", Endpoint: server.URL, HTTPClient: server.Client()})
	if got := checker.Check(context.Background(), false); got.State != StateAvailable {
		t.Fatalf("initial state = %+v", got)
	}
	fail.Store(true)
	got := checker.Check(context.Background(), true)
	if got.State != StateAvailable || !got.Stale || got.Message == "" {
		t.Fatalf("failed refresh = %+v", got)
	}
	if repeated := checker.Check(context.Background(), false); !repeated.Stale || requests.Load() != 2 {
		t.Fatalf("automatic retry was not throttled: status=%+v requests=%d", repeated, requests.Load())
	}
}

func TestInitialFailureIsTemporarilyCached(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	now := time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)
	checker := New(Options{
		CurrentVersion: "1.0.0",
		Endpoint:       server.URL,
		HTTPClient:     server.Client(),
		Clock:          func() time.Time { return now },
	})

	if got := checker.Check(context.Background(), false); got.State != StateError {
		t.Fatalf("initial state = %+v", got)
	}
	if got := checker.Check(context.Background(), false); got.State != StateError || requests.Load() != 1 {
		t.Fatalf("cached failure = %+v, requests = %d", got, requests.Load())
	}
	now = now.Add(defaultFailureTTL + time.Second)
	_ = checker.Check(context.Background(), false)
	if requests.Load() != 2 {
		t.Fatalf("requests after retry window = %d, want 2", requests.Load())
	}
}
