package parakeetworker

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/voice/protocol"
)

func TestManagedCrashRemainsVisibleUntilRecovery(t *testing.T) {
	var output bytes.Buffer
	s := &session{writer: &output, loaded: true, runner: &managedServer{stderr: newServerTail(1024)}}
	for _, id := range []string{"first", "next-poll"} {
		output.Reset()
		s.handleHealth(protocol.Request{ID: id})
		var response protocol.Response
		if err := json.Unmarshal(output.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.Type != protocol.ResponseError || response.Error == nil {
			t.Fatalf("crash disappeared from health: %+v", response)
		}
	}
	s.setLoaded(true)
	s.runner = nil // A replacement managed process has reached ready.
	output.Reset()
	s.handleHealth(protocol.Request{ID: "recovered"})
	var response protocol.Response
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Type != protocol.ResponseHealth || response.ModelState != protocol.ModelStateReady {
		t.Fatalf("old failure survived recovery: %+v", response)
	}
}
