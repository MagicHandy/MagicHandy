package httpapi

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSetupRetryPlanSurvivesRestartWithoutPublicReferenceData(t *testing.T) {
	server := newTestServer(t)
	manager := server.setup
	ctx, job, err := manager.reserveJob("install_plan", "selected_components", "", "queued")
	if err != nil {
		t.Fatal(err)
	}
	original := &setupInstallPlanRequest{Voice: &setupVoiceInstallRequest{Module: "faster-qwen3-tts", Device: "cuda", AutoLaunch: true, Reference: &setupQwenReference{WAV: "private-sample.wav", Transcript: "private exact transcript"}}, Parakeet: true, EnableVoice: true}
	manager.mu.Lock()
	manager.job.retryPlan = cloneSetupInstallPlan(original)
	manager.job.RetryAvailable = true
	manager.mu.Unlock()
	original.Voice.Module = "tampered"
	manager.finishJob(ctx, job.ID, errors.New("download interrupted"), "Selected components")
	reloaded := &setupManager{dataDir: manager.dataDir}
	reloaded.loadPersistedSetupJob()
	result := reloaded.Snapshot()
	if result == nil || !result.RetryAvailable || result.retryPlan == nil || result.retryPlan.Llama != nil || result.retryPlan.Model != nil || result.retryPlan.Voice.Module != "faster-qwen3-tts" || result.retryPlan.Voice.Reference.Transcript != "private exact transcript" || !result.retryPlan.Parakeet || !result.retryPlan.EnableVoice {
		t.Fatalf("original plan lost: %+v", result)
	}
	result.retryPlan.Voice.Reference.Transcript = "tampered"
	if reloaded.Snapshot().retryPlan.Voice.Reference.Transcript != "private exact transcript" {
		t.Fatal("retry snapshots share mutable storage")
	}
	for _, value := range []any{reloaded.Snapshot(), reloaded.lastFailureReport} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "private exact transcript") || strings.Contains(string(encoded), "private-sample.wav") {
			t.Fatal("private retry reference leaked into public job or report")
		}
	}
}
