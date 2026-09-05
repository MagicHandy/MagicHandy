package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStaticFallbackSeparatesPagesAndMissingAPIEndpoints(t *testing.T) {
	server := newTestServer(t)
	for _, check := range []struct {
		path    string
		status  int
		content string
	}{
		{"/chat", http.StatusOK, "<!doctype html>"},
		{"/api/missing-endpoint", http.StatusNotFound, `"error":"API endpoint not found"`},
		{"/api", http.StatusNotFound, `"error":"API endpoint not found"`},
		{"/missing.js", http.StatusNotFound, "404"},
	} {
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, check.path, nil))
		if response.Code != check.status || !strings.Contains(response.Body.String(), check.content) {
			t.Fatalf("%s: %d %s", check.path, response.Code, response.Body)
		}
	}
}
