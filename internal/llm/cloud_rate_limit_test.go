package llm

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCloudRateLimitsStayDistinctFromQuota(t *testing.T) {
	for _, tc := range []struct {
		status     int
		code, want string
	}{
		{429, "", "rate_limit"}, {0, "rate_limit_exceeded", "rate_limit"},
		{402, "", "quota"}, {429, "insufficient_quota", "quota"},
		{429, "subscription_sharing_usage_limit_exceeded", "quota"},
		{0, "subscription_sharing_usage_unavailable", "quota"},
	} {
		var outcome *CloudError
		err := cloudResponseError(tc.status, tc.code)
		if !errors.As(err, &outcome) || outcome.Kind != tc.want || !outcome.HoldMotion() {
			t.Fatalf("status %d code %q: %v", tc.status, tc.code, err)
		}
	}
}

func TestCompatibleRateLimitUsesOnlySafeErrorCode(t *testing.T) {
	for _, tc := range []struct{ code, want string }{{"", "rate_limit"}, {"insufficient_quota", "quota"}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"code":"` + tc.code + `","message":"private prompt secret-key"}}`))
		}))
		p, err := NewCompatibleProvider(CompatibleOptions{HTTPProviderOptions: HTTPProviderOptions{BaseURL: server.URL, Model: "model"}, Provider: "openrouter"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.request(t.Context(), http.MethodPost, "/chat/completions", nil)
		server.Close()
		var outcome *CloudError
		if !errors.As(err, &outcome) || outcome.Kind != tc.want || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "secret-key") {
			t.Fatalf("unsafe or misleading outcome: %v", err)
		}
	}
}
