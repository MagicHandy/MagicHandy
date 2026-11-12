package autoblow

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCredentialsNeverFollowRedirects(t *testing.T) {
	for _, path := range []string{"/vacuglide/connected", "/vacuglide/info", "/events/stream"} {
		t.Run(path, func(t *testing.T) {
			var leaked atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				leaked.Add(1)
				w.WriteHeader(http.StatusTeapot)
			}))
			defer target.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == path {
					http.Redirect(w, r, target.URL+"?deviceToken="+url.QueryEscape("private-token"), http.StatusTemporaryRedirect)
					return
				}
				_, _ = w.Write([]byte(`{"connected":true,"cluster":"test","deviceType":"vacuglide"}`))
			}))
			defer server.Close()
			client, err := New(FamilyVacuGlide, "private-token", Options{LatencyURL: server.URL,
				HTTPClient: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return nil }},
				ClusterURL: func(string) (string, error) { return server.URL, nil },
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Lookup(context.Background())
			if path != "/vacuglide/connected" {
				if err != nil {
					t.Fatal(err)
				}
				if path == "/events/stream" {
					stream, streamErr := client.Events(context.Background())
					err = streamErr
					if stream != nil {
						_ = stream.Close()
					}
				} else {
					_, err = client.Info(context.Background())
				}
			}
			if err == nil || leaked.Load() != 0 {
				t.Fatalf("redirect: error=%v, destination requests=%d", err, leaked.Load())
			}
		})
	}
}

func TestProviderErrorsCannotEchoDeviceCredentials(t *testing.T) {
	const secret = "private-token/+?"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{
			"code": secret, "message": "rejected " + secret + " and " + url.QueryEscape(secret),
		}})
	}))
	defer server.Close()
	client, err := New(FamilyUltra, secret, Options{LatencyURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Lookup(context.Background())
	if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), url.QueryEscape(secret)) {
		t.Fatalf("provider response was not safely redacted: %v", err)
	}
}
