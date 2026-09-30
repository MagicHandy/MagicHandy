package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mapledaemon/MagicHandy/internal/remote"
)

func (s *Server) remoteState(r *http.Request) remote.State {
	state, _ := s.remote.State(remoteIdentity(r, clientIDFromRequest(r)))
	state.StopSequence = s.stopSequence.Load()
	state.CanControl = s.capabilities(r).RemoteControl
	return state
}

func (s *Server) remoteView(w http.ResponseWriter, r *http.Request) (remote.State, bool) {
	if !s.requireRemoteControl(w, r) {
		return remote.State{}, false
	}
	if !s.remoteSenderActive(r, remoteIdentity(r, clientIDFromRequest(r))) {
		s.writeAuthenticationRequired(w)
		return remote.State{}, false
	}
	state := s.remoteState(r)
	if !state.Connected || state.OtherAccount {
		writeError(w, http.StatusConflict, remote.ErrNoDesktop)
		return state, false
	}
	return state, true
}

func (s *Server) handleRemoteVideos(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.remoteView(w, r); !ok {
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	offset := 0
	var err error
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
	}
	if err != nil || offset < 0 || offset > 1000000 || !utf8.ValidString(query) || utf8.RuneCountInString(query) > 200 {
		writeError(w, http.StatusBadRequest, errors.New("invalid catalog query"))
		return
	}
	items, more, err := s.media.RemoteVideos(r.Context(), query, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("video catalog is unavailable"))
		return
	}
	if _, ok := s.remoteView(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"videos": items, "has_more": more, "next_offset": offset + len(items)})
}

func (s *Server) handleRemoteMessages(w http.ResponseWriter, r *http.Request) {
	state, ok := s.remoteView(w, r)
	if !ok {
		return
	}
	id := r.URL.Query().Get("session_id")
	if state.Chat == nil || id == "" || state.Chat.SessionID != id {
		writeError(w, http.StatusConflict, remote.ErrTargetUnavailable)
		return
	}
	items, err := s.chatLog.RecentDisplayMessages(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("conversation is unavailable"))
		return
	}
	state, ok = s.remoteView(w, r)
	if !ok {
		return
	}
	if state.Chat == nil || state.Chat.SessionID != id {
		writeError(w, http.StatusConflict, remote.ErrTargetUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session_id": id, "messages": items})
}
