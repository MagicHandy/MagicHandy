package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestLinkedCoyoteLossCannotReportConfirmedGlobalStop(t *testing.T) {
	s, connector := newCoyoteTestServer(t, config.DeviceOutputHandyCoyote, true)
	connectCoyote(t, s)
	engine, _, err := s.motionEngineForStart()
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := s.store.Snapshot()
	if _, err := engine.Start(t.Context(), motion.MotionTarget{PatternID: motion.PatternStroke, SpeedPercent: 30}, settings.Motion); err != nil {
		t.Fatal(err)
	}
	connector.LastLink().Lose(errors.New("test radio loss"))
	waitCoyoteStatus(t, s, func(status transport.CoyoteStatus) bool { return !status.Connected })
	// No controller lease: this is the public safety action used by observers.
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader(`{}`)))
	if w.Code != http.StatusBadGateway || engine.Snapshot().Running {
		t.Fatalf("lost participant reported success or blocked engine Stop: %d %s", w.Code, w.Body.String())
	}
	result, err := s.ConsoleStop(t.Context())
	if err == nil || result.Confirmed || !result.Available {
		t.Fatalf("console falsely confirmed lost participant Stop: %+v %v", result, err)
	}
	handy := s.motion.transport.(*transport.Fake)
	commands := handy.Commands()
	if len(commands) == 0 || commands[len(commands)-1].Kind != transport.CommandKindStop {
		t.Fatal("lost Coyote prevented healthy Handy Stop")
	}
}
