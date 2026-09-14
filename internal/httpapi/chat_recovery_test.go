package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
	"github.com/mapledaemon/MagicHandy/internal/chat"
)

func TestChatCursorIsBoundToLoginAndBrowser(t *testing.T) {
	for _, actor := range []string{"another-login", "another-account", "another-tab"} {
		t.Run(actor, func(t *testing.T) {
			s, store, admin, firstCookie := newControllerSessionFixture(t)
			otherCookie, otherTab := firstCookie, "reader"
			if actor == "another-tab" {
				otherTab = "another-reader"
			} else {
				account := admin
				if actor == "another-account" {
					var err error
					account, err = store.Create(t.Context(), "observer", "synthetic cursor observer", accounts.RoleOperator)
					if err != nil {
						t.Fatal(err)
					}
				}
				token, _, err := store.NewSession(t.Context(), account.ID)
				if err != nil {
					t.Fatal(err)
				}
				otherCookie = testSessionCookie(token)
			}
			seq, err := s.chatLog.Append(chat.MessageRoleUser, "synthetic read tracking", "seed")
			if err != nil {
				t.Fatal(err)
			}
			body := fmt.Sprintf(`{"seq":%d,"revision":1,"server_epoch":%q}`, seq, s.controller.epoch)
			response := authenticatedControlRequest(s, otherCookie, http.MethodPost, "/api/chat/cursor", otherTab, body)
			if response.Code != http.StatusOK {
				t.Fatalf("observer cursor request: %d %s", response.Code, response.Body.String())
			}
			read := func(cookie *http.Cookie, tab string) chat.MessagePage {
				t.Helper()
				response := authenticatedControlRequest(s, cookie, http.MethodGet, "/api/chat/messages?after_revision=0", tab, "")
				var page chat.MessagePage
				if response.Code != http.StatusOK {
					t.Fatalf("read status: %d", response.Code)
				}
				if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
					t.Fatal(err)
				}
				return page
			}
			if page := read(firstCookie, "reader"); page.Cursor != 0 || page.CursorRevision != 0 {
				t.Fatal("another login/tab advanced this browser's cursor")
			}
			if page := read(otherCookie, otherTab); page.Cursor != seq || page.CursorRevision != 1 {
				t.Fatal("observer could not advance its own cursor")
			}
			var key string
			if err := s.store.Datastore().SQL().QueryRow(`SELECT client_id FROM chat_session_cursors LIMIT 1`).Scan(&key); err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(key, "login:") || strings.Contains(key, firstCookie.Value) || strings.Contains(key, otherCookie.Value) {
				t.Fatal("cursor persistence contains an invalid login identity")
			}
		})
	}
}

func TestChatRecoveryHTTPRejectsObsoleteMetadataAndFindsLateCommit(t *testing.T) {
	s, _, _, cookie := newControllerSessionFixture(t)
	id, err := s.chatLog.ActiveSessionID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.chatLog.Append(chat.MessageRoleUser, "first", "seed"); err != nil {
		t.Fatal(err)
	}
	pending, err := s.chatLog.AppendPendingAssistantTo(id, "committed later", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.chatLog.Append(chat.MessageRoleUser, "newer", "seed"); err != nil {
		t.Fatal(err)
	}
	if err := s.chatLog.CommitPending(pending); err != nil {
		t.Fatal(err)
	}
	response := authenticatedControlRequest(s, cookie, http.MethodGet, "/api/chat/messages?after_revision=2", "reader", "")
	var page struct {
		chat.MessagePage
		ServerEpoch string `json:"server_epoch"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || page.ServerEpoch != s.controller.epoch || page.NextRevision != 3 || len(page.Messages) != 1 || page.Messages[0].Seq != pending {
		t.Fatalf("bad recovery page: %d %+v", response.Code, page)
	}
	response = authenticatedControlRequest(s, cookie, http.MethodPost, "/api/chat/cursor", "reader", `{"seq":3,"revision":3,"server_epoch":"previous-process"}`)
	if response.Code != http.StatusConflict {
		t.Fatal("obsolete process cursor was accepted")
	}
	for _, route := range []string{"/api/chat/messages?after_revision=-1", "/api/chat/messages?after=1&after_revision=1"} {
		if response := authenticatedControlRequest(s, cookie, http.MethodGet, route, "reader", ""); response.Code != http.StatusBadRequest {
			t.Fatalf("invalid recovery query accepted: %s", route)
		}
	}
}
