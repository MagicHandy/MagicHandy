package memory

import "context"

// Summary is the count-only memory view used by the regular app-state poll.
// EnabledCount counts item switches independently of the global switch.
type Summary struct {
	Enabled      bool
	Count        int
	EnabledCount int
}

// Summary reads aggregate counts without loading private memory text.
func (s *Store) Summary(ctx context.Context) (Summary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	enabled, err := s.memoryEnabledLocked(ctx)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{Enabled: enabled}
	err = s.db.SQL().QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(enabled), 0) FROM memories
	`).Scan(&summary.Count, &summary.EnabledCount)
	return summary, err
}
