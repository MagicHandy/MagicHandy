package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func labObservationRequest(t *testing.T, server *Server, method, path string, body any, controller bool) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	if controller {
		request = withController(request)
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	return recorder
}

func motionObservationBody(server *Server, text string) observationRequest {
	state := server.labState()
	return observationRequest{Source: "motion", Method: "flow", Text: text, Spec: &state.Current, SettingsKey: state.SettingsKey}
}

func TestLabObservationsPersistWithoutInfluencingModelOrMotion(t *testing.T) {
	fake := transport.NewFake()
	provider := &scriptedLLMProvider{responses: []string{`{"reply":"A preview only.","controls":{"anchor_percent":100}}`, `{"reply":"Still a preview."}`}}
	server := newEnabledLabServer(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
	const observation = "OBSERVATION_SENTINEL: plotted range looked uneven; not a device test"
	response := labObservationRequest(t, server, http.MethodPost, "/api/labs/observations", motionObservationBody(server, observation), true)
	if response.Code != http.StatusOK {
		t.Fatalf("save: %d %s", response.Code, response.Body)
	}
	state := server.labState()
	for index := range 2 {
		response = labObservationRequest(t, server, http.MethodPost, "/api/labs/llm/chat", map[string]any{"message": "Review the current preview", "method": "controls", "revision": state.Revision}, true)
		if response.Code != http.StatusOK {
			t.Fatalf("chat: %d %s", response.Code, response.Body)
		}
		state = server.labState()
		if index == 0 {
			// Saving a reply after a limits edit must retain the original trial limits.
			original := state.Turns[0].Limits
			saveSettings(t, server.store, func(settings config.Settings) config.Settings { settings.Motion.SpeedMaxPercent--; return settings })
			turn := 0
			response = labObservationRequest(t, server, http.MethodPost, "/api/labs/observations", observationRequest{Source: "llm", Text: "LLM_OBSERVATION_SENTINEL", Revision: state.Revision, TurnIndex: &turn}, true)
			if response.Code != http.StatusOK {
				t.Fatalf("save reply: %d %s", response.Code, response.Body)
			}
			rows, err := server.readLabObservations(context.Background())
			if err != nil || rows[0].Settings != original || rows[0].Trial == nil || rows[0].Spec.AnchorPercent != 100 {
				t.Fatalf("reply context lost: %+v / %v", rows, err)
			}
		}
	}
	for _, request := range provider.requests {
		encoded, _ := json.Marshal(request)
		if bytes.Contains(encoded, []byte("OBSERVATION_SENTINEL")) {
			t.Fatal("saved observation entered model context automatically")
		}
	}
	if len(fake.Commands()) != 0 || server.currentMotionEngine() != nil {
		t.Fatal("recording review evidence issued motion")
	}
	response = labObservationRequest(t, server, http.MethodPost, "/api/labs/llm/reset", map[string]any{"spec": state.Current}, true)
	if response.Code != http.StatusOK || len(server.labState().Turns) != 0 {
		t.Fatal("lab reset failed")
	}
	assertLabObservationsSurviveReopen(t, server, observation)
}

func assertLabObservationsSurviveReopen(t *testing.T, server *Server, observation string) {
	t.Helper()
	dataDir, databasePath := server.store.DataDir(), server.store.Datastore().Path()
	server.Close()
	reopened, err := config.OpenStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	server = newTestServerWithStore(t, reopened, Runtime{})
	response := labObservationRequest(t, server, http.MethodGet, "/api/labs/observations", nil, false)
	var payload struct {
		Observations []labObservation `json:"observations"`
		StoragePath  string           `json:"storage_path"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || len(payload.Observations) != 2 || payload.StoragePath != databasePath || payload.Observations[1].Text != observation || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("observations did not survive reopen: %s", response.Body)
	}
	if len(server.labState().Turns) != 0 {
		t.Fatal("saved observations restored or altered the temporary conversation")
	}
}

func TestLabObservationScopeAndControllerGuards(t *testing.T) {
	server := newEnabledLabServer(t, Runtime{})
	valid := motionObservationBody(server, "A plotted estimate")
	response := labObservationRequest(t, server, http.MethodPost, "/api/labs/observations", valid, true)
	if response.Code != http.StatusOK {
		t.Fatalf("initial save: %s", response.Body)
	}
	for _, mutate := range []func(*observationRequest){
		func(body *observationRequest) { body.SettingsKey = "stale" },
		func(body *observationRequest) { body.Text = strings.Repeat("x", 2001) },
		func(body *observationRequest) { body.Text = " " },
		func(body *observationRequest) { body.Method = "unknown" },
		func(body *observationRequest) { body.Spec = nil },
		func(body *observationRequest) {
			body.Source = "llm"
			body.Revision = 99
			turn := 0
			body.TurnIndex = &turn
		},
	} {
		body := valid
		mutate(&body)
		response = labObservationRequest(t, server, http.MethodPost, "/api/labs/observations", body, true)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid scope accepted: %d %s", response.Code, response.Body)
		}
	}
	rows, err := server.readLabObservations(context.Background())
	if err != nil || len(rows) != 1 {
		t.Fatalf("rejected save changed records: %+v / %v", rows, err)
	}
	path := "/api/labs/observations/" + rows[0].ID
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		endpoint := "/api/labs/observations"
		if method == http.MethodDelete {
			endpoint = path
		}
		response = labObservationRequest(t, server, method, endpoint, valid, false)
		if response.Code < 400 {
			t.Fatal("read-only client mutated observations")
		}
	}
	response = labObservationRequest(t, server, http.MethodDelete, path, nil, true)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"observations":[]`) {
		t.Fatalf("delete: %d %s", response.Code, response.Body)
	}
	response = labObservationRequest(t, server, http.MethodDelete, path, nil, true)
	if response.Code != http.StatusNotFound {
		t.Fatal("missing observation deletion was not reported")
	}
}

func TestLabObservationsConcurrentWritesAndStorageBound(t *testing.T) {
	server := newEnabledLabServer(t, Runtime{})
	var workers sync.WaitGroup
	failures := make(chan error, 8)
	for index := range 8 {
		row, err := server.prepareLabObservation(motionObservationBody(server, fmt.Sprint(index)))
		if err != nil {
			t.Fatal(err)
		}
		workers.Go(func() {
			_, err := server.updateLabObservations(context.Background(), func(rows []labObservation) ([]labObservation, error) { return append(rows, row), nil })
			failures <- err
		})
	}
	workers.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, err := server.readLabObservations(context.Background())
	if err != nil || len(rows) != 8 {
		t.Fatalf("concurrent save lost records: %d / %v", len(rows), err)
	}
	_, err = server.updateLabObservations(context.Background(), func([]labObservation) ([]labObservation, error) {
		return make([]labObservation, maximumLabObservations), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	response := labObservationRequest(t, server, http.MethodPost, "/api/labs/observations", motionObservationBody(server, "at capacity"), true)
	if response.Code < 400 {
		t.Fatal("observation capacity was not enforced")
	}
	rows, err = server.readLabObservations(context.Background())
	if err != nil || len(rows) != maximumLabObservations {
		t.Fatal("full storage silently evicted review evidence")
	}
	_, err = server.store.Datastore().SQL().Exec("UPDATE app_kv SET value = ? WHERE key = ?", "{corrupt", labObservationsKey)
	if err != nil {
		t.Fatal(err)
	}
	response = labObservationRequest(t, server, http.MethodGet, "/api/labs/observations", nil, false)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatal("corrupt observations were presented as an empty collection")
	}
}
