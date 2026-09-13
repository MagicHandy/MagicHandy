package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
)

func TestOperatorRoutePermissionsMatchControlAndHostBoundaries(t *testing.T) {
	s, store, admin, _ := newControllerSessionFixture(t)
	operator, err := store.Create(t.Context(), "operator", "synthetic control permission", accounts.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GrantControl(t.Context(), admin.ID, operator.ID, time.Hour); err != nil {
		t.Fatal(err)
	}
	token, _, err := store.NewSession(t.Context(), operator.ID)
	if err != nil {
		t.Fatal(err)
	}
	handler := s.authenticateRequests(s.authorizeRoutes(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })))
	for _, test := range []struct {
		method, path string
		status       int
	}{
		{http.MethodGet, "/api/setup", http.StatusForbidden},
		{http.MethodGet, "/api/accounts/other/control-grant", http.StatusForbidden},
		{http.MethodPut, "/api/settings/llm-motion-mode", http.StatusNoContent},
		{http.MethodPost, "/api/library/feedback/1/undo", http.StatusNoContent},
	} {
		r := httptest.NewRequest(test.method, test.path, strings.NewReader(`{}`))
		r.AddCookie(testSessionCookie(token))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Errorf("%s %s status=%d, want%d", test.method, test.path, w.Code, test.status)
		}
	}
}
