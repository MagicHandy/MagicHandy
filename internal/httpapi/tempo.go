package httpapi

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

var errTempoUnavailable = errors.New("tempo is unavailable")

// tempoRoutes are the tempo channel's controls (ADR 0071).
func (s *Server) tempoRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/motion/tempo/routines", s.handleTempoRoutines)
	mux.HandleFunc("POST /api/motion/tempo", s.handleMotionTempo)
}

// tempoLeads reports a playback whose strokers all make their own strokes:
// the VacuGlide plays and no position stroker does. Positions lead whenever
// a Handy or an Ultra plays (ADR 0071).
func tempoLeads(device config.DeviceSettings) bool {
	strokers, paceOnly := countStrokers(device.PlaybackSet())
	return paceOnly > 0 && paceOnly == strokers
}

func (s *Server) tempoReadiness() error {
	settings, _ := s.store.Snapshot()
	if !settings.Device.Plays(config.DeviceVacuGlide) {
		return fmt.Errorf("%w: include the VacuGlide in playback first", errTempoUnavailable)
	}
	if s.autoblow.vacuglide == nil {
		return fmt.Errorf("%w: connect the VacuGlide first", errTempoUnavailable)
	}
	if status := s.autoblow.vacuglide.Status(); !status.Connected || status.PlaybackState == "stale" {
		return fmt.Errorf("%w: connect the VacuGlide first", errTempoUnavailable)
	}
	if s.playbackDeviceRetired(config.DeviceVacuGlide) {
		return fmt.Errorf("%w: the VacuGlide left this playback session; start a new session to include it", errTempoUnavailable)
	}
	return nil
}

// tempoTarget is a tempo-only run: the shared engine keeps its ordinary clock,
// lifecycle and Stop, and no position device moves, like a vibration-only run.
func tempoTarget(settings config.Settings, spec motion.TempoSpec, source, label string) (motion.MotionTarget, error) {
	return motion.NormalizeTarget(motion.MotionTarget{Source: source, Label: label, TempoOnly: true, Tempo: &spec}, settings.EffectiveMotion()), spec.Validate()
}

// seededTempo gives a routine a fresh seed, so each start varies a little.
func seededTempo(spec motion.TempoSpec) motion.TempoSpec {
	if spec.Mode == motion.TempoModeRoutine && spec.Seed == 0 {
		// Below 2^53, so a client reads the seed exactly as JSON.
		spec.Seed = rand.Uint64N(1<<53-1) + 1 // #nosec G404 -- musical variation, not security randomness.
	}
	return spec
}

func (s *Server) handleTempoRoutines(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"routines": motion.TempoRoutines()})
}

// handleMotionTempo is the person's own tempo choice: a level or a routine.
// Like choosing a library pattern, it ends Autopilot. A running run changes
// tempo in place; from idle it starts a tempo-only run.
func (s *Server) handleMotionTempo(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	stopSequence, err := s.requestStopSequence(r)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	var body struct {
		Tempo motion.TempoSpec `json:"tempo"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	spec := seededTempo(body.Tempo)
	if err := spec.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.tempoReadiness(); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	if s.modes != nil {
		finishModeStop := s.modes.BeginUserStop()
		defer finishModeStop()
	}
	state, err := s.applyTempo(r.Context(), spec, stopSequence, motion.TargetSourceManualUI, "VacuGlide tempo")
	s.writeMotionResult(w, r, state, err)
}

// applyTempo changes the tempo of the running run, or starts a tempo-only run
// from idle. stopSequence is the Stop epoch the request was admitted under.
func (s *Server) applyTempo(ctx context.Context, spec motion.TempoSpec, stopSequence uint64, source, label string) (motion.ActiveMotionState, error) {
	if engine := s.currentMotionEngine(); engine != nil {
		state := engine.Snapshot()
		if state.Paused {
			return state, errors.New("playback is paused; resume it before changing the tempo")
		}
		if state.Running || state.Starting {
			admission := engine.AdmissionGeneration()
			if s.stopSequence.Load() != stopSequence {
				return state, errors.New("the tempo change was canceled by Stop")
			}
			if candidate, ok := ctx.Value(chatRepairContextKey{}).(*chatRepairCandidate); ok && candidate.active.Load() {
				if candidate.engine != engine {
					return state, errChatRepairStale
				}
				return engine.ApplyTempoIfCurrent(ctx, spec, admission, candidate.state.Generation)
			}
			return engine.ApplyTempo(ctx, spec, admission)
		}
	}
	if spec.Mode == motion.TempoModeFollow {
		return motion.ActiveMotionState{}, errors.New("tempo follows movement only while positions play")
	}
	if state, err := s.stopActiveMotionForReplacement(ctx, "tempo_start_replace"); err != nil {
		return state, err
	}
	engine, admission, err := s.motionEngineForStart()
	if err != nil {
		return motion.ActiveMotionState{}, err
	}
	settings, _ := s.store.Snapshot()
	target, err := tempoTarget(settings, spec, source, label)
	if err != nil {
		return motion.ActiveMotionState{}, err
	}
	if s.stopSequence.Load() != stopSequence {
		return engine.Snapshot(), errors.New("the tempo start was canceled by Stop")
	}
	return engine.StartAtGeneration(ctx, target, settings.EffectiveMotion(), admission)
}

// applyMotionAir takes the valve steps chat or Autopilot chose. The owner
// keeps them inside the person's permission and the session's bound.
func (s *Server) applyMotionAir(ctx context.Context, steps int, stopSequence uint64) error {
	if steps == 0 {
		return nil
	}
	if err := s.tempoReadiness(); err != nil {
		return err
	}
	engine := s.currentMotionEngine()
	if engine == nil {
		return errors.New("start playback before adjusting the air")
	}
	admission := engine.AdmissionGeneration()
	if s.stopSequence.Load() != stopSequence {
		return errors.New("the air adjustment was canceled by Stop")
	}
	_, err := engine.ApplyAir(ctx, steps, admission)
	return err
}

// dispatchChatTempo applies a chat or Autopilot tempo, then its air steps,
// under the Stop epoch the turn was admitted with.
func (s *Server) dispatchChatTempo(ctx context.Context, command *chat.MotionCommand, stopSequence *uint64) (chatMotionDispatch, error) {
	result := chatMotionDispatch{Action: "tempo"}
	sequence := s.stopSequence.Load()
	if stopSequence != nil {
		sequence = *stopSequence
	}
	if s.stopSequence.Load() != sequence {
		return result, errors.New("the tempo change was canceled by Stop")
	}
	if err := s.tempoReadiness(); err != nil {
		return result, err
	}
	var tempoErr error
	if command.Tempo != nil {
		spec := seededTempo(*command.Tempo)
		handoff := s.prepareChatTarget()
		var state motion.ActiveMotionState
		state, tempoErr = s.applyTempo(ctx, spec, sequence, "chat", "Chat tempo")
		if tempoErr != nil {
			s.cancelChatTarget(handoff)
		} else {
			// A native update can also accompany an existing position run.
			// Adopt what the engine actually kept, not an invented tempo-only run.
			s.notifyChatTarget(handoff, state.Target)
		}
		result.Engine = state
	}
	var airErr error
	if command.AirSteps != nil && tempoErr == nil {
		airErr = s.applyMotionAir(ctx, *command.AirSteps, sequence)
	}
	result.Applied = tempoErr == nil && airErr == nil
	return result, errors.Join(tempoErr, airErr)
}

// mapAutopilotTempo turns an Autopilot tempo choice into a held decision: the
// positions stay, and the tempo and air change. A tempo that lost its device
// while the model thought is dropped with its announcement.
func (s *Server) mapAutopilotTempo(command *chat.MotionCommand, vibration *motion.VibrationSpec, say string,
	next modes.TimingPreference, variability modes.VariabilityPreference) modes.Decision {
	decision := modes.Decision{Hold: true, Vibration: vibration, Say: say, Next: next, Variability: variability}
	if err := s.tempoReadiness(); err != nil {
		decision.Say = ""
		return decision
	}
	if command.Tempo != nil {
		spec := seededTempo(*command.Tempo)
		decision.Tempo = &spec
	}
	if command.AirSteps != nil {
		decision.AirSteps = *command.AirSteps
	}
	return decision
}
