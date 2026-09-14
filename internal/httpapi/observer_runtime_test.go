package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/media"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
	"github.com/mapledaemon/MagicHandy/internal/voice"
)

func TestObserverRuntimeViewsPreserveProgressWithoutPrivateFailures(t *testing.T) {
	s, store, admin, cookie := newControllerSessionFixture(t)
	observer := newAdmissionIdentity(t, store, admin.ID, "observer", false)
	const private = "private-host-runtime-fixture"
	views := map[string]func(*http.Request) any{
		"scan": func(r *http.Request) any {
			return s.clientScanState(r, media.ScanState{Running: true, Cancellable: true, FilesVisited: 17, CurrentLocation: private, Error: private, Summary: media.ScanSummary{Issues: []media.ScanIssue{{Location: private, Message: private}}}})
		},
		"job": func(r *http.Request) any {
			return s.clientJobState(r, media.JobState{Running: true, Cancellable: true, Processed: 17, CurrentName: private, Error: private, Issues: []media.JobIssue{{Name: private, Message: private}}})
		},
		"media": func(r *http.Request) any {
			return s.clientVideos(r, []media.Video{{ID: "shared-video", DisplayName: "Shared video", SizeBytes: 17, LocationPath: private, RelativePath: private}})
		},
		"provider": func(r *http.Request) any {
			return s.clientProviderStatus(r, llm.ProviderStatus{Provider: private, BaseURL: private, Model: private, Models: []string{private}, Message: private, Loaded: true})
		},
		"voice": func(r *http.Request) any {
			return s.clientVoiceRequest(r, voice.RequestSnapshot{ID: "shared-speech", AudioBytes: 17, Rejected: private, Transcript: []voice.TranscriptCandidate{{Text: "Shared transcript"}}, Error: &voice.WorkerError{Code: private, Message: private, Retryable: true}})
		},
		"motion": func(r *http.Request) any {
			return s.clientMotionSnapshot(r, motion.ActiveMotionState{Running: true, LastError: private})
		},
		"sync": func(r *http.Request) any {
			return s.clientSyncStatus(r, mediaSyncStatus{Active: false, State: "error", Message: private})
		},
		"transport": func(r *http.Request) any {
			return s.clientTransportDiagnostics(r, transport.TransportDiagnostics{Name: "fake", Connected: true, CommandCount: 17, LastError: private})
		},
	}
	for name, view := range views {
		t.Run(name, func(t *testing.T) {
			for _, identity := range []struct {
				cookie *http.Cookie
				host   bool
			}{{observer.cookie, false}, {cookie, true}} {
				r := httptest.NewRequest(http.MethodGet, "/api/state", nil)
				r.AddCookie(identity.cookie)
				w := httptest.NewRecorder()
				s.authenticateRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, view(r)) })).ServeHTTP(w, r)
				if w.Code != http.StatusOK || strings.Contains(w.Body.String(), private) != identity.host {
					t.Fatalf("host=%v projection: %d %s", identity.host, w.Code, w.Body.String())
				}
				if !identity.host {
					assertObserverRuntimeProgress(t, name, w.Body.String())
				}
			}
		})
	}
}

func assertObserverRuntimeProgress(t *testing.T, name, body string) {
	t.Helper()
	markers := map[string]string{"scan": `"files_visited":17`, "job": `"processed":17`, "media": `"id":"shared-video"`, "provider": `"loaded":true`, "voice": "Shared transcript", "motion": `"running":true`, "sync": `"state":"error"`, "transport": `"command_count":17`}
	if !strings.Contains(body, markers[name]) || strings.Contains(body, `"cancellable":true`) {
		t.Fatalf("observer lost progress or gained host job control: %s", body)
	}
}

func TestVoiceStatusHandlerProjectsWorkerCommandForObserver(t *testing.T) {
	s, store, admin, cookie := newControllerSessionFixture(t)
	observer := newAdmissionIdentity(t, store, admin.ID, "observer", false)
	s.voice.Worker(voice.RoleTTS).SetConfig(voice.WorkerConfig{Enabled: true, Command: "private-worker-command-fixture"})
	for _, identity := range []struct {
		cookie *http.Cookie
		host   bool
	}{{observer.cookie, false}, {cookie, true}} {
		for _, route := range []string{"/api/voice/status", "/api/state"} {
			w := authenticatedControlRequest(s, identity.cookie, http.MethodGet, route, "observer", "")
			if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "private-worker-command-fixture") != identity.host {
				t.Fatalf("host=%v %s: %d %s", identity.host, route, w.Code, w.Body.String())
			}
			var response struct {
				Voice struct {
					Workers map[voice.Role]voice.WorkerStatus `json:"workers"`
				} `json:"voice"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if !response.Voice.Workers[voice.RoleTTS].Configured {
				t.Fatal("configured voice lost operational readiness")
			}
		}
	}
}

func TestSharedChatEventsRejectUnreviewedShapesAndPreserveStopCancellation(t *testing.T) {
	s, _, _, _ := newControllerSessionFixture(t)
	r := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	for _, event := range []string{"status", "delta", "repair_delta", "message", "speech", "done", "malformed"} {
		view, err := s.clientChatEvent(r, event, map[string]any{"text": "shared", "diagnostics": "private-host-event-fixture", "unexpected": "private-host-event-fixture"})
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(view)
		if err != nil || strings.Contains(string(data), "private-host-event-fixture") {
			t.Fatalf("event %s disclosed unreviewed fields", event)
		}
		if _, err := s.clientChatEvent(r, event, "unexpected payload"); err == nil {
			t.Fatalf("event %s accepted unreviewed shape", event)
		}
	}
	if _, err := s.clientChatEvent(r, "unknown", map[string]any{}); err == nil {
		t.Fatal("unknown event was accepted")
	}
	if got := clientChatFailure(map[string]string{"message": "Chat canceled by Emergency Stop."}); got != "Chat canceled by Emergency Stop." {
		t.Fatal("Stop cancellation lost its actionable explanation")
	}
	if got := clientChatFailure(map[string]string{"message": "http://private-service.invalid failed"}); got != administratorDetails {
		t.Fatal("provider error escaped shared view")
	}
}
