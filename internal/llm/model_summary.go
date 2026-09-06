package llm

import (
	"context"
	"errors"
	"strings"
)

// ModelSummary is the compact status view; full metadata belongs to inventory UI.
type ModelSummary struct {
	ModelCount        int
	ActiveImportCount int
	SelectedReady     bool
}

// Summary counts inventory without inspecting unselected files. A nonempty
// selectedID is checked on every call, so removal/size/mtime changes use the
// same validation and compatibility-cache invalidation as an explicit Model read.
// An empty ID requests counts only (for external providers or absent runtimes).
func (m *ModelManager) Summary(ctx context.Context, selectedID string) (ModelSummary, error) {
	var summary ModelSummary
	if err := m.db.SQL().QueryRowContext(ctx, "SELECT COUNT(*) FROM llm_models").Scan(&summary.ModelCount); err != nil {
		return summary, modelInventoryError("count managed models", err)
	}
	if selectedID = strings.TrimSpace(selectedID); selectedID != "" {
		selected, err := m.Model(ctx, selectedID)
		if err != nil && !errors.Is(err, ErrModelNotFound) {
			return summary, err
		}
		summary.SelectedReady = err == nil && selected.State == modelStateReady
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, job := range m.jobs {
		if job.snapshot.Status == ImportStatusQueued || job.snapshot.Status == ImportStatusCopying {
			summary.ActiveImportCount++
		}
	}
	return summary, nil
}
