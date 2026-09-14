package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/chat"
)

func parseChatHistoryRequest(r *http.Request) (chat.MessagePageRequest, error) {
	query := r.URL.Query()
	request := chat.MessagePageRequest{SessionID: strings.TrimSpace(query.Get("session_id")), ClientID: chatCursorClientID(r)}
	for key, target := range map[string]*int64{"after": &request.AfterSequence, "after_revision": nil} {
		if value := query.Get(key); value != "" {
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil || parsed < 0 {
				return request, errors.New(key + " must be a non-negative integer")
			}
			if target != nil {
				*target = parsed
			} else {
				request.AfterRevision = &parsed
			}
		}
	}
	if request.AfterSequence > 0 && request.AfterRevision != nil {
		return request, errors.New("use either after or after_revision")
	}
	if value := query.Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			return request, errors.New("limit must be a positive integer")
		}
		request.Limit = parsed
	}
	var err error
	request.Snapshot, err = parseChatSnapshot(query, request.AfterRevision != nil)
	return request, err
}

func parseChatSnapshot(query url.Values, revisionRead bool) (*chat.MessageSnapshot, error) {
	if query.Get("snapshot_revision") == "" && query.Get("snapshot_pruned_revision") == "" && query.Get("snapshot_first_seq") == "" {
		return nil, nil
	}
	head, headErr := strconv.ParseInt(query.Get("snapshot_revision"), 10, 64)
	pruned, prunedErr := strconv.ParseInt(query.Get("snapshot_pruned_revision"), 10, 64)
	first, firstErr := strconv.ParseInt(query.Get("snapshot_first_seq"), 10, 64)
	if !revisionRead || headErr != nil || prunedErr != nil || firstErr != nil || first <= 0 || head <= 0 || pruned < 0 || pruned > head {
		return nil, errors.New("invalid chat snapshot continuation")
	}
	return &chat.MessageSnapshot{Revision: head, PrunedRevision: pruned, FirstSeq: first}, nil
}
