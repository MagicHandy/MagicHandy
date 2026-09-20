package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
)

const (
	lastMotionTraceKey           = "diagnostics.last_motion_trace.v1"
	lastMotionTraceArchiveSchema = "motion_trace_archive.v1"
	lastMotionTraceMaximumRows   = 128
	lastMotionTraceMaximumBytes  = 1 << 20
)

func (s *Server) traceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/traces", s.handleTraceExport)
	mux.HandleFunc("GET /api/traces/last-motion", s.handleLastMotionTrace)
}

// motionTraceArchive is the durable, bounded envelope for one stopped run.
// Trace rows have already crossed the diagnostics redaction boundary before
// reaching this type. Stop reasons are intentionally not duplicated here.
type motionTraceArchive struct {
	SchemaVersion string                  `json:"schema_version"`
	CapturedAt    string                  `json:"captured_at"`
	CoreEpoch     string                  `json:"core_epoch,omitempty"`
	FirstSequence uint64                  `json:"first_sequence"`
	LastSequence  uint64                  `json:"last_sequence"`
	RowsOmitted   uint64                  `json:"rows_omitted"`
	Trace         diagnostics.TraceExport `json:"trace"`
}

func (s *Server) currentTraceExport() diagnostics.TraceExport {
	export := s.traces.Export()
	intifaceStatus := s.intifaceSnapshot().Status
	export.IntifaceDispatches = intifaceStatus.RecentDispatches
	export.IntifaceDispatchesDropped = intifaceStatus.RecentDispatchesDropped
	export.IntifaceLinearSentCount = intifaceStatus.LinearSentCount
	return export
}

func (s *Server) persistLastMotionTrace(_ string, firstSequence uint64) {
	if firstSequence == 0 || s.store == nil || s.traces == nil {
		return
	}
	archive, document, ok, err := boundedMotionTraceArchive(s.currentTraceExport(), firstSequence, s.controller.epoch)
	if err != nil {
		s.logger.Warn("last motion trace could not be bounded", "error", err)
		return
	}
	if !ok {
		return
	}
	if s.traceArchive != nil {
		s.traceArchive.record(archive.LastSequence, document)
	}
}

func (s *Server) writeLastMotionTrace(ctx context.Context, document []byte) {
	err := s.store.Datastore().WithTx(ctx, func(tx *sql.Tx) error {
		_, execErr := tx.ExecContext(ctx, `
			INSERT INTO app_kv(key, value, updated_at) VALUES(?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
		`, lastMotionTraceKey, string(document), time.Now().UTC().Format(time.RFC3339Nano))
		return execErr
	})
	if err != nil {
		// Motion has already stopped. Persistence is diagnostic-only and must not
		// turn a successful Stop into a user-visible failure.
		s.logger.Warn("last motion trace could not be persisted", "error", err)
	}
}

func boundedMotionTraceArchive(
	export diagnostics.TraceExport,
	firstSequence uint64,
	epoch string,
) (motionTraceArchive, []byte, bool, error) {
	rows := export.Rows[:0]
	for _, row := range export.Rows {
		if row.Sequence >= firstSequence {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return motionTraceArchive{}, nil, false, nil
	}
	export.Rows = rows
	archive := motionTraceArchive{
		SchemaVersion: lastMotionTraceArchiveSchema,
		CoreEpoch:     epoch,
		CapturedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		FirstSequence: rows[0].Sequence,
		LastSequence:  rows[len(rows)-1].Sequence,
		Trace:         export,
	}
	if len(export.Rows) > lastMotionTraceMaximumRows {
		omitted := len(export.Rows) - lastMotionTraceMaximumRows
		for range omitted {
			archive.RowsOmitted++
		}
		archive.Trace.Rows = archive.Trace.Rows[omitted:]
		archive.FirstSequence = archive.Trace.Rows[0].Sequence
	}
	for {
		document, err := json.Marshal(archive)
		if err != nil {
			return motionTraceArchive{}, nil, false, err
		}
		if len(document) <= lastMotionTraceMaximumBytes {
			return archive, document, true, nil
		}
		if len(archive.Trace.Rows) <= 1 {
			return motionTraceArchive{}, nil, false,
				fmt.Errorf("one sanitized trace row exceeds %d bytes", lastMotionTraceMaximumBytes)
		}
		archive.Trace.Rows = archive.Trace.Rows[1:]
		archive.RowsOmitted++
		archive.FirstSequence = archive.Trace.Rows[0].Sequence
	}
}

func (s *Server) loadLastMotionTrace(ctx context.Context) (motionTraceArchive, bool, error) {
	var document string
	if s.traceArchive != nil {
		document = string(s.traceArchive.snapshot())
	}
	if document == "" {
		err := s.store.Datastore().SQL().QueryRowContext(ctx, `
		SELECT value FROM app_kv WHERE key = ?
	`, lastMotionTraceKey).Scan(&document)
		if errors.Is(err, sql.ErrNoRows) {
			return motionTraceArchive{}, false, nil
		}
		if err != nil {
			return motionTraceArchive{}, false, err
		}
	}
	if len(document) > lastMotionTraceMaximumBytes {
		return motionTraceArchive{}, false, errors.New("persisted motion trace exceeds its size limit")
	}
	var archive motionTraceArchive
	if err := json.Unmarshal([]byte(document), &archive); err != nil {
		return motionTraceArchive{}, false, errors.New("persisted motion trace is invalid")
	}
	if archive.SchemaVersion != lastMotionTraceArchiveSchema || len(archive.Trace.Rows) == 0 {
		return motionTraceArchive{}, false, errors.New("persisted motion trace has an unsupported schema")
	}
	return archive, true, nil
}

func (s *Server) handleLastMotionTrace(w http.ResponseWriter, r *http.Request) {
	archive, ok, err := s.loadLastMotionTrace(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("last stopped motion trace is unavailable"))
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("no stopped motion trace has been retained yet"))
		return
	}
	writeJSON(w, http.StatusOK, archive)
}
