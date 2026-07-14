package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostPathPickerReturnsValidatedLocalSelection(t *testing.T) {
	server := newTestServer(t)
	selected := filepath.Join(t.TempDir(), "stream_pcm.exe")
	if err := os.WriteFile(selected, []byte("runner"), 0o600); err != nil {
		t.Fatal(err)
	}
	server.hostPathPicker = func(_ context.Context, spec hostPathPickerSpec, current string) (string, bool, error) {
		if spec.Directory || !strings.Contains(spec.Filter, "*.exe") || current != `C:\existing\stream_pcm.exe` {
			t.Fatalf("picker input = %+v, %q", spec, current)
		}
		return selected, false, nil
	}
	request := withController(httptest.NewRequest(http.MethodPost, "/api/host/path-picker", strings.NewReader(`{"kind":"executable","current":"C:\\existing\\stream_pcm.exe"}`)))
	prepareLocalPathPickerRequest(request)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), filepath.Base(selected)) || !strings.Contains(recorder.Body.String(), `"canceled":false`) {
		t.Fatalf("picker response = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestHostPathPickerRejectsRemoteClients(t *testing.T) {
	server := newTestServer(t)
	called := false
	server.hostPathPicker = func(context.Context, hostPathPickerSpec, string) (string, bool, error) {
		called = true
		return "", true, nil
	}
	request := withController(httptest.NewRequest(http.MethodPost, "/api/host/path-picker", strings.NewReader(`{"kind":"file","current":""}`)))
	request.Header.Set("Content-Type", "application/json")
	request.Host = "127.0.0.1:49717"
	request.RemoteAddr = "192.0.2.10:54321"
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || called {
		t.Fatalf("remote picker response = %d, called=%t: %s", recorder.Code, called, recorder.Body.String())
	}
}

func TestHostPathPickerRequiresControllerAndAllowsCancel(t *testing.T) {
	server := newTestServer(t)
	server.hostPathPicker = func(context.Context, hostPathPickerSpec, string) (string, bool, error) {
		return "", true, nil
	}
	request := httptest.NewRequest(http.MethodPost, "/api/host/path-picker", strings.NewReader(`{"kind":"directory","current":""}`))
	prepareLocalPathPickerRequest(request)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code == http.StatusOK {
		t.Fatal("picker must require the controller lease")
	}

	request = withController(httptest.NewRequest(http.MethodPost, "/api/host/path-picker", strings.NewReader(`{"kind":"directory","current":""}`)))
	prepareLocalPathPickerRequest(request)
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"canceled":true`) {
		t.Fatalf("cancel response = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestHostPathPickerRejectsCrossOriginAndSimpleRequests(t *testing.T) {
	server := newTestServer(t)
	for name, mutate := range map[string]func(*http.Request){
		"cross origin":        func(request *http.Request) { request.Header.Set("Origin", "https://attacker.example") },
		"simple content type": func(request *http.Request) { request.Header.Set("Content-Type", "text/plain") },
	} {
		t.Run(name, func(t *testing.T) {
			request := withController(httptest.NewRequest(http.MethodPost, "/api/host/path-picker", strings.NewReader(`{"kind":"file","current":""}`)))
			prepareLocalPathPickerRequest(request)
			mutate(request)
			recorder := httptest.NewRecorder()
			server.Handler().ServeHTTP(recorder, request)
			if recorder.Code == http.StatusOK {
				t.Fatalf("unsafe picker request succeeded: %s", recorder.Body.String())
			}
		})
	}
}

func prepareLocalPathPickerRequest(request *http.Request) {
	request.RemoteAddr = "127.0.0.1:54321"
	request.Host = "127.0.0.1:49717"
	request.Header.Set("Origin", "http://127.0.0.1:49717")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
}
