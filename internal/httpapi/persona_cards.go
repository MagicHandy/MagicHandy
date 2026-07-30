package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/charcard"
	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/persona"
)

// cardImportMaxBytes bounds one uploaded card. Card PNGs carry full-size art,
// so the bound is the download bound, not the persona archive bound.
const cardImportMaxBytes = charcard.MaxFetchBytes

// cardFetchTimeout bounds one URL import end to end.
const cardFetchTimeout = 45 * time.Second

// finishCardImport converts a parsed card into a persona and answers with the
// personas payload plus any truncation warnings.
func (s *Server) finishCardImport(w http.ResponseWriter, r *http.Request, card charcard.Card, artPNG []byte) {
	unlock, ok := s.beginPersonaMutation(w)
	if !ok {
		return
	}
	defer unlock()
	payload, ok := s.preparePersonasMutation(w, r)
	if !ok {
		return
	}
	item, warnings, err := s.personas.ImportCard(r.Context(), card, artPNG)
	if err != nil {
		s.writePersonaError(w, err)
		return
	}
	upsertPersonaPayload(payload, item)
	payload["persona"] = item
	if len(warnings) > 0 {
		payload["import_warnings"] = warnings
	}
	writeJSON(w, http.StatusCreated, payload)
}

// handleCardUpload takes over /api/personas/import when the body is a card
// rather than a persona archive.
func (s *Server) handleCardUpload(w http.ResponseWriter, r *http.Request, data []byte) {
	card, err := charcard.Parse(data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var artPNG []byte
	if charcard.IsPNG(data) {
		artPNG = data
	}
	s.finishCardImport(w, r, card, artPNG)
}

// handlePersonaImportURL imports a character card published at a URL: a card
// PNG or JSON file, or a page the card can be discovered from.
func (s *Server) handlePersonaImportURL(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		URL string `json:"url"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(body.URL) == "" {
		writeError(w, http.StatusBadRequest, errors.New("a URL is required"))
		return
	}
	client := &http.Client{Timeout: cardFetchTimeout}
	result, err := charcard.Fetch(r.Context(), client, body.URL)
	if err != nil {
		switch {
		case errors.Is(err, charcard.ErrNoCardData):
			writeError(w, http.StatusUnprocessableEntity, err)
		case errors.Is(err, charcard.ErrFetchFailed):
			writeError(w, http.StatusBadGateway, err)
		default:
			writeError(w, http.StatusBadRequest, err)
		}
		return
	}
	s.finishCardImport(w, r, result.Card, result.PortraitPNG)
}

// seedPersonaGreeting opens an empty session with the persona's greeting. A
// failure is logged rather than failing the attach: the binding already
// succeeded, and a missing opening line is recoverable by typing first.
func (s *Server) seedPersonaGreeting(sessionID string, selected persona.Persona) {
	if selected.Greeting == "" {
		return
	}
	latest, err := s.chatLog.LatestSeqSession(sessionID)
	if err != nil || latest != 0 {
		return
	}
	if _, err := s.chatLog.AppendTo(sessionID, chat.MessageRoleAssistant, selected.Greeting, "", nil); err != nil {
		s.logger.Warn("persona greeting was not seeded", "session_id", sessionID, "error", err)
	}
}
