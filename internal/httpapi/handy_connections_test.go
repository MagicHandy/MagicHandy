package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func handyAPI(t *testing.T, s *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := withController(httptest.NewRequest(method, path, strings.NewReader(body)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func addTestHandy(t *testing.T, s *Server, method string) string {
	t.Helper()
	w := handyAPI(t, s, "POST", "/api/settings/handys", fmt.Sprintf(`{"name":"Second Handy","method":%q}`, method))
	if w.Code != 200 {
		t.Fatalf("add: %d %s", w.Code, w.Body.String())
	}
	var body struct {
		ID string `json:"device_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.ID
}
func connectTestHandy(t *testing.T, s *Server, id string) {
	t.Helper()
	w := handyAPI(t, s, "POST", "/api/transport/handys/"+id+"/connect", `{}`)
	if w.Code != 200 {
		t.Fatalf("connect: %d %s", w.Code, w.Body.String())
	}
}
func TestNamedHandysReconnectRetiresOldEngineLease(t *testing.T) {
	s := newTestServer(t)
	s.motion.simulated = true
	id := addTestHandy(t, s, config.DispatchOwnerCloudREST)
	for n := 0; n < 3; n++ {
		connectTestHandy(t, s, id)
		lease, err := s.namedHandyTransport(id)
		if err != nil {
			t.Fatal(err)
		}
		owner, _ := s.handyEntry(id).current()
		old := owner.(*transport.Fake)
		if w := handyAPI(t, s, "POST", "/api/transport/handys/"+id+"/disconnect", `{}`); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		connectTestHandy(t, s, id)
		replacement, _ := s.handyEntry(id).current()
		before := len(replacement.(*transport.Fake).Commands())
		if _, err := lease.Play(t.Context(), transport.PlayCommand{}); err == nil {
			t.Fatal("retired lease admitted playback")
		}
		if _, err := lease.Stop(t.Context(), transport.StopCommand{}); err != nil {
			t.Fatal(err)
		}
		if len(replacement.(*transport.Fake).Commands()) != before || old.Diagnostics().PlaybackState == "playing" {
			t.Fatal("old run reached replacement")
		}
	}
}
func TestNamedHandysShareEngineAndStopExcludedConnections(t *testing.T) {
	s := newTestServer(t)
	s.motion.simulated = true
	a := addTestHandy(t, s, config.DispatchOwnerCloudREST)
	b := addTestHandy(t, s, config.DispatchOwnerCloudREST)
	connectTestHandy(t, s, a)
	connectTestHandy(t, s, b)
	editConnection(t, s, config.DeviceOutputHandy, "exclude")
	editConnection(t, s, a, "include")
	engine, _, err := s.motionEngineForStart()
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := s.store.Snapshot()
	if _, err := engine.Start(t.Context(), motion.MotionTarget{PatternID: motion.PatternStroke, SpeedPercent: 30}, settings.EffectiveMotion()); err != nil {
		t.Fatal(err)
	}
	excluded, _ := s.handyEntry(b).current()
	if len(excluded.(*transport.Fake).Commands()) != 0 {
		t.Fatal("excluded device received playback")
	}
	if _, err := s.emergencyStop(context.Background(), "test"); err != nil {
		t.Fatal(err)
	}
	if engine.Snapshot().Running || len(excluded.(*transport.Fake).Commands()) == 0 {
		t.Fatal("Stop omitted prepared device")
	}
	if w := handyAPI(t, s, "DELETE", "/api/settings/handys/"+a, ``); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	saved, _ := s.store.Snapshot()
	if saved.Device.HasDevice(a) || saved.Device.Plays(a) {
		t.Fatal("removal retained device")
	}
}
func TestNamedHandyStopStillRunsWhenPrimaryCannotBuild(t *testing.T) {
	s := newTestServer(t)
	s.motion.simulated = true
	id := addTestHandy(t, s, config.DispatchOwnerCloudREST)
	connectTestHandy(t, s, id)
	// The primary has no Cloud credentials. Its construction failure must not
	// prevent Stop on the separately prepared connection.
	s.motion.simulated = false
	owner, _ := s.handyEntry(id).current()
	before := len(owner.(*transport.Fake).Commands())
	_, _ = s.stopSelectedTransport(t.Context(), "test_partial")
	if len(owner.(*transport.Fake).Commands()) <= before {
		t.Fatal("primary failure skipped another Handy's Stop")
	}
}
func TestNamedHandyDiscoveryTokensAreBoundAndPrivate(t *testing.T) {
	s := newTestServer(t)
	s.motion.simulated = true
	a := addTestHandy(t, s, config.DispatchOwnerNativeBluetooth)
	b := addTestHandy(t, s, config.DispatchOwnerNativeBluetooth)
	w := handyAPI(t, s, "POST", "/api/transport/handys/"+a+"/discover", `{}`)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if strings.Contains(w.Body.String(), "simulated-handy") {
		t.Fatal("private address in discovery")
	}
	var result struct {
		Candidates []handyDiscoveryChoice `json:"candidates"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || len(result.Candidates) != 1 {
		t.Fatalf("discovery: %s", w.Body.String())
	}
	token := result.Candidates[0].ID
	if _, err := s.handyCandidateAddress(b, token, ""); err == nil {
		t.Fatal("candidate crossed instance")
	}
	if address, err := s.handyCandidateAddress(a, token, ""); err != nil || address == "" {
		t.Fatalf("candidate: %v", err)
	}
	if _, err := s.handyCandidateAddress(a, token, ""); err == nil {
		t.Fatal("candidate reused")
	}
	w = handyAPI(t, s, "POST", "/api/transport/handys/coyote/discover", `{}`)
	if w.Code != http.StatusNotFound {
		t.Fatal("non-Handy discovery accepted")
	}
}
