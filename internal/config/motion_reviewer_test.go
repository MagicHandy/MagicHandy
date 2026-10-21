package config

import "testing"

func TestMotionReviewerSettingsPreserveIndependentRoles(t *testing.T) {
	settings := DefaultSettings().LLM
	if settings.MotionReviewer.Enabled || settings.ModelRouting().Reviewer != nil {
		t.Fatal("reviewer is not opt-in")
	}
	connection := ModelConnection{ID: "review", Name: "Reviewer", Provider: LLMProviderChatGPT, Model: "review-model"}.Normalize()
	settings.Connections = []ModelConnection{connection}
	settings.MotionReviewer = MotionReviewerSettings{Enabled: true, ConnectionID: connection.ID}
	update := LLMUpdateFromSettings(settings)
	next, err := applyLLMUpdate(DefaultSettings().LLM, update)
	if err != nil || next.MotionReviewer != settings.MotionReviewer {
		t.Fatalf("review settings lost: %v", err)
	}
	update.MotionReviewer = nil
	if next, err = applyLLMUpdate(next, update); err != nil || !next.MotionReviewer.Enabled {
		t.Fatalf("old client lost reviewer: %v", err)
	}
	if err := validateLLMConnections(next); err != nil {
		t.Fatal(err)
	}
	routes := next.ModelRouting()
	if routes.Chat.Kind != "local" || routes.Autopilot.Kind != "local" || routes.Reviewer == nil || routes.Reviewer.ConnectionID != "review" || routes.Reviewer.ContextPolicy != "motion_and_user_requests" {
		t.Fatalf("roles %+v", routes)
	}
	next.Connections = nil
	if validateLLMConnections(next) == nil {
		t.Fatal("enabled missing reviewer was accepted")
	}
	next.MotionReviewer.Enabled = false
	if err := validateLLMConnections(next); err != nil {
		t.Fatalf("cannot disable missing reviewer: %v", err)
	}
}
