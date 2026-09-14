package httpapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
)

func setSessionIdleFixture(t *testing.T, s *Server, key string, lastSeen time.Time) string {
	t.Helper()
	value := lastSeen.UTC().Format(time.RFC3339Nano)
	if _, err := s.store.Datastore().SQL().ExecContext(t.Context(), `UPDATE user_sessions SET last_seen_at = ? WHERE token_hash = ?`, value, key); err != nil {
		t.Fatal(err)
	}
	return value
}

func readSessionIdleFixture(t *testing.T, s *Server, key string) string {
	t.Helper()
	var value string
	if err := s.store.Datastore().SQL().QueryRowContext(t.Context(), `SELECT last_seen_at FROM user_sessions WHERE token_hash = ?`, key).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestAutomaticBrowserAcknowledgementsDoNotRenewLoginIdleTime(t *testing.T) {
	s, store, _, cookie := newControllerSessionFixture(t)
	claimAuthenticatedController(t, s, cookie)
	session, err := store.InspectSession(t.Context(), cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []struct{ method, route, body string }{
		{http.MethodGet, "/api/state", ""},
		{http.MethodPost, "/api/controller/heartbeat", `{}`},
		{http.MethodPost, "/api/chat/cursor", `{"seq":0}`},
		{http.MethodPost, "/api/transport/bluetooth/status", `{"client_id":"unbound-browser","connected":false}`},
		{http.MethodPost, "/api/transport/bluetooth/ack", `{"client_id":"unbound-browser","id":"missing","ok":true}`},
		{http.MethodPost, "/api/transport/bluetooth/disconnect", `{"client_id":"unbound-browser"}`},
		{http.MethodPost, "/api/voice/requests/missing/played", `{}`},
		{http.MethodPost, "/api/media/duration", `{"id":"missing","duration_ms":1000}`},
		{http.MethodPost, "/api/media/sync", `{"video_id":"missing","session_id":"player","event_sequence":1,"state":"playing","event":"heartbeat","media_time_ms":0,"playback_rate":1}`},
	} {
		t.Run(request.route, func(t *testing.T) {
			before := setSessionIdleFixture(t, s, session.Key, time.Now().Add(-2*time.Minute))
			response := authenticatedControlRequest(s, cookie, request.method, request.route, gatewayTestTab, request.body)
			if response.Code == http.StatusUnauthorized {
				t.Fatal("fixture was rejected before its activity policy was exercised")
			}
			if after := readSessionIdleFixture(t, s, session.Key); after != before {
				t.Fatalf("automatic request renewed login idle time: %s", request.route)
			}
		})
	}
}

func TestExplicitPlaybackActivityRenewsOnlyAValidSession(t *testing.T) {
	s, store, _, cookie := newControllerSessionFixture(t)
	claimAuthenticatedController(t, s, cookie)
	session, err := store.InspectSession(t.Context(), cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	before := setSessionIdleFixture(t, s, session.Key, time.Now().Add(-2*time.Minute))
	// The event is a valid playback intention; this fixture has no video file.
	// Its eventual media error must not turn this explicit action into polling.
	_ = authenticatedControlRequest(s, cookie, http.MethodPost, "/api/media/sync", gatewayTestTab,
		`{"video_id":"missing","session_id":"player","event_sequence":1,"state":"playing","event":"play","media_time_ms":0,"playback_rate":1}`)
	if readSessionIdleFixture(t, s, session.Key) == before {
		t.Fatal("explicit playback action did not renew login activity")
	}
	before = setSessionIdleFixture(t, s, session.Key, time.Now().Add(-accounts.DefaultSessionIdleLimit-time.Second))
	response := authenticatedControlRequest(s, cookie, http.MethodPost, "/api/controller/heartbeat", gatewayTestTab, `{}`)
	if response.Code != http.StatusUnauthorized || readSessionIdleFixture(t, s, session.Key) != before {
		t.Fatal("heartbeat revived an expired login")
	}
}
