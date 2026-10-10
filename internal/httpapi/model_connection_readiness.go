package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func connectionTestKey(connection config.ModelConnection, generation uint64) string {
	encoded, _ := json.Marshal(connection.Normalize())
	return fmt.Sprint(generation) + string(encoded)
}

func (s *Server) recordConnectionReadiness(connection config.ModelConnection, generation uint64, ready bool, message string, elapsedMillis int64) {
	s.cloudPlanning.mu.Lock()
	defer s.cloudPlanning.mu.Unlock()
	if s.cloudPlanning.connectionTests == nil {
		s.cloudPlanning.connectionTests = map[string]cloudReadiness{}
	}
	if len(s.cloudPlanning.connectionTests) >= 32 {
		clear(s.cloudPlanning.connectionTests)
	}
	state := "failed"
	if ready {
		state = "ready"
	}
	checked := connection.Normalize()
	checked.SupportedParameters = append([]string(nil), checked.SupportedParameters...)
	checked.AllowedProviders = append([]string(nil), checked.AllowedProviders...)
	s.cloudPlanning.connectionTests[connectionTestKey(connection, generation)] = cloudReadiness{Connection: &checked, Provider: connection.Provider, Model: connection.Model, Ready: ready, State: state, Message: message, CheckedAt: time.Now().UTC(), ElapsedMillis: elapsedMillis}
}

func (s *Server) connectionReadiness(connection config.ModelConnection, generation uint64) cloudReadiness {
	s.cloudPlanning.mu.Lock()
	defer s.cloudPlanning.mu.Unlock()
	result := s.cloudPlanning.connectionTests[connectionTestKey(connection, generation)]
	if result.CheckedAt.IsZero() || time.Since(result.CheckedAt) > 30*time.Minute {
		return cloudReadiness{Provider: connection.Provider, Model: connection.Model, State: "untested"}
	}
	return result
}

// Setup cannot claim a hosted connection works just because a model ID was
// typed. Only this host's recent completion for the exact connection and
// credential generation admits the selected hosted roles.
func (s *Server) validateSetupConnections(settings config.Settings) error {
	planning, err := settings.LLM.PlanningSettings()
	if err != nil {
		return err
	}
	roles := []config.LLMSettings{settings.LLM.ConversationSettings()}
	if settings.LLM.MotionGenerationMode != config.LLMMotionModeOff && settings.LLM.Capabilities().Motion {
		if settings.LLM.MotionPlanner.Provider == config.MotionPlannerDecisions {
			s.cloudPlanning.mu.Lock()
			ready := s.cloudPlanning.readiness
			s.cloudPlanning.mu.Unlock()
			if !ready.Ready || ready.Provider != config.MotionPlannerDecisions || time.Since(ready.CheckedAt) > 30*time.Minute {
				return errors.New("test the selected Decisions connection before finishing setup")
			}
		} else {
			roles = append(roles, planning)
		}
	}
	for _, role := range roles {
		if !role.IsHosted() {
			continue
		}
		if s.cloudPlanning.auth == nil || role.ActiveConnection == nil {
			return errors.New("connect and test the selected hosted model before finishing setup")
		}
		if !s.connectionReadiness(*role.ActiveConnection, s.cloudPlanning.auth.Generation()).Ready {
			return errors.New("test the selected hosted model before finishing setup; the connection or credentials may have changed")
		}
	}
	return nil
}
