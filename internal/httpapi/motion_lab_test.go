package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func labAPIRequest() motion.LabRequest {
	return motion.LabRequest{SpeedPercent: 30, CenterPercent: 50, SpanPercent: 80,
		SpanMinPercent: 25, SpanProfile: "wander", VariationPercent: 25,
		RangeAnchorPercent: 0, OutboundTimePercent: 35, Seed: 17}
}

func TestLabPreviewNeverCreatesEngineOrDispatches(t *testing.T) {
	fake := transport.NewFake()
	server := newEnabledLabServer(t, Runtime{Transport: fake, MotionTransport: fake})
	before, _ := server.store.Snapshot()
	encoded, _ := json.Marshal(labAPIRequest())
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/motion/lab/preview", bytes.NewReader(encoded)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("preview %d: %s", recorder.Code, recorder.Body.String())
	}
	var preview motion.LabPreview
	if err := json.Unmarshal(recorder.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	after, _ := server.store.Snapshot()
	if len(preview.Candidates) != 4 || preview.SettingsKey != motion.LabSettingsKey(before.Motion) ||
		motion.LabSettingsKey(before.Motion) != motion.LabSettingsKey(after.Motion) ||
		server.currentMotionEngine() != nil || len(fake.Commands()) != 0 {
		t.Fatal("preview mutated runtime or omitted comparisons")
	}
}

func TestMotionLabAuditionUsesControllerAndUnconditionalStop(t *testing.T) {
	fake := transport.NewFake()
	server := newEnabledLabServer(t, Runtime{Transport: fake, MotionTransport: fake})
	settings, _ := server.store.Snapshot()
	start := motion.LabStart{Request: labAPIRequest(), Method: "combined", SettingsKey: motion.LabSettingsKey(settings.Motion)}
	encoded, _ := json.Marshal(motionRequest{Lab: &start})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/motion/start", bytes.NewReader(encoded)))
	if recorder.Code < 400 || len(fake.Commands()) > 0 {
		t.Fatal("audition bypassed controller")
	}
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, withController(httptest.NewRequest(http.MethodPost, "/api/motion/start", bytes.NewReader(encoded))))
	if recorder.Code != http.StatusOK {
		t.Fatalf("start %d: %s", recorder.Code, recorder.Body.String())
	}
	if server.currentMotionEngine().Snapshot().Target.Source != motion.TargetSourceMotionLab {
		t.Fatal("audition provenance missing")
	}
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/motion/stop", nil))
	if recorder.Code != http.StatusOK || server.currentMotionEngine().Snapshot().Running {
		t.Fatal("unconditional stop failed")
	}
	count := len(fake.Commands())
	time.Sleep(150 * time.Millisecond)
	if len(fake.Commands()) != count {
		t.Fatal("audition dispatched after Stop")
	}
}

func TestMotionLabRejectsStalePreviewBeforeDispatch(t *testing.T) {
	fake := transport.NewFake()
	server := newEnabledLabServer(t, Runtime{Transport: fake, MotionTransport: fake})
	start := motion.LabStart{Request: labAPIRequest(), Method: "combined", SettingsKey: "stale"}
	encoded, _ := json.Marshal(motionRequest{Lab: &start})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, withController(httptest.NewRequest(http.MethodPost, "/api/motion/start", bytes.NewReader(encoded))))
	if recorder.Code != http.StatusBadRequest || len(fake.Commands()) != 0 {
		t.Fatalf("stale preview was accepted: %d", recorder.Code)
	}
}

func TestMotionLabProposalIsPreviewOnly(t *testing.T) {
	fake := transport.NewFake()
	provider := &scriptedLLMProvider{responses: []string{`{"range_anchor_percent":0,"outbound_time_percent":35,"explanation":"Try a fixed base and slower return."}`}}
	server := newEnabledLabServer(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
	before, _ := server.store.Snapshot()
	encoded, _ := json.Marshal(map[string]any{"request": labAPIRequest(), "message": "Hold the base and slow the return"})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, withController(httptest.NewRequest(http.MethodPost, "/api/motion/lab/proposal", bytes.NewReader(encoded))))
	if recorder.Code != http.StatusOK {
		t.Fatalf("proposal %d: %s", recorder.Code, recorder.Body.String())
	}
	after, _ := server.store.Snapshot()
	if server.currentMotionEngine() != nil || len(fake.Commands()) != 0 ||
		motion.LabSettingsKey(before.Motion) != motion.LabSettingsKey(after.Motion) {
		t.Fatal("proposal mutated motion")
	}
}

func TestMotionLabStopCancelsGeneratingProposal(t *testing.T) {
	fake := transport.NewFake()
	provider := &blockingLLMProvider{started: make(chan struct{})}
	server := newEnabledLabServer(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
	encoded, _ := json.Marshal(map[string]any{"request": labAPIRequest(), "message": "Compare timing"})
	result := make(chan int, 1)
	go func() {
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, withController(httptest.NewRequest(http.MethodPost, "/api/motion/lab/proposal", bytes.NewReader(encoded))))
		result <- recorder.Code
	}()
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("proposal did not start")
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/motion/stop", nil))
	select {
	case code := <-result:
		if code != http.StatusConflict {
			t.Fatalf("canceled proposal returned %d", code)
		}
	case <-time.After(time.Second):
		t.Fatal("proposal survived Stop")
	}
	if server.currentMotionEngine() != nil {
		t.Fatal("proposal created an engine")
	}
}
