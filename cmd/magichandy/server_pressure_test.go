package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPServerRejectsOversizedHeadersBeforeApplicationWork(t *testing.T) {
	var calls atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	})
	host := httptest.NewUnstartedServer(handler)
	host.Config = newHTTPServer(host.Listener.Addr().String(), handler, nil)
	host.Start()
	defer host.Close()
	client := host.Client()
	client.Timeout = 2 * time.Second
	for _, test := range []struct{ bytes, status int }{{1024, http.StatusNoContent}, {64 << 10, http.StatusRequestHeaderFieldsTooLarge}} {
		r, err := http.NewRequestWithContext(t.Context(), http.MethodGet, host.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("X-Pressure-Test", strings.Repeat("x", test.bytes))
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != test.status {
			t.Fatalf("%d header bytes: status=%d", test.bytes, response.StatusCode)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("oversized headers entered application work")
	}
}
