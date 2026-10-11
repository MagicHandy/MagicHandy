package config

import "errors"

// MotionReviewerSettings opts local chat and Autopilot into a second opinion.
// The primary model always finishes first; the reviewer cannot start motion.
type MotionReviewerSettings struct {
	Enabled      bool   `json:"enabled"`
	ConnectionID string `json:"connection_id,omitempty"`
}

// ReviewerSettings resolves an existing named connection, keeping credentials
// in the same protected storage as the other model roles.
func (s LLMSettings) ReviewerSettings() (LLMSettings, error) {
	connection, ok := s.ConnectionByID(s.MotionReviewer.ConnectionID)
	if !ok || connection.Model == "" {
		return LLMSettings{}, errors.New("select a saved connection and model for the second opinion")
	}
	return s.WithConnection(connection), nil
}

func validateMotionReviewer(s LLMSettings) error {
	if !s.MotionReviewer.Enabled {
		return nil
	}
	_, err := s.ReviewerSettings()
	return err
}
