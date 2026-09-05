package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func createTestSequence(t *testing.T, server *Server, body labTestCreateRequest) labTestRunView {
	t.Helper()
	response := labObservationRequest(t, server, http.MethodPost, "/api/labs/tests", body, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("create sequence: %d %s", response.Code, response.Body)
	}
	var view labTestRunView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	return view
}

func feedbackBody(view labTestRunView) map[string]any {
	return map[string]any{"revision": view.Run.Revision, "step_id": view.Run.Steps[view.NextIndex].ID, "rating": 2, "basis": "preview", "comment": "Visual check only: uneven reversals."}
}

func checkTestSequencePreviews(t *testing.T, view labTestRunView) {
	t.Helper()
	for index, method := range []string{"creative", "anchored", "flow"} {
		step := view.Run.Steps[index]
		if step.Source.Method != method || len(step.Preview.Candidates) != 1 || step.Preview.Candidates[0].Method != method || len(step.Preview.Candidates[0].Samples) == 0 {
			t.Fatal("test did not capture the shared engine output for its method")
		}
	}
}

func TestLabTestSequencePersistsExactSourceAndProgress(t *testing.T) {
	fake := transport.NewFake()
	server := newEnabledLabServer(t, Runtime{Transport: fake, MotionTransport: fake})
	view := createTestSequence(t, server, labTestCreateRequest{Preset: "motion_comparison"})
	if len(view.Run.Steps) != 3 || view.NextIndex != 0 || !view.CanAudition || len(fake.Commands()) != 0 {
		t.Fatalf("invalid initial sequence: %+v", view)
	}
	checkTestSequencePreviews(t, view)
	path := "/api/labs/tests/" + view.Run.ID
	response := labObservationRequest(t, server, http.MethodPost, path+"/feedback", feedbackBody(view), true)
	if response.Code != http.StatusOK {
		t.Fatalf("save feedback: %s", response.Body)
	}
	response = labObservationRequest(t, server, http.MethodPost, path+"/feedback", feedbackBody(view), true)
	if response.Code != http.StatusConflict {
		t.Fatal("duplicate save advanced a second round")
	}
	dataDir := server.store.DataDir()
	server.Close()
	store, err := config.OpenStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	server = newTestServerWithStore(t, store, Runtime{})
	response = labObservationRequest(t, server, http.MethodGet, path, nil, false)
	var reopened labTestRunView
	if err := json.Unmarshal(response.Body.Bytes(), &reopened); err != nil {
		t.Fatal(err)
	}
	if reopened.NextIndex != 1 || reopened.Run.Steps[0].Feedback.Comment != "Visual check only: uneven reversals." || !sameTestPreview(view.Run.Steps[0].Preview, reopened.Run.Steps[0].Preview) || reopened.StoragePath != store.Datastore().Path() {
		t.Fatal("progress, comments, source or database location did not survive restart")
	}
	for reopened.NextIndex < len(reopened.Run.Steps) {
		body := feedbackBody(reopened)
		body["rating"], body["basis"], body["comment"] = 0, "skipped", "Unable to assess this round."
		response = labObservationRequest(t, server, http.MethodPost, path+"/feedback", body, true)
		if response.Code != http.StatusOK {
			t.Fatalf("skip: %s", response.Body)
		}
		if err := json.Unmarshal(response.Body.Bytes(), &reopened); err != nil {
			t.Fatal(err)
		}
	}
	if reopened.CanAudition || reopened.Run.Steps[2].Feedback.Comment == "" || len(server.labState().Turns) != 0 || len(fake.Commands()) != 0 {
		t.Fatal("completion lost evidence or caused model/motion effects")
	}
}

func TestLabTestSequenceCapturesLLMFailureAndBeforeState(t *testing.T) {
	provider := &scriptedLLMProvider{responses: []string{"not a valid motion response"}}
	server := newEnabledLabServer(t, Runtime{LLMProvider: provider})
	state := server.labState()
	response := labObservationRequest(t, server, http.MethodPost, "/api/labs/llm/chat", map[string]any{"message": "Keep the tip fixed", "method": "controls", "revision": state.Revision}, true)
	if response.Code != http.StatusOK {
		t.Fatalf("LLM trial: %s", response.Body)
	}
	state = server.labState()
	turn := 0
	target := observationRequest{Source: "llm", Revision: state.Revision, TurnIndex: &turn}
	view := createTestSequence(t, server, labTestCreateRequest{Preset: "llm_comparison", Target: &target})
	if len(view.Run.Steps) != 2 || view.Run.Steps[0].Phase != "before" || view.Run.Steps[1].Phase != "after" || view.Run.Steps[0].Preview == nil || view.Run.Steps[1].Preview != nil || view.Run.Steps[1].PreviewError == "" || view.Run.Steps[1].Source.Trial.Raw != "not a valid motion response" {
		t.Fatal("failed response was dropped or offered for audition")
	}
	// Clearing the ephemeral conversation must not alter a captured trial.
	response = labObservationRequest(t, server, http.MethodPost, "/api/labs/llm/reset", map[string]any{"spec": state.Current}, true)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body)
	}
	runs, err := server.readLabTestRuns(context.Background())
	if err != nil || runs[0].Steps[1].Source.Trial.Message != "Keep the tip fixed" {
		t.Fatal("captured reply depended on the conversation remaining in memory")
	}
}

func TestLabTestSequenceLimitsAndCompilerChangesDisableAudition(t *testing.T) {
	server := newEnabledLabServer(t, Runtime{})
	view := createTestSequence(t, server, labTestCreateRequest{Preset: "motion_comparison"})
	settings, _ := server.store.Snapshot()
	saveSettings(t, server.store, func(current config.Settings) config.Settings {
		current.Motion.ReverseDirection = !current.Motion.ReverseDirection
		return current
	})
	changed := server.testRunView(view.Run)
	if changed.CanAudition || changed.Warning == "" || !sameTestPreview(changed.Run.Steps[0].Preview, view.Run.Steps[0].Preview) {
		t.Fatal("limits edit silently changed the frozen test or left audition enabled")
	}
	saveSettings(t, server.store, func(current config.Settings) config.Settings { current.Motion = settings.Motion; return current })
	view.Run.Steps[0].Preview.Candidates[0].Samples[0].PositionPercent++
	if changed = server.testRunView(view.Run); changed.CanAudition || !strings.Contains(changed.Warning, "Motion output changed") {
		t.Fatal("old compiler output was offered for audition under a different curve")
	}
}

func TestLabTestFeedbackRejectsMovingEngineAndOutOfOrderWrites(t *testing.T) {
	fake := transport.NewFake()
	server := newEnabledLabServer(t, Runtime{Transport: fake, MotionTransport: fake})
	view := createTestSequence(t, server, labTestCreateRequest{Preset: "motion_comparison"})
	path := "/api/labs/tests/" + view.Run.ID + "/feedback"
	preview := view.Run.Steps[2].Preview
	response := labObservationRequest(t, server, http.MethodPost, "/api/motion/start", motionRequest{Lab: &motion.LabStart{Method: "flow", Flow: &preview.Spec, SettingsKey: preview.SettingsKey}}, true)
	if response.Code != http.StatusOK {
		t.Fatalf("fake audition: %s", response.Body)
	}
	response = labObservationRequest(t, server, http.MethodPost, path, feedbackBody(view), true)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "stop motion") {
		t.Fatal("feedback advanced while motion was running")
	}
	response = labObservationRequest(t, server, http.MethodPost, "/api/motion/stop", nil, true)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body)
	}
	body := feedbackBody(view)
	body["step_id"] = view.Run.Steps[1].ID
	if response = labObservationRequest(t, server, http.MethodPost, path, body, true); response.Code != http.StatusConflict {
		t.Fatal("out-of-order round accepted")
	}
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for range 2 {
		wg.Go(func() {
			codes <- labObservationRequest(t, server, http.MethodPost, path, feedbackBody(view), true).Code
		})
	}
	wg.Wait()
	close(codes)
	success := 0
	for code := range codes {
		if code == http.StatusOK {
			success++
		} else if code != http.StatusConflict {
			t.Fatalf("unexpected concurrent save: %d", code)
		}
	}
	if success != 1 {
		t.Fatal("concurrent feedback did not have exactly one winner")
	}
}

func TestLabTestRunGuardsBoundsAndCorruptStorage(t *testing.T) {
	server := newEnabledLabServer(t, Runtime{})
	view := createTestSequence(t, server, labTestCreateRequest{Preset: "motion_comparison"})
	for _, endpoint := range []string{"/api/labs/tests", "/api/labs/tests/" + view.Run.ID + "/feedback"} {
		if response := labObservationRequest(t, server, http.MethodPost, endpoint, feedbackBody(view), false); response.Code < 400 {
			t.Fatal("reader changed test storage")
		}
	}
	for _, mutate := range []func(map[string]any){
		func(body map[string]any) { body["rating"] = 0 },
		func(body map[string]any) { body["basis"] = "invented" },
		func(body map[string]any) { body["comment"] = strings.Repeat("x", 2001) },
	} {
		body := feedbackBody(view)
		mutate(body)
		if response := labObservationRequest(t, server, http.MethodPost, "/api/labs/tests/"+view.Run.ID+"/feedback", body, true); response.Code != http.StatusUnprocessableEntity {
			t.Fatal("invalid feedback accepted")
		}
	}
	setLabsForTest(t, server, false)
	if response := labObservationRequest(t, server, http.MethodGet, "/api/labs/tests", nil, false); response.Code != http.StatusForbidden {
		t.Fatal("disabled Labs exposed tests")
	}
	setLabsForTest(t, server, true)
	_, err := server.updateLabTestRuns(context.Background(), func([]labTestRun) ([]labTestRun, error) { return make([]labTestRun, maximumLabTestRuns), nil })
	if err != nil {
		t.Fatal(err)
	}
	if response := labObservationRequest(t, server, http.MethodPost, "/api/labs/tests", labTestCreateRequest{Preset: "motion_comparison"}, true); response.Code != http.StatusServiceUnavailable {
		t.Fatal("full collection silently evicted a run")
	}
	if _, err = server.store.Datastore().SQL().Exec("UPDATE app_kv SET value = ? WHERE key = ?", "{broken", labTestRunsKey); err != nil {
		t.Fatal(err)
	}
	if response := labObservationRequest(t, server, http.MethodGet, "/api/labs/tests", nil, false); response.Code != http.StatusServiceUnavailable {
		t.Fatal("corrupt runs presented as empty")
	}
}

func TestLabTestCustomSequenceAndByteBound(t *testing.T) {
	server := newEnabledLabServer(t, Runtime{})
	target := motionObservationBody(server, "unused")
	body := labTestCreateRequest{Title: "Prepared change review", Steps: []labTestStepRequest{{Title: "Inspect reversals", Instruction: "Comment on reversal smoothness.", Target: target}}}
	view := createTestSequence(t, server, body)
	if len(view.Run.Steps) != 1 || view.Run.Title != body.Title || view.Run.Steps[0].Instruction != body.Steps[0].Instruction {
		t.Fatal("authored sequence was replaced by a preset")
	}
	body.Steps[0].Target.SettingsKey = "stale"
	if response := labObservationRequest(t, server, http.MethodPost, "/api/labs/tests", body, true); response.Code != http.StatusUnprocessableEntity {
		t.Fatal("stale custom capture accepted")
	}
	if _, err := decodeLabTestRuns(strings.Repeat(" ", maximumLabTestBytes+1), nil); err == nil {
		t.Fatal("storage byte bound was not enforced")
	}
	oversized := view.Run
	oversized.Steps[0].Instruction = strings.Repeat("x", maximumLabTestBytes)
	if _, err := server.updateLabTestRuns(context.Background(), func([]labTestRun) ([]labTestRun, error) { return []labTestRun{oversized}, nil }); err == nil {
		t.Fatal("oversized update was saved")
	}
	retained, err := server.readLabTestRuns(context.Background())
	if err != nil || len(retained) != 1 || retained[0].Steps[0].Instruction != "Comment on reversal smoothness." {
		t.Fatal("oversized update replaced existing evidence")
	}
}
