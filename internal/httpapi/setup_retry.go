package httpapi

import (
	"errors"
	"net/http"
)

// Retain the user's exact install selection on the host. The plan is excluded
// from public jobs and failure exports, including audio paths and transcripts.
func cloneSetupInstallPlan(plan *setupInstallPlanRequest) *setupInstallPlanRequest {
	if plan == nil {
		return nil
	}
	result := *plan
	if plan.Llama != nil {
		item := *plan.Llama
		result.Llama = &item
	}
	if plan.Model != nil {
		item := *plan.Model
		result.Model = &item
	}
	if plan.Voice != nil {
		item := *plan.Voice
		if item.Reference != nil {
			reference := *item.Reference
			item.Reference = &reference
		}
		result.Voice = &item
	}
	return &result
}

func (s *Server) handleSetupRetry(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	job := s.setup.Snapshot()
	if job == nil || job.retryPlan == nil || (job.Status != setupJobFailed && job.Status != setupJobCancelled) {
		writeError(w, http.StatusConflict, errors.New("the original install plan is unavailable; choose the features again"))
		return
	}
	if job.retryPlan.Voice != nil && job.retryPlan.Voice.Reference != nil && !s.capabilities(r).ConfigureHost {
		writeError(w, http.StatusForbidden, errors.New(administratorHostAccessRequired))
		return
	}
	next, err := s.setup.StartInstallPlan(*job.retryPlan)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"installation": next})
}
