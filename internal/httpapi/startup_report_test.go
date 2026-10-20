package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/startup"
)

func TestStartupReportsRequireAdministratorAndNoController(t *testing.T) {
	s, _, _, adminCookie := newControllerSessionFixture(t)
	recorder := startup.NewRecorder(s.store.DataDir(), "original-version", "original-commit")
	recorder.Report.Stage = "bind_app"
	if _, err := recorder.Finish(errors.New("private key must not enter diagnostics")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/startup", "/api/startup/report"} {
		request := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
		denied := httptest.NewRecorder()
		s.Handler().ServeHTTP(denied, request)
		if denied.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s: %d", path, denied.Code)
		}
		request.AddCookie(adminCookie)
		allowed := httptest.NewRecorder()
		s.Handler().ServeHTTP(allowed, request)
		if allowed.Code != http.StatusOK || !strings.Contains(allowed.Body.String(), "original-version") || strings.Contains(allowed.Body.String(), "private key") {
			t.Fatalf("admin report %s: %d %s", path, allowed.Code, allowed.Body.String())
		}
		if allowed.Header().Get("Cache-Control") != "no-store" || allowed.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("missing report response protections")
		}
		if strings.HasSuffix(path, "/report") && !strings.Contains(allowed.Header().Get("Content-Disposition"), "magichandy-startup-report.json") {
			t.Fatal("missing report attachment")
		}
	}
}

func TestBootstrapLocalStartupReportHasNoPrivateInstanceData(t *testing.T) {
	s := newTestServer(t)
	instance, err := startup.Acquire(s.store.DataDir())
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()
	if err := instance.Publish("http://127.0.0.1:49717", "http://127.0.0.1:49718"); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://localhost/api/startup/report", nil)
	request.RemoteAddr = "127.0.0.1:40000"
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "token") || strings.Contains(response.Body.String(), "49717") {
		t.Fatalf("report exposed private instance record: %d %s", response.Code, response.Body.String())
	}
}
