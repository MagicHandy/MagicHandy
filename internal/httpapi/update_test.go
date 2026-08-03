package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/updatecheck"
)

func TestUpdateStatusUsesConfiguredGitHubReleaseEndpoint(t *testing.T) {
	t.Parallel()
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("release request omitted User-Agent")
		}
		_, _ = w.Write([]byte(`[{"tag_name":"v1.2.3","name":"MagicHandy 1.2.3"}]`))
	}))
	t.Cleanup(github.Close)

	server := newTestServerWithRuntime(t, Runtime{
		UpdateHTTPClient:    github.Client(),
		UpdateReleaseAPIURL: github.URL,
	})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/update?refresh=1", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", recorder.Header().Get("Cache-Control"))
	}
	var status updatecheck.Status
	if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode update status: %v", err)
	}
	if status.State != updatecheck.StateDevelopment || status.Latest == nil || status.Latest.Version != "1.2.3" {
		t.Fatalf("update status = %+v", status)
	}
}
