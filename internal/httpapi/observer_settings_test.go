package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestObserverSettingsExcludeHostConfigurationAndKeepSemanticControls(t *testing.T) {
	s, store, admin, adminCookie := newControllerSessionFixture(t)
	observer := newAdmissionIdentity(t, store, admin.ID, "observer", false)
	hostDirectory := filepath.Join(t.TempDir(), "host-detail-fixture")
	if _, _, err := s.store.Update(func(settings config.Settings) (config.Settings, error) {
		settings.LLM.OllamaModelsPath = filepath.Join(hostDirectory, "models")
		settings.LLM.OllamaBaseURL = "http://host-detail-fixture.invalid:11434"
		settings.Voice.TTSWorkerPath = filepath.Join(hostDirectory, "worker.exe")
		settings.Voice.TTSWorkerArgs = []string{"--private-host-detail-fixture"}
		settings.Media.LibraryPaths = []string{filepath.Join(hostDirectory, "videos")}
		settings.Media.ScriptSmoothingPercent = 3
		settings.Motion.SpeedMinPercent = 19
		settings.Motion.SpeedMaxPercent = 29
		return settings, nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"/api/state", "/api/settings"} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			for _, identity := range []struct {
				name   string
				cookie *http.Cookie
				host   bool
			}{
				{"observer", observer.cookie, false}, {"administrator", adminCookie, true},
			} {
				t.Run(method+route+identity.name, func(t *testing.T) {
					r := httptest.NewRequest(method, route, nil)
					r.AddCookie(identity.cookie)
					w := httptest.NewRecorder()
					s.Handler().ServeHTTP(w, r)
					if w.Code != http.StatusOK {
						t.Fatalf("status %d", w.Code)
					}
					body := w.Body.String()
					if strings.Contains(body, "host-detail-fixture") != identity.host {
						t.Fatal("host metadata visibility does not match role")
					}
					var response struct {
						Settings config.PublicSettings `json:"settings"`
					}
					if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
						t.Fatal(err)
					}
					if response.Settings.Media.ScriptSmoothingPercent != 3 || response.Settings.Motion.SpeedMinPercent != 19 {
						t.Fatalf("semantic values: smoothing=%d min=%v", response.Settings.Media.ScriptSmoothingPercent, response.Settings.Motion.SpeedMinPercent)
					}
					encodedPath, err := json.Marshal(s.store.DataDir())
					if err != nil {
						t.Fatal(err)
					}
					if !identity.host && strings.Contains(body, string(encodedPath)) {
						t.Fatal("host datastore path entered observer response")
					}
				})
			}
		}
	}
}
