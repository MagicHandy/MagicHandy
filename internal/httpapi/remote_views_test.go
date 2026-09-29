package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestRemoteCatalogIsBoundedAndDisplayOnly(t *testing.T) {
	s, store, admin, desktop, _ := newRemoteFixture(t)
	_, cookie := remoteOnlyLogin(t, s, store, admin, desktop)
	names := make([]string, 62)
	for i := range names {
		names[i] = fmt.Sprintf("clip-%02d", i)
	}
	scanMetadataFixture(t, s, names...)
	response := remotePortalRequest(s, cookie, http.MethodGet, "/api/remote/videos", "")
	var page struct {
		Videos []map[string]any `json:"videos"`
		More   bool             `json:"has_more"`
		Next   int              `json:"next_offset"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || len(page.Videos) != 60 || !page.More || page.Next != 60 {
		t.Fatalf("page: %d %+v", response.Code, page)
	}
	for _, item := range page.Videos {
		if len(item) != 4 || item["id"] == nil || item["title"] == nil || item["has_funscript"] == nil {
			t.Fatalf("unexpected display fields: %v", item)
		}
	}
	response = remotePortalRequest(s, cookie, http.MethodGet, "/api/remote/videos?offset=60", "")
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Videos) != 2 || page.More || page.Next != 62 {
		t.Fatalf("next page: %+v", page)
	}
	if response := authenticatedRemoteRequest(s, desktop, http.MethodDelete, "/api/remote/presence", "test-controller", ""); response.Code != http.StatusOK {
		t.Fatalf("withdraw: %d", response.Code)
	}
	if response = remotePortalRequest(s, cookie, http.MethodGet, "/api/remote/videos", ""); response.Code != http.StatusConflict {
		t.Fatalf("catalog after disconnect: %d", response.Code)
	}
}

func TestRemoteConversationOnlyExposesBoundedCommittedActiveText(t *testing.T) {
	s, store, admin, desktop, _ := newRemoteFixture(t)
	_, cookie := remoteOnlyLogin(t, s, store, admin, desktop)
	chat, err := s.chatLog.CreateSession(true)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 42; i++ {
		if _, err := s.chatLog.AppendTo(chat.ID, "user", fmt.Sprintf("message-%02d", i), "private-browser-attribution", nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.chatLog.AppendTo(chat.ID, "assistant", strings.Repeat("界", 2050), "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.chatLog.AppendPendingAssistantTo(chat.ID, "uncommitted private text", nil); err != nil {
		t.Fatal(err)
	}
	presence := fmt.Sprintf(`{"route":"chat","chat":{"session_id":%q,"ready":true}}`, chat.ID)
	if response := authenticatedRemoteRequest(s, desktop, http.MethodPost, "/api/remote/presence", "test-controller", presence); response.Code != http.StatusOK {
		t.Fatalf("presence: %d", response.Code)
	}
	response := remotePortalRequest(s, cookie, http.MethodGet, "/api/remote/chat/messages?session_id="+chat.ID, "")
	var page struct {
		Messages []struct {
			Content   string `json:"content"`
			Truncated bool   `json:"truncated"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || len(page.Messages) != 40 || page.Messages[0].Content != "message-03" || !page.Messages[39].Truncated || len([]rune(page.Messages[39].Content)) != 2048 {
		t.Fatalf("unbounded history: %d, %d", response.Code, len(page.Messages))
	}
	for _, private := range []string{"private-browser-attribution", "uncommitted private text", "diagnostics", "client_id"} {
		if strings.Contains(response.Body.String(), private) {
			t.Fatalf("leaked %s", private)
		}
	}
	if response = remotePortalRequest(s, cookie, http.MethodGet, "/api/remote/chat/messages?session_id=another", ""); response.Code != http.StatusConflict {
		t.Fatalf("arbitrary history admitted: %d", response.Code)
	}
}
