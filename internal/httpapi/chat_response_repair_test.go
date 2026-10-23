package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func configureResponseRepair(t *testing.T, s *Server, enabled bool) {
	t.Helper()
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.HostedResponseRepairEnabled = &enabled
		return settings
	})
}

func repairOffer(t *testing.T, s *Server, stream string) (string, int64) {
	t.Helper()
	var id int64
	for _, block := range strings.Split(stream, "\n\n") {
		if !strings.HasPrefix(block, "event: error\n") {
			continue
		}
		var payload struct {
			Repair struct {
				ID        int64 `json:"failure_id"`
				Available bool  `json:"available"`
			} `json:"repair"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(block, "event: error\ndata: ")), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Repair.Available {
			id = payload.Repair.ID
		}
	}
	if id == 0 {
		t.Fatalf("no eligible repair in %s", stream)
	}
	session, err := s.chatLog.ActiveSessionIDContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return session, id
}

func postRepair(s *Server, session string, id int64) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"session_id":%q,"failure_id":%d}`, session, id)
	r := withController(httptest.NewRequest(http.MethodPost, "/api/chat/repair", strings.NewReader(body)))
	r.Header.Set(stopSequenceHeader, strconv.FormatUint(s.stopSequence.Load(), 10))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func TestHostedResponseRepairAutomaticKeepsRejectedEvidence(t *testing.T) {
	var calls atomic.Int32
	s, fake, _ := hostedFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			writeHostedFixtureReply(w, `{"reply":"Rejected partial","unknown":true}`)
		} else {
			writeHostedFixtureReply(w, `{"reply":"Corrected reply."}`)
		}
	})
	stream := postChatStream(t, s, `{"message":"A text-only greeting."}`)
	if calls.Load() != 2 || !strings.Contains(stream, `"state":"repairing_response"`) || !strings.Contains(stream, `"reply":"Corrected reply."`) || !strings.Contains(stream, `"repaired":true`) || strings.Contains(stream, "event: motion") || len(fake.Commands()) != 0 {
		t.Fatalf("automatic repair = %s, calls=%d", stream, calls.Load())
	}
	session, _ := s.chatLog.ActiveSessionIDContext(t.Context())
	rows, err := s.chatLog.RecentSession(session, 10)
	if err != nil || len(rows) != 2 || rows[0].Role != chat.MessageRoleUser || rows[1].Diagnostics.ProviderCalls != 2 || !rows[1].Diagnostics.Repaired {
		t.Fatalf("history=%+v %v", rows, err)
	}
	failures, err := s.chatLog.Failures(t.Context(), session)
	if err != nil || len(failures) != 1 || !failures[0].Diagnostics.Repaired || failures[0].Kind != "response_repair" {
		t.Fatalf("failures=%+v %v", failures, err)
	}
	if failures[0].UserSeq != rows[0].Seq {
		t.Fatalf("failure user_seq=%d want=%d", failures[0].UserSeq, rows[0].Seq)
	}
	assertRetainedRepairEvidence(t, s, session, failures[0].ID)
}

func assertRetainedRepairEvidence(t *testing.T, s *Server, session string, id int64) {
	t.Helper()
	evidence, err := s.chatLog.Failure(t.Context(), session, id, true)
	if err != nil || !strings.Contains(evidence.Details.Raw, "unknown") || !strings.Contains(evidence.Details.RepairRaw, "Corrected reply") || evidence.Details.InitialError == "" || evidence.Details.Schema == "" {
		t.Fatalf("evidence=%+v %v", evidence, err)
	}
}

func TestHostedResponseRepairRefusalCannotAddThirdLocalAttempt(t *testing.T) {
	var calls, localCalls atomic.Int32
	local := localRetryEndpoint(t, "valid", &localCalls, make(chan []llm.Message, 1))
	defer local.Close()
	s, fake, _ := hostedFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			writeHostedFixtureReply(w, `{"unknown":true}`)
		} else {
			writeRefusalFixture(w)
		}
	})
	configureLocalRetry(t, s, local.URL, true)
	stream := postChatStream(t, s, `{"message":"Text only."}`)
	if calls.Load() != 2 || localCalls.Load() != 0 || strings.Contains(stream, "event: message") || len(fake.Commands()) != 0 {
		t.Fatalf("repair launched fallback: calls=%d local=%d %s", calls.Load(), localCalls.Load(), stream)
	}
}

func TestHostedResponseRepairManualDoesNotDuplicateUserAndCannotReplay(t *testing.T) {
	for _, success := range []bool{true, false} {
		t.Run(strconv.FormatBool(success), func(t *testing.T) {
			var calls atomic.Int32
			s, fake, _ := hostedFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				if calls.Add(1) == 1 || !success {
					writeHostedFixtureReply(w, `{"unknown":true}`)
				} else {
					writeHostedFixtureReply(w, `{"reply":"Corrected reply."}`)
				}
			})
			configureResponseRepair(t, s, false)
			session, id := repairOffer(t, s, postChatStream(t, s, `{"message":"Text only."}`))
			if calls.Load() != 1 {
				t.Fatal("disabled setting retried automatically")
			}
			w := postRepair(s, session, id)
			if w.Code != 200 || calls.Load() != 2 || strings.Contains(w.Body.String(), `"available":true`) || strings.Contains(w.Body.String(), "event: message") != success {
				t.Fatalf("manual repair=%d %s", w.Code, w.Body.String())
			}
			rows, err := s.chatLog.RecentSession(session, 10)
			want := 1
			if success {
				want = 2
			}
			if err != nil || len(rows) != want || rows[0].Role != chat.MessageRoleUser {
				t.Fatalf("duplicate user or partial reply: %+v %v", rows, err)
			}
			w = postRepair(s, session, id)
			if w.Code != http.StatusConflict || calls.Load() != 2 || len(fake.Commands()) != 0 {
				t.Fatalf("replay=%d %s calls=%d", w.Code, w.Body.String(), calls.Load())
			}
		})
	}
}

func TestHostedResponseRepairRejectsChangedVideoMotionOwner(t *testing.T) {
	for _, owner := range []string{"script", "off"} {
		t.Run(owner, func(t *testing.T) {
			var calls atomic.Int32
			s, _, _ := hostedFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				writeHostedFixtureReply(w, `{"unknown":true}`)
			})
			configureResponseRepair(t, s, false)
			session, id := repairOffer(t, s, postChatStream(t, s, `{"message":"Text only."}`))
			body := fmt.Sprintf(`{"session_id":%q,"failure_id":%d,"motion_owner":%q}`, session, id, owner)
			r := withController(httptest.NewRequest(http.MethodPost, "/api/chat/repair", strings.NewReader(body)))
			r.Header.Set(stopSequenceHeader, strconv.FormatUint(s.stopSequence.Load(), 10))
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != http.StatusConflict || calls.Load() != 1 {
				t.Fatalf("owner change admitted: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestHostedResponseRepairAdmissionRaceDoesNotConsumeOffer(t *testing.T) {
	var calls atomic.Int32
	s, _, _ := hostedFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			writeHostedFixtureReply(w, `{"unknown":true}`)
		} else {
			writeHostedFixtureReply(w, `{"reply":"Corrected."}`)
		}
	})
	configureResponseRepair(t, s, false)
	session, id := repairOffer(t, s, postChatStream(t, s, `{"message":"Text only."}`))
	// Recreate the offered response's original turn still owning the workspace
	// while its SSE completion is flushing. No later user message is appended.
	_, finish, err := s.chatWorkspace.BeginTurn(t.Context(), session)
	if err != nil {
		t.Fatal(err)
	}
	s.chatRepair.mu.Lock()
	s.chatRepair.pending.workspaceRevision = s.chatWorkspace.Revision()
	s.chatRepair.mu.Unlock()
	w := postRepair(s, session, id)
	if w.Code != http.StatusConflict || calls.Load() != 1 {
		finish()
		t.Fatalf("busy repair admitted: %d %s", w.Code, w.Body.String())
	}
	finish()
	w = postRepair(s, session, id)
	if w.Code != http.StatusOK || calls.Load() != 2 || !strings.Contains(w.Body.String(), `"reply":"Corrected."`) {
		t.Fatalf("busy admission consumed action: %d %s", w.Code, w.Body.String())
	}
}

func TestHostedResponseRepairExpiresOnContextChanges(t *testing.T) {
	for name, change := range map[string]func(*testing.T, *Server, string){
		"Stop":     func(_ *testing.T, s *Server, _ string) { s.stopSequence.Add(1); s.chatWorkspace.CancelTurns() },
		"new turn": func(t *testing.T, s *Server, _ string) { postChatStream(t, s, `{"message":"A different request."}`) },
		"history": func(t *testing.T, s *Server, session string) {
			if _, err := s.chatLog.AppendTo(session, chat.MessageRoleUser, "Later context", "test-controller", nil); err != nil {
				t.Fatal(err)
			}
		},
		"provider": func(t *testing.T, s *Server, _ string) {
			saveSettings(t, s.store, func(settings config.Settings) config.Settings {
				settings.LLM.Connections[0].Model = "changed-model"
				return settings
			})
		},
		"mode": func(t *testing.T, s *Server, _ string) {
			saveSettings(t, s.store, func(settings config.Settings) config.Settings {
				settings.LLM.MotionGenerationMode = config.LLMMotionModeLayered
				return settings
			})
		},
		"credentials": func(_ *testing.T, s *Server, _ string) { s.invalidateCloudPlanning() },
		"controller": func(_ *testing.T, s *Server, _ string) {
			s.controller.mu.Lock()
			s.controller.generation++
			s.controller.mu.Unlock()
		},
		"expired": func(_ *testing.T, s *Server, _ string) {
			s.chatRepair.mu.Lock()
			s.chatRepair.pending.expires = time.Now().Add(-time.Second)
			s.chatRepair.mu.Unlock()
		},
		"session roundtrip": func(t *testing.T, s *Server, session string) {
			if _, err := s.chatWorkspace.Save(session); err != nil {
				t.Fatal(err)
			}
			if _, err := s.chatWorkspace.Create(false); err != nil {
				t.Fatal(err)
			}
			if _, err := s.chatWorkspace.Activate(session, true); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			s, fake, _ := hostedFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				writeHostedFixtureReply(w, `{"unknown":true}`)
			})
			configureResponseRepair(t, s, false)
			session, id := repairOffer(t, s, postChatStream(t, s, `{"message":"Text only."}`))
			change(t, s, session)
			before := calls.Load()
			w := postRepair(s, session, id)
			if w.Code != http.StatusConflict || calls.Load() != before || len(fake.Commands()) != 0 {
				t.Fatalf("stale repair admitted: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestHostedResponseRepairCanceledDuringGenerationCannotPublish(t *testing.T) {
	for _, manual := range []bool{false, true} {
		t.Run(strconv.FormatBool(manual), func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			s, fake, _ := hostedFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					writeHostedFixtureReply(w, `{"unknown":true}`)
					return
				}
				close(started)
				select {
				case <-release:
					writeHostedFixtureReply(w, `{"reply":"Late correction."}`)
				case <-r.Context().Done():
				}
			})
			configureResponseRepair(t, s, !manual)
			var session string
			var id int64
			if manual {
				session, id = repairOffer(t, s, postChatStream(t, s, `{"message":"Text only."}`))
			}
			done := make(chan string, 1)
			go func() {
				if manual {
					done <- postRepair(s, session, id).Body.String()
				} else {
					done <- postChatStream(t, s, `{"message":"Text only."}`)
				}
			}()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("repair never started")
			}
			s.stopSequence.Add(1)
			s.chatWorkspace.CancelTurns()
			close(release)
			select {
			case stream := <-done:
				if strings.Contains(stream, "event: message") || strings.Contains(stream, "event: motion") {
					t.Fatalf("late effect: %s", stream)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("repair did not cancel")
			}
			if len(fake.Commands()) != 0 {
				t.Fatal("canceled repair dispatched")
			}
			assertRepairCancellationEvidence(t, s, "stopped")
		})
	}
}

func TestHostedResponseRepairLayeredStartsExactlyOnceThroughSharedEngine(t *testing.T) {
	for _, manual := range []bool{false, true} {
		t.Run(strconv.FormatBool(manual), func(t *testing.T) {
			var calls atomic.Int32
			s, fake, _ := hostedFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				if calls.Add(1) == 1 {
					writeHostedFixtureReply(w, `{"action":"start","edits":{"controls":{"speed_percent":{"op":"set","value":999}}},"reply":"Bad."}`)
				} else {
					writeHostedFixtureReply(w, `{"action":"start","edits":{"controls":{"speed_percent":{"op":"set","value":30}}},"reply":"Starting."}`)
				}
			})
			configureResponseRepair(t, s, !manual)
			saveSettings(t, s.store, func(settings config.Settings) config.Settings {
				settings.LLM.MotionGenerationMode = config.LLMMotionModeLayered
				return settings
			})
			stream := postChatStream(t, s, `{"message":"Start moving at speed 30."}`)
			if manual {
				session, id := repairOffer(t, s, stream)
				if len(fake.Commands()) != 0 {
					t.Fatal("rejected response had motion effects")
				}
				stream = postRepair(s, session, id).Body.String()
			}
			engine := s.currentMotionEngine()
			if engine == nil || !engine.Snapshot().Running || engine.Snapshot().Target.Flow == nil || !strings.Contains(stream, `"applied":true`) {
				t.Fatalf("repair did not use shared engine: %s", stream)
			}
			plays := 0
			for _, command := range fake.Commands() {
				if command.Kind == transport.CommandKindPointsPlay {
					plays++
				}
			}
			if plays != 1 || calls.Load() != 2 {
				t.Fatalf("plays=%d calls=%d", plays, calls.Load())
			}
			callMotion(t, s, http.MethodPost, "/api/motion/stop", `{}`)
			if engine.Snapshot().Running {
				t.Fatal("Stop failed after repair")
			}
		})
	}
}

func TestHostedResponseRepairRejectsNewerMotionTarget(t *testing.T) {
	for _, during := range []bool{false, true} {
		t.Run(strconv.FormatBool(during), func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			s, _, _ := hostedFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					writeHostedFixtureReply(w, `{"action":"update","edits":{"unknown":true},"reply":"Bad."}`)
					return
				}
				close(entered)
				select {
				case <-release:
					writeHostedFixtureReply(w, `{"action":"update","edits":{"controls":{"speed_percent":{"op":"set","value":30}}},"reply":"Old correction."}`)
				case <-r.Context().Done():
				}
			})
			configureResponseRepair(t, s, false)
			saveSettings(t, s.store, func(settings config.Settings) config.Settings {
				settings.LLM.MotionGenerationMode = config.LLMMotionModeLayered
				return settings
			})
			callMotion(t, s, http.MethodPost, "/api/motion/start", `{"speed_percent":25}`)
			session, id := repairOffer(t, s, postChatStream(t, s, `{"message":"Change the motion."}`))
			var done chan *httptest.ResponseRecorder
			if during {
				done = make(chan *httptest.ResponseRecorder, 1)
				go func() { done <- postRepair(s, session, id) }()
				select {
				case <-entered:
				case <-time.After(3 * time.Second):
					t.Fatal("repair did not start")
				}
			}
			engine := s.currentMotionEngine()
			target := engine.Snapshot().Target
			target.SpeedPercent = 35
			if _, err := engine.ApplyTarget(t.Context(), target, "newer_autopilot_target"); err != nil {
				t.Fatal(err)
			}
			newPlan := engine.Snapshot().PlanID
			var w *httptest.ResponseRecorder
			if during {
				close(release)
				select {
				case w = <-done:
				case <-time.After(3 * time.Second):
					t.Fatal("late repair did not finish")
				}
			} else {
				w = postRepair(s, session, id)
			}
			if strings.Contains(w.Body.String(), "event: message") || strings.Contains(w.Body.String(), "event: motion") || engine.Snapshot().PlanID != newPlan {
				t.Fatalf("newer target replaced: %s", w.Body.String())
			}
			if !during && calls.Load() != 1 {
				t.Fatal("stale manual action still called provider")
			}
			if during {
				assertRepairCancellationEvidence(t, s, "stale")
			}
			callMotion(t, s, http.MethodPost, "/api/motion/stop", `{}`)
		})
	}
}

func assertRepairCancellationEvidence(t *testing.T, s *Server, outcome string) {
	t.Helper()
	session, err := s.chatLog.ActiveSessionIDContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	failures, err := s.chatLog.Failures(t.Context(), session)
	if err != nil || len(failures) == 0 {
		t.Fatalf("missing cancellation evidence: %v", err)
	}
	if failures[0].RepairOutcome != outcome {
		t.Fatalf("repair outcome=%q want=%q", failures[0].RepairOutcome, outcome)
	}
	evidence, err := s.chatLog.Failure(t.Context(), session, failures[0].ID, true)
	if err != nil || evidence.Details.Raw == "" || evidence.Details.InitialError == "" || evidence.UserSeq == 0 {
		t.Fatalf("cancellation lost original rejection: %+v %v", evidence, err)
	}
}

type repairPhaseWriter struct {
	*httptest.ResponseRecorder
	phase   string
	changed bool
	change  func()
}

func (w *repairPhaseWriter) Write(data []byte) (int, error) {
	if !w.changed && strings.Contains(string(data), "event: "+w.phase+"\n") {
		w.changed = true
		w.change()
	}
	return w.ResponseRecorder.Write(data)
}

func TestHostedResponseRepairLateStalenessIsNotReportedAsEmergencyStop(t *testing.T) {
	for _, phase := range []string{"malformed", "message"} {
		t.Run(phase, func(t *testing.T) {
			var calls atomic.Int32
			s, fake, _ := hostedFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				if calls.Add(1) == 1 {
					writeHostedFixtureReply(w, `{"unknown":true}`)
				} else {
					writeHostedFixtureReply(w, `{"reply":"Corrected."}`)
				}
			})
			configureResponseRepair(t, s, false)
			session, id := repairOffer(t, s, postChatStream(t, s, `{"message":"Text only."}`))
			w := &repairPhaseWriter{ResponseRecorder: httptest.NewRecorder(), phase: phase, change: func() {
				saveSettings(t, s.store, func(settings config.Settings) config.Settings { settings.LLM.ReplyLength = "short"; return settings })
			}}
			r := withController(httptest.NewRequest(http.MethodPost, "/api/chat/repair", strings.NewReader(fmt.Sprintf(`{"session_id":%q,"failure_id":%d}`, session, id))))
			r.Header.Set(stopSequenceHeader, strconv.FormatUint(s.stopSequence.Load(), 10))
			s.Handler().ServeHTTP(w, r)
			if !w.changed || !strings.Contains(w.Body.String(), "repair expired") || strings.Contains(w.Body.String(), "Emergency Stop") {
				t.Fatalf("wrong cancellation: %s", w.Body.String())
			}
			rows, err := s.chatLog.RecentSession(session, 10)
			if err != nil || len(rows) != 1 || len(fake.Commands()) != 0 {
				t.Fatalf("late reply applied: %+v %v", rows, err)
			}
			assertRepairCancellationEvidence(t, s, "stale")
		})
	}
}
