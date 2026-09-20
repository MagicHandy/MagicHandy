package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
)

func TestChatPreviewAndAuthenticatedFullContentDownload(t *testing.T) {
	s, _, _, cookie := newControllerSessionFixture(t)
	id, err := s.chatLog.ActiveSessionID()
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Repeat("<synthetic>界\n", 16000)
	seq, err := s.chatLog.AppendTo(id, chat.MessageRoleAssistant, content, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	content = strings.TrimSpace(content) // append's longstanding canonical normalization
	route := fmt.Sprintf("/api/chat/messages/%d/content?session_id=%s&revision=1", seq, id)
	response := authenticatedControlRequest(s, cookie, http.MethodGet, "/api/chat/messages?after_revision=0", "reader", "")
	var page chat.MessagePage
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || response.Body.Len() > chat.MessagePageMaxBytes || len(page.Messages) != 1 || !page.Messages[0].ContentTruncated {
		t.Fatalf("history preview: %d, %d bytes", response.Code, response.Body.Len())
	}
	response = authenticatedControlRequest(s, cookie, http.MethodGet, route, "reader", "")
	if response.Code != http.StatusOK || response.Body.String() != content || !strings.HasPrefix(response.Header().Get("Content-Disposition"), "attachment;") || response.Header().Get("Content-Type") != "text/plain; charset=utf-8" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("full message download differs from canonical text or lacks safe download headers")
	}
	unauthenticated := httptest.NewRecorder()
	s.Handler().ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, route, nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unprotected download: %d", unauthenticated.Code)
	}
	for _, bad := range []string{strings.Replace(route, "revision=1", "revision=2", 1), strings.Replace(route, "session_id="+id, "session_id=another-conversation", 1)} {
		if response := authenticatedControlRequest(s, cookie, http.MethodGet, bad, "reader", ""); response.Code != http.StatusNotFound {
			t.Fatalf("mismatched message identity: %d", response.Code)
		}
	}
	pending, err := s.chatLog.AppendPendingAssistantTo(id, "private pending line", nil)
	if err != nil {
		t.Fatal(err)
	}
	if response := authenticatedControlRequest(s, cookie, http.MethodGet, fmt.Sprintf("/api/chat/messages/%d/content?session_id=%s&revision=1", pending, id), "reader", ""); response.Code != http.StatusNotFound {
		t.Fatal("pending row was downloaded")
	}
}

type chatChunkProbe struct {
	*httptest.ResponseRecorder
	onWrite func([]byte)
}

func (w chatChunkProbe) Write(data []byte) (int, error) {
	w.onWrite(data)
	return w.ResponseRecorder.Write(data)
}

func TestChatDownloadReleasesDatabaseBeforeWritingAndAbortsOnDeletion(t *testing.T) {
	s := newTestServer(t)
	t.Cleanup(s.Close)
	id, err := s.chatLog.ActiveSessionID()
	if err != nil {
		t.Fatal(err)
	}
	seq, err := s.chatLog.Append(chat.MessageRoleAssistant, strings.Repeat("large content\n", 20000), "")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/?session_id="+id+"&revision=1", nil)
	r.SetPathValue("seq", fmt.Sprint(seq))
	writes := 0
	w := chatChunkProbe{ResponseRecorder: httptest.NewRecorder(), onWrite: func(data []byte) {
		writes++
		if len(data) > chat.MessageContentChunkBytes {
			t.Fatal("unbounded download write")
		}
		// One pool connection proves no query/transaction is held across Write.
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		if _, err := s.store.Datastore().SQL().ExecContext(ctx, `DELETE FROM messages WHERE seq = ?`, seq); err != nil {
			t.Fatalf("write held a database connection: %v", err)
		}
	}}
	s.store.Datastore().SQL().SetMaxOpenConns(1)
	defer func() {
		if recovered := recover(); recovered != http.ErrAbortHandler {
			t.Fatalf("deleted content did not abort transfer: %v", recovered)
		}
		if writes != 1 || w.Body.Len() != chat.MessageContentChunkBytes {
			t.Fatalf("invalid partial transfer: %d writes, %d bytes", writes, w.Body.Len())
		}
	}()
	s.handleChatMessageContent(w, r)
}

func TestChatHTTPContinuationsAreBoundedAndValidated(t *testing.T) {
	s, _, _, cookie := newControllerSessionFixture(t)
	for index := 0; index < 3; index++ {
		if _, err := s.chatLog.Append(chat.MessageRoleUser, fmt.Sprint(index), ""); err != nil {
			t.Fatal(err)
		}
	}
	response := authenticatedControlRequest(s, cookie, http.MethodGet, "/api/chat/messages?after_revision=0&limit=1", "reader", "")
	var first chat.MessagePage
	if err := json.Unmarshal(response.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.Snapshot == nil || first.NextRevision != 1 || !first.Reset {
		t.Fatalf("initial reset: %+v", first)
	}
	route := fmt.Sprintf("/api/chat/messages?after_revision=%d&snapshot_revision=%d&snapshot_pruned_revision=%d&snapshot_first_seq=%d&limit=1", first.NextRevision, first.Snapshot.Revision, first.Snapshot.PrunedRevision, first.Snapshot.FirstSeq)
	response = authenticatedControlRequest(s, cookie, http.MethodGet, route, "reader", "")
	var next chat.MessagePage
	if err := json.Unmarshal(response.Body.Bytes(), &next); err != nil {
		t.Fatal(err)
	}
	if next.Reset || next.NextRevision != 2 || next.Snapshot == nil {
		t.Fatalf("HTTP restarted snapshot: %+v", next)
	}
	for _, query := range []string{"snapshot_revision=3", "after_revision=1&snapshot_revision=3", "after_revision=1&snapshot_revision=3&snapshot_pruned_revision=-1", "after_revision=1&snapshot_revision=3&snapshot_pruned_revision=4"} {
		if response := authenticatedControlRequest(s, cookie, http.MethodGet, "/api/chat/messages?"+query, "reader", ""); response.Code != http.StatusBadRequest {
			t.Fatalf("invalid continuation accepted: %s", query)
		}
	}
}
