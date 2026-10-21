package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/openaiauth"
)

func TestCloudCredentialRecoveryRequiresConfirmationAndPreservesSettings(t *testing.T) {
	server := newTestServer(t)
	if err := server.cloudPlanning.auth.SetAPIKey(t.Context(), "fixture-key"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(server.store.DataDir(), "openai-private", "credentials.json")
	if err := os.WriteFile(path, []byte("unreadable-credential-fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	settingsBefore, _ := server.store.Snapshot()
	before, err := json.Marshal(settingsBefore)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		body string
		want int
	}{
		{`{}`, http.StatusBadRequest},
		{`{"confirm":false}`, http.StatusBadRequest},
		{`{"confirm":true}`, http.StatusOK},
		{`{"confirm":true}`, http.StatusConflict},
	} {
		response := httptest.NewRecorder()
		request := withController(httptest.NewRequest(http.MethodPost, "/api/llm/cloud/reset-credentials", strings.NewReader(tc.body)))
		request.Header.Set("Content-Type", "application/json")
		server.Handler().ServeHTTP(response, request)
		if response.Code != tc.want {
			t.Fatalf("reset status %d, want %d: %s", response.Code, tc.want, response.Body.String())
		}
		if tc.want == http.StatusOK {
			var status struct {
				Connection openaiauth.Status `json:"connection"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
				t.Fatal(err)
			}
			if status.Connection.RecoveryRequired || status.Connection.DecisionsKeySet || status.Connection.Active != "" {
				t.Fatal("reset did not return empty usable credentials")
			}
		}
	}
	settingsAfter, _ := server.store.Snapshot()
	after, err := json.Marshal(settingsAfter)
	if err != nil || string(before) != string(after) {
		t.Fatal("credential recovery changed model settings")
	}
}
