package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLabsDefaultOffRejectsRoutesAndAuditions(t *testing.T) {
	server := newTestServer(t)
	for _, path := range []string{"/api/labs/llm", "/api/labs/llm/chat", "/api/labs/observations", "/api/motion/lab/flow", "/api/motion/lab/preview"} {
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, withController(httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{}`))))
		if recorder.Code < 400 {
			t.Fatalf("public build exposed lab endpoint %s: %d", path, recorder.Code)
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, withController(httptest.NewRequest(method, "/api/labs/observations", nil)))
		if recorder.Code < 400 {
			t.Fatalf("public build exposed observations via %s", method)
		}
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, withController(httptest.NewRequest(http.MethodPost, "/api/motion/start", bytes.NewBufferString(`{"lab":{"method":"flow","flow":{}}}`))))
	if recorder.Code < 400 || server.currentMotionEngine() != nil {
		t.Fatal("public build accepted lab audition")
	}
}
