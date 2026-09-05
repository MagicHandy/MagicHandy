package httpapi

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// A test run freezes review artifacts, not transport commands. Progress and
// feedback are durable and backend-owned; no run operation starts motion.
type labTestRun struct {
	ID        string        `json:"id"`
	Title     string        `json:"title"`
	CreatedAt string        `json:"created_at"`
	Version   string        `json:"version"`
	Commit    string        `json:"commit"`
	Revision  uint64        `json:"revision"`
	Steps     []labTestStep `json:"steps"`
}

type labTestStep struct {
	ID           string              `json:"id"`
	Title        string              `json:"title"`
	Instruction  string              `json:"instruction"`
	Phase        string              `json:"phase,omitempty"`
	Source       labObservation      `json:"source"`
	Preview      *motion.FlowPreview `json:"preview,omitempty"`
	PreviewError string              `json:"preview_error,omitempty"`
	Feedback     *labTestFeedback    `json:"feedback,omitempty"`
}

type labTestFeedback struct {
	Rating    int    `json:"rating"`
	Basis     string `json:"basis"`
	Comment   string `json:"comment"`
	CreatedAt string `json:"created_at"`
}

type labTestStepRequest struct {
	Title       string             `json:"title"`
	Instruction string             `json:"instruction"`
	Target      observationRequest `json:"target"`
	Phase       string             `json:"phase,omitempty"`
}

type labTestCreateRequest struct {
	Title  string               `json:"title"`
	Preset string               `json:"preset,omitempty"`
	Target *observationRequest  `json:"target,omitempty"`
	Steps  []labTestStepRequest `json:"steps,omitempty"`
}

type labTestRunView struct {
	Run         labTestRun `json:"run"`
	NextIndex   int        `json:"next_index"`
	CanAudition bool       `json:"can_audition"`
	Warning     string     `json:"warning,omitempty"`
	StoragePath string     `json:"storage_path"`
}

func (s *Server) labTestRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/labs/tests", s.withLabs(s.handleLabTestRuns))
	mux.HandleFunc("POST /api/labs/tests", s.withLabs(s.handleCreateLabTestRun))
	mux.HandleFunc("GET /api/labs/tests/{id}", s.withLabs(s.handleLabTestRun))
	mux.HandleFunc("POST /api/labs/tests/{id}/feedback", s.withLabs(s.handleLabTestFeedback))
	mux.HandleFunc("DELETE /api/labs/tests/{id}", s.withLabs(s.handleDeleteLabTestRun))
}

func nextLabTest(run labTestRun) int {
	for index, step := range run.Steps {
		if step.Feedback == nil {
			return index
		}
	}
	return len(run.Steps)
}

func (s *Server) testRunView(run labTestRun) labTestRunView {
	view := labTestRunView{Run: run, NextIndex: nextLabTest(run), StoragePath: s.store.Datastore().Path()}
	if view.NextIndex == len(run.Steps) {
		return view
	}
	step := run.Steps[view.NextIndex]
	if step.Preview == nil {
		view.Warning = step.PreviewError
		return view
	}
	settings, _ := s.store.Snapshot()
	if motion.LabSettingsKey(settings.Motion) != step.Preview.SettingsKey {
		view.Warning = "Saved limits changed. You can review the captured result, or create a new sequence to audition with current limits."
		return view
	}
	// A compiler update must not play a different curve under an old picture.
	current, err := testStepPreview(step.Source)
	if err != nil || !sameTestPreview(current, step.Preview) {
		view.Warning = "Motion output changed since this test was captured. Create a new sequence to audition the updated output."
		return view
	}
	view.CanAudition = true
	return view
}

func sameTestPreview(a, b *motion.FlowPreview) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}

func (s *Server) handleLabTestRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.readLabTestRuns(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	type summary struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		CreatedAt string `json:"created_at"`
		Completed int    `json:"completed"`
		Total     int    `json:"total"`
	}
	rows := make([]summary, 0, len(runs))
	for _, run := range runs {
		rows = append(rows, summary{run.ID, run.Title, run.CreatedAt, nextLabTest(run), len(run.Steps)})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"runs": rows, "storage_path": s.store.Datastore().Path(), "capacity": maximumLabTestRuns})
}

func (s *Server) handleLabTestRun(w http.ResponseWriter, r *http.Request) {
	runs, err := s.readLabTestRuns(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	for _, run := range runs {
		if run.ID == r.PathValue("id") {
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, http.StatusOK, s.testRunView(run))
			return
		}
	}
	writeError(w, http.StatusNotFound, errLabTestNotFound)
}

func (s *Server) handleCreateLabTestRun(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body labTestCreateRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	run, err := s.prepareLabTestRun(body)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	_, err = s.updateLabTestRuns(r.Context(), func(runs []labTestRun) ([]labTestRun, error) {
		if len(runs) >= maximumLabTestRuns {
			return nil, errors.New("test sequences are full; export and remove one before creating another")
		}
		return append([]labTestRun{run}, runs...), nil
	})
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, s.testRunView(run))
}

func (s *Server) prepareLabTestRun(body labTestCreateRequest) (labTestRun, error) {
	if err := s.expandLabTestPreset(&body); err != nil {
		return labTestRun{}, err
	}
	if strings.TrimSpace(body.Title) == "" || utf8.RuneCountInString(body.Title) > 120 || len(body.Steps) < 1 || len(body.Steps) > 12 {
		return labTestRun{}, errors.New("provide a test title of 1 to 120 characters and 1 to 12 steps")
	}
	run := labTestRun{ID: rand.Text(), Title: strings.TrimSpace(body.Title), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Version: s.version.Version, Commit: s.version.Commit, Revision: 1}
	for _, request := range body.Steps {
		step, err := s.prepareLabTestStep(request)
		if err != nil {
			return labTestRun{}, err
		}
		run.Steps = append(run.Steps, step)
	}
	return run, nil
}

func (s *Server) prepareLabTestStep(request labTestStepRequest) (labTestStep, error) {
	if strings.TrimSpace(request.Title) == "" || utf8.RuneCountInString(request.Title) > 120 || strings.TrimSpace(request.Instruction) == "" || utf8.RuneCountInString(request.Instruction) > 1000 {
		return labTestStep{}, errors.New("each test needs a title and an instruction of up to 1000 characters")
	}
	request.Target.Text = request.Instruction
	source, err := s.prepareLabObservation(request.Target)
	if err != nil {
		return labTestStep{}, err
	}
	if request.Phase != "" && request.Phase != "before" && request.Phase != "after" {
		return labTestStep{}, errors.New("unknown test phase")
	}
	if request.Phase == "before" && source.Trial != nil {
		source.Spec = source.Trial.Before
	}
	step := labTestStep{ID: rand.Text(), Title: strings.TrimSpace(request.Title), Instruction: strings.TrimSpace(request.Instruction), Phase: request.Phase, Source: source}
	if source.Trial != nil && !source.Trial.Valid && request.Phase != "before" {
		step.PreviewError = "This reply was rejected. Review the response and explain what failed; it cannot be auditioned."
		return step, nil
	}
	step.Preview, err = testStepPreview(source)
	return step, err
}

func testStepPreview(source labObservation) (*motion.FlowPreview, error) {
	method := source.Method
	if source.Source == "llm" {
		method = "flow"
	}
	preview, err := motion.PreviewFlow(source.Spec, source.Settings, method != "flow")
	if err != nil {
		return nil, err
	}
	for _, candidate := range preview.Candidates {
		if candidate.Method == method {
			preview.Candidates = []motion.LabCandidate{candidate}
			return &preview, nil
		}
	}
	return nil, errors.New("this generator cannot use the captured range")
}

type labTestFeedbackRequest struct {
	Revision uint64 `json:"revision"`
	StepID   string `json:"step_id"`
	Rating   int    `json:"rating"`
	Basis    string `json:"basis"`
	Comment  string `json:"comment"`
}

func (body labTestFeedbackRequest) valid() bool {
	if body.Rating < 0 || body.Rating > 3 || utf8.RuneCountInString(body.Comment) > 2000 || (body.Rating == 0) != (body.Basis == "skipped") {
		return false
	}
	switch body.Basis {
	case "preview", "device", "simulation", "reply", "skipped":
		return true
	default:
		return false
	}
}

func (s *Server) handleLabTestFeedback(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body labTestFeedbackRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	body.Comment = strings.TrimSpace(body.Comment)
	if !body.valid() {
		writeError(w, http.StatusUnprocessableEntity, errors.New("choose a rating and review basis, or skip this test; comments may contain up to 2000 characters"))
		return
	}
	// Advancing never silently leaves an audition running behind the next card.
	// This is an admission check only; Stop continues to use the shared path.
	s.motion.lifecycleMu.Lock()
	defer s.motion.lifecycleMu.Unlock()
	if engine := s.currentMotionEngine(); engine != nil {
		snapshot := engine.Snapshot()
		if snapshot.Running || snapshot.Paused {
			writeError(w, http.StatusConflict, errors.New("stop motion before saving and moving to the next test"))
			return
		}
	}
	var saved labTestRun
	_, err := s.updateLabTestRuns(r.Context(), func(runs []labTestRun) ([]labTestRun, error) {
		for index := range runs {
			run := &runs[index]
			if run.ID != r.PathValue("id") {
				continue
			}
			next := nextLabTest(*run)
			if run.Revision != body.Revision || next == len(run.Steps) || run.Steps[next].ID != body.StepID {
				return nil, errLabTestChanged
			}
			if run.Steps[next].Preview == nil && body.Basis != "reply" && body.Basis != "skipped" {
				return nil, errors.New("a rejected reply has no motion output to review")
			}
			if body.Basis == "reply" && (run.Steps[next].Source.Trial == nil || run.Steps[next].Phase == "before") {
				return nil, errors.New("this round reviews motion rather than an LLM reply")
			}
			run.Steps[next].Feedback = &labTestFeedback{Rating: body.Rating, Basis: body.Basis, Comment: body.Comment, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
			run.Revision++
			saved = *run
			return runs, nil
		}
		return nil, errLabTestNotFound
	})
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.testRunView(saved))
}

func (s *Server) handleDeleteLabTestRun(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	_, err := s.updateLabTestRuns(r.Context(), func(runs []labTestRun) ([]labTestRun, error) {
		for index, run := range runs {
			if run.ID == r.PathValue("id") {
				return append(runs[:index], runs[index+1:]...), nil
			}
		}
		return nil, errLabTestNotFound
	})
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
