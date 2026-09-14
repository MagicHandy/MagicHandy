package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/mapledaemon/MagicHandy/internal/chat"
)

// A browser download streams the unchanged stored UTF-8 text rather than
// materializing it in the JSON history, a Blob, or the rendered conversation.
func (s *Server) handleChatMessageContent(w http.ResponseWriter, r *http.Request) {
	seq, seqErr := strconv.ParseInt(r.PathValue("seq"), 10, 64)
	revision, revisionErr := strconv.ParseInt(r.URL.Query().Get("revision"), 10, 64)
	sessionID := r.URL.Query().Get("session_id")
	if seqErr != nil || revisionErr != nil || seq <= 0 || revision <= 0 || sessionID == "" {
		writeError(w, http.StatusBadRequest, errors.New("a conversation, message and committed revision are required"))
		return
	}
	chunk, err := s.chatLog.ReadMessageContentChunkContext(r.Context(), sessionID, seq, revision, 0)
	if err != nil {
		if errors.Is(err, chat.ErrChatMessageNotFound) {
			writeError(w, http.StatusNotFound, err)
		} else {
			s.writeChatStorageError(w, err)
		}
		return
	}
	total := chunk.TotalBytes
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="chat-message-%d.txt"`, seq))
	w.Header().Set("Content-Length", strconv.FormatInt(total, 10))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method == http.MethodHead {
		return
	}
	for offset := int64(0); offset < total; {
		if len(chunk.Data) == 0 || chunk.TotalBytes != total || int64(len(chunk.Data)) > total-offset {
			panic(http.ErrAbortHandler)
		}
		if err := writeContextBytes(r.Context(), w, chunk.Data); err != nil {
			panic(http.ErrAbortHandler)
		}
		offset += int64(len(chunk.Data))
		if offset < total {
			chunk, err = s.chatLog.ReadMessageContentChunkContext(r.Context(), sessionID, seq, revision, offset)
			// Retention/deletion during a download must produce an incomplete
			// transfer, never a successful shortened file or a JSON error suffix.
			if err != nil {
				panic(http.ErrAbortHandler)
			}
		}
	}
}
