package httpapi

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/store"
)

const maxPersonaAvatarBytes = 4 << 20

type personaWriteRequest struct {
	Name           string  `json:"name"`
	Description    *string `json:"description"`
	SystemPrompt   string  `json:"system_prompt"`
	ToneJSON       *string `json:"tone_json"`
	MoodJSON       *string `json:"mood_json"`
	BoundariesJSON *string `json:"boundaries_json"`
	MotionBiasJSON *string `json:"motion_bias_json"`
}

func (s *Server) registerPersonaRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/personas", s.handlePersonasList)
	mux.HandleFunc("POST /api/personas", s.handlePersonaCreate)
	mux.HandleFunc("GET /api/personas/{id}", s.handlePersonaGet)
	mux.HandleFunc("PUT /api/personas/{id}", s.handlePersonaUpdate)
	mux.HandleFunc("DELETE /api/personas/{id}", s.handlePersonaDelete)
	mux.HandleFunc("POST /api/personas/{id}/activate", s.handlePersonaActivate)
	mux.HandleFunc("POST /api/personas/{id}/avatar", s.handlePersonaAvatarUpload)
	mux.HandleFunc("POST /api/import/lso", s.handleImportLSO)
	mux.HandleFunc("GET /media/{path...}", s.handleMedia)
}

func (s *Server) handlePersonasList(w http.ResponseWriter, _ *http.Request) {
	db := s.store.DB()
	rows, err := db.ListPersonas()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	state, err := db.LoadAppState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	activeID := s.resolveActivePersonaID(state.ActivePersonaID, rows)
	payload := make([]store.PersonaPayload, 0, len(rows))
	for _, row := range rows {
		payload = append(payload, store.PersonaPayloadFromRow(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"personas":            payload,
		"active_persona_id":   activeID,
	})
}

func (s *Server) handlePersonaGet(w http.ResponseWriter, r *http.Request) {
	row, err := s.store.DB().GetPersona(r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrPersonaNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, store.PersonaPayloadFromRow(row))
}

func (s *Server) handlePersonaCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body personaWriteRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	row, err := s.store.DB().SavePersona("", personaWriteFromRequest(body))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, store.PersonaPayloadFromRow(row))
}

func (s *Server) handlePersonaUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	id := r.PathValue("id")
	if _, err := s.store.DB().GetPersona(id); err != nil {
		if errors.Is(err, store.ErrPersonaNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var body personaWriteRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	row, err := s.store.DB().SavePersona(id, personaWriteFromRequest(body))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, store.PersonaPayloadFromRow(row))
}

func (s *Server) handlePersonaDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	id := r.PathValue("id")
	if err := s.store.DB().DeletePersona(id); err != nil {
		if errors.Is(err, store.ErrPersonaNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	state, _ := s.store.DB().LoadAppState()
	if state.ActivePersonaID == id {
		state.ActivePersonaID = ""
		_, _ = s.store.DB().SaveAppState(state)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handlePersonaActivate(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	id := r.PathValue("id")
	if _, err := s.store.DB().GetPersona(id); err != nil {
		if errors.Is(err, store.ErrPersonaNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.store.DB().SetActivePersonaID(id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                true,
		"active_persona_id": id,
	})
}

func (s *Server) handlePersonaAvatarUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	id := r.PathValue("id")
	if _, err := s.store.DB().GetPersona(id); err != nil {
		if errors.Is(err, store.ErrPersonaNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := r.ParseMultipartForm(maxPersonaAvatarBytes); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid upload"))
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("image file is required"))
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxPersonaAvatarBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(data) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("empty upload"))
		return
	}
	if len(data) > maxPersonaAvatarBytes {
		writeError(w, http.StatusBadRequest, errors.New("image too large (max 4 MB)"))
		return
	}

	saved, err := savePersonaAvatar(s.store.DataDir(), id, data, header.Filename)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"path":       saved,
		"avatar_url": personaAvatarURLFromDir(s.store.DataDir(), id),
	})
}

func (s *Server) handleImportLSO(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	lsoPath := strings.TrimSpace(r.URL.Query().Get("lso_db"))
	if lsoPath == "" {
		candidates := store.DefaultLSODatabaseCandidates()
		if len(candidates) == 0 {
			writeError(w, http.StatusBadRequest, errors.New("LSO database not found; pass ?lso_db= path"))
			return
		}
		lsoPath = candidates[0]
	}
	result, err := store.ImportFromLSOWithOptions(s.store.DB(), lsoPath, store.LSOImportOptions{
		LSODataDir: store.ResolveLSODataDir(lsoPath),
		TargetDir:  s.store.DataDir(),
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                  true,
		"lso_db":              lsoPath,
		"personas":            result.Personas,
		"funscript_files":     result.FunscriptFiles,
		"motion_blocks":       result.MotionBlocks,
		"saved_queues":        result.SavedQueues,
		"persona_media_files": result.PersonaMediaFiles,
		"active_persona_id":   result.ActivePersonaID,
	})
}

func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/media/")
	rel = filepath.Clean(strings.ReplaceAll(rel, "\\", "/"))
	if rel == "." || strings.HasPrefix(rel, "..") {
		http.NotFound(w, r)
		return
	}
	full := filepath.Join(s.store.DataDir(), filepath.FromSlash(rel))
	dataDir := s.store.DataDir()
	absData, err := filepath.Abs(dataDir)
	if err != nil {
		http.Error(w, "media unavailable", http.StatusInternalServerError)
		return
	}
	absFull, err := filepath.Abs(full)
	if err != nil || !strings.HasPrefix(absFull, absData+string(os.PathSeparator)) {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, absFull)
}

func (s *Server) activePersonaSnapshot() (store.PersonaRow, string, error) {
	db := s.store.DB()
	rows, err := db.ListPersonas()
	if err != nil {
		return store.PersonaRow{}, "", err
	}
	state, err := db.LoadAppState()
	if err != nil {
		return store.PersonaRow{}, "", err
	}
	activeID := s.resolveActivePersonaID(state.ActivePersonaID, rows)
	if activeID == "" {
		return store.PersonaRow{}, "", nil
	}
	row, err := db.GetPersona(activeID)
	if err != nil {
		return store.PersonaRow{}, activeID, nil
	}
	return row, activeID, nil
}

func (s *Server) resolveActivePersonaID(configured string, rows []store.PersonaRow) string {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		for _, row := range rows {
			if row.ID == configured {
				return configured
			}
		}
	}
	if len(rows) == 0 {
		return ""
	}
	for _, preferred := range []string{"persona_calm", rows[0].ID} {
		for _, row := range rows {
			if row.ID == preferred {
				return preferred
			}
		}
	}
	return rows[0].ID
}

func personaWriteFromRequest(body personaWriteRequest) store.PersonaWrite {
	return store.PersonaWrite{
		Name:           body.Name,
		Description:    body.Description,
		SystemPrompt:   body.SystemPrompt,
		ToneJSON:       body.ToneJSON,
		MoodJSON:       body.MoodJSON,
		BoundariesJSON: body.BoundariesJSON,
		MotionBiasJSON: body.MotionBiasJSON,
	}
}

func savePersonaAvatar(dataDir, personaID string, data []byte, filename string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp":
	default:
		ext = ".png"
	}
	refsDir := filepath.Join(dataDir, "personas", personaID, "refs")
	if err := os.MkdirAll(refsDir, 0o700); err != nil {
		return "", err
	}
	for _, old := range []string{"portrait.png", "portrait.jpg", "portrait.jpeg", "portrait.webp"} {
		_ = os.Remove(filepath.Join(refsDir, old))
	}
	dest := filepath.Join(refsDir, "portrait"+ext)
	if err := os.WriteFile(dest, data, 0o600); err != nil {
		return "", err
	}
	return dest, nil
}

func personaAvatarURLFromDir(dataDir, personaID string) *string {
	if strings.TrimSpace(personaID) == "" {
		return nil
	}
	refsDir := filepath.Join(dataDir, "personas", personaID, "refs")
	for _, name := range []string{"portrait.png", "portrait.jpg", "portrait.jpeg", "portrait.webp"} {
		candidate := filepath.Join(refsDir, name)
		if _, err := os.Stat(candidate); err == nil {
			url := "/media/personas/" + personaID + "/refs/" + name
			return &url
		}
	}
	entries, err := os.ReadDir(refsDir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".webp" {
			url := "/media/personas/" + personaID + "/refs/" + entry.Name()
			return &url
		}
	}
	return nil
}

func (s *Server) personaAvatarURLFor(personaID string) *string {
	return personaAvatarURLFromDir(s.store.DataDir(), personaID)
}

func (s *Server) chatPromptForRequest(settings config.Settings) chat.PromptSet {
	if persona, _, err := s.activePersonaSnapshot(); err == nil && persona.ID != "" {
		return chat.PromptSet{
			ID:     "persona:" + persona.ID,
			Name:   persona.Name,
			System: persona.SystemPrompt,
		}
	}
	prompt, ok := s.personalization.prompts.Resolve(settings.LLM.PromptSet)
	if ok {
		return prompt
	}
	prompt, _ = chat.BuiltinPromptSetByID(chat.DefaultPromptSetID)
	return prompt
}

func (s *Server) tryAutoImportLSO() {
	result, source, err := store.AutoImportLSOIfEmpty(s.store.DB())
	if err != nil {
		s.logger.Warn("LSO persona import failed", "error", err)
		return
	}
	if source == "" {
		return
	}
	s.logger.Info(
		"imported personas from LSO",
		"source", source,
		"personas", result.Personas,
		"media_files", result.PersonaMediaFiles,
		"active_persona_id", result.ActivePersonaID,
	)
}
