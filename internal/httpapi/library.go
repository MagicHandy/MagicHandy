package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/patterns"
)

const maxAuthoringRequestBytes = 1 << 20

func (s *Server) libraryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/library", s.handleLibraryGet)
	mux.HandleFunc("POST /api/library/preview", s.handleLibraryPreview)
	mux.HandleFunc("POST /api/library/import", s.handleLibraryImport)
	mux.HandleFunc("POST /api/library/patterns", s.handleLibraryPatternCreate)
	mux.HandleFunc("PATCH /api/library/patterns/{id}", s.handleLibraryPatternPatch)
	mux.HandleFunc("DELETE /api/library/patterns/{id}", s.handleLibraryPatternDelete)
	mux.HandleFunc("GET /api/library/patterns/{id}/export", s.handleLibraryPatternExport)
	mux.HandleFunc("POST /api/library/patterns/{id}/play", s.handleLibraryPatternPlay)
	mux.HandleFunc("DELETE /api/library/programs/{id}", s.handleLibraryProgramDelete)
	mux.HandleFunc("GET /api/library/programs/{id}/export", s.handleLibraryProgramExport)
	mux.HandleFunc("POST /api/library/programs/{id}/play", s.handleLibraryProgramPlay)
	mux.HandleFunc("POST /api/library/feedback", s.handleLibraryFeedback)
	mux.HandleFunc("POST /api/library/feedback/{id}/undo", s.handleLibraryFeedbackUndo)
	mux.HandleFunc("PUT /api/library/auto-disable", s.handleLibraryAutoDisable)
}

func (s *Server) libraryState() patterns.Summary {
	if s.patterns == nil {
		return patterns.Summary{}
	}
	return s.patterns.Summary()
}

func (s *Server) handleLibraryGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"library": s.patterns.Snapshot()})
}

func (s *Server) handleLibraryPreview(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var input patterns.PatternInput
	if err := decodeLimitedJSON(w, r, &input, maxAuthoringRequestBytes); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	preview, err := patterns.PreviewPattern(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"preview": preview})
}

func (s *Server) handleLibraryPatternCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var input patterns.PatternInput
	if err := decodeLimitedJSON(w, r, &input, maxAuthoringRequestBytes); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	pattern, err := s.patterns.CreatePattern(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"pattern": pattern})
}

func (s *Server) handleLibraryPatternPatch(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var patch patterns.PatternPatch
	if err := decodeLimitedJSON(w, r, &patch, maxAuthoringRequestBytes); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	pattern, err := s.patterns.UpdatePattern(r.PathValue("id"), patch)
	if err != nil {
		s.writeLibraryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pattern": pattern})
}

func (s *Server) handleLibraryPatternDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	if err := s.patterns.DeletePattern(r.PathValue("id")); err != nil {
		s.writeLibraryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLibraryProgramDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	if err := s.patterns.DeleteProgram(r.PathValue("id")); err != nil {
		s.writeLibraryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLibraryPatternExport(w http.ResponseWriter, r *http.Request) {
	data, filename, err := s.patterns.ExportPattern(r.PathValue("id"))
	if err != nil {
		s.writeLibraryError(w, err)
		return
	}
	writeDownload(w, filename, data)
}

func (s *Server) handleLibraryProgramExport(w http.ResponseWriter, r *http.Request) {
	data, filename, err := s.patterns.ExportProgram(r.PathValue("id"))
	if err != nil {
		s.writeLibraryError(w, err)
		return
	}
	writeDownload(w, filename, data)
}

func (s *Server) handleLibraryImport(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, patterns.MaxImportBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, errors.New("motion content file exceeds 8 MiB"))
			return
		}
		writeError(w, http.StatusBadRequest, errors.New("motion content file could not be read"))
		return
	}
	filename := strings.TrimSpace(r.URL.Query().Get("filename"))
	if filename == "" || len(filename) > 255 {
		writeError(w, http.StatusBadRequest, errors.New("import filename is required"))
		return
	}
	result, err := s.patterns.Import(filename, data, r.URL.Query().Get("as"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"import": result})
}

type libraryPlayRequest struct {
	Intensity int    `json:"intensity,omitempty"`
	Feel      string `json:"feel,omitempty"`
}

func (s *Server) handleLibraryPatternPlay(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	stopSequence := s.stopSequence.Load()
	request, ok := decodeLibraryPlayRequest(w, r)
	if !ok {
		return
	}
	definition, found := s.patterns.ResolveEnabled(r.PathValue("id"))
	if !found {
		writeError(w, http.StatusConflict, errors.New("pattern is disabled or unavailable"))
		return
	}
	definition = patterns.AuditionDefinition(definition, request.Feel)
	target := motion.MotionTarget{
		Label: definition.Name, Source: "pattern_library", PatternID: definition.ID,
		SpeedPercent: request.Intensity, Pattern: &definition,
	}
	state, err := s.playLibraryPattern(r, target, stopSequence)
	s.writeMotionResult(w, state, err)
}

func (s *Server) handleLibraryProgramPlay(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	stopSequence := s.stopSequence.Load()
	request, ok := decodeLibraryPlayRequest(w, r)
	if !ok {
		return
	}
	program, err := s.patterns.Program(r.PathValue("id"))
	if err != nil {
		s.writeLibraryError(w, err)
		return
	}
	definition := program.Definition()
	target := motion.MotionTarget{
		Label: definition.Name, Source: "program_player", ProgramID: definition.ID,
		SpeedPercent: request.Intensity, Program: &definition,
	}
	state, err := s.playLibraryProgram(r, target, stopSequence)
	s.writeMotionResult(w, state, err)
}

func (s *Server) playLibraryPattern(r *http.Request, target motion.MotionTarget, stopSequence uint64) (motion.ActiveMotionState, error) {
	if s.modes != nil {
		s.modes.NotifyUserStop()
	}
	engine, admission, err := s.motionEngineForStart()
	if err != nil {
		return motion.ActiveMotionState{}, err
	}
	current := engine.Snapshot()
	if current.Paused {
		return current, errors.New("stop or resume paused motion before playing a pattern")
	}
	if current.Running {
		if s.stopSequence.Load() != stopSequence {
			return current, errors.New("pattern play was invalidated by Emergency Stop")
		}
		return engine.ApplyTarget(r.Context(), target, "library_pattern")
	}
	settings, _ := s.store.Snapshot()
	if s.stopSequence.Load() != stopSequence {
		return engine.Snapshot(), errors.New("pattern play was invalidated by Emergency Stop")
	}
	return engine.StartAtGeneration(r.Context(), target, settings.Motion, admission)
}

func (s *Server) playLibraryProgram(r *http.Request, target motion.MotionTarget, stopSequence uint64) (motion.ActiveMotionState, error) {
	if s.modes != nil {
		s.modes.NotifyUserStop()
	}
	engine, admission, err := s.motionEngineForStart()
	if err != nil {
		return motion.ActiveMotionState{}, err
	}
	current := engine.Snapshot()
	if current.Running || current.Paused {
		if s.stopSequence.Load() != stopSequence {
			return current, errors.New("program play was invalidated by Emergency Stop")
		}
		if _, err := engine.Stop(r.Context(), "program_player_replace"); err != nil {
			return engine.Snapshot(), err
		}
		admission, err = s.motionAdmissionFor(engine)
		if err != nil {
			return engine.Snapshot(), err
		}
	}
	settings, _ := s.store.Snapshot()
	if s.stopSequence.Load() != stopSequence {
		return engine.Snapshot(), errors.New("program play was invalidated by Emergency Stop")
	}
	return engine.StartAtGeneration(r.Context(), target, settings.Motion, admission)
}

func (s *Server) handleLibraryFeedback(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		PatternID string `json:"pattern_id"`
		Rating    int    `json:"rating"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	feedback, pattern, err := s.patterns.ApplyFeedback(body.PatternID, body.Rating)
	if err != nil {
		s.writeLibraryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"feedback": feedback, "pattern": pattern})
}

func (s *Server) handleLibraryFeedbackUndo(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("feedback id must be positive"))
		return
	}
	feedback, pattern, err := s.patterns.UndoFeedback(id)
	if err != nil {
		s.writeLibraryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"feedback": feedback, "pattern": pattern})
}

func (s *Server) handleLibraryAutoDisable(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.patterns.SetAutoDisable(body.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("auto-disable preference could not be saved"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"auto_disable": body.Enabled})
}

func decodeLibraryPlayRequest(w http.ResponseWriter, r *http.Request) (libraryPlayRequest, bool) {
	request := libraryPlayRequest{Intensity: 30}
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &request); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return libraryPlayRequest{}, false
		}
	}
	if request.Intensity < 1 || request.Intensity > 100 {
		writeError(w, http.StatusBadRequest, errors.New("intensity must be between 1 and 100"))
		return libraryPlayRequest{}, false
	}
	request.Feel = strings.ToLower(strings.TrimSpace(request.Feel))
	if request.Feel != "" && request.Feel != "original" && request.Feel != "smooth" && request.Feel != "crisp" {
		writeError(w, http.StatusBadRequest, errors.New("feel must be original, smooth, or crisp"))
		return libraryPlayRequest{}, false
	}
	return request, true
}

func decodeLimitedJSON(w http.ResponseWriter, r *http.Request, target any, limit int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	defer func() { _ = r.Body.Close() }()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode JSON request: %w", err)
	}
	var extra struct{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("decode JSON request: multiple JSON values are not allowed")
	}
	return nil
}

func (s *Server) writeLibraryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, patterns.ErrPatternNotFound), errors.Is(err, patterns.ErrProgramNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, patterns.ErrBuiltinPattern), errors.Is(err, patterns.ErrFeedbackOrder):
		writeError(w, http.StatusConflict, err)
	default:
		writeError(w, http.StatusBadRequest, err)
	}
}

func writeDownload(w http.ResponseWriter, filename string, data []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	// #nosec G705 -- data is validated JSON served as an attachment with
	// nosniff; it is never interpolated into an HTML response.
	_, _ = w.Write(data)
}
