package httpapi

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/funscript"
	"github.com/mapledaemon/MagicHandy/internal/library"
)

const maxImportUploadBytes = 32 << 20

var allowedImportSuffixes = map[string]struct{}{
	".csv":       {},
	".json":      {},
	".funscript": {},
}

func (s *Server) registerLibraryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/import", s.handleImportList)
	mux.HandleFunc("POST /api/import", s.handleImportUpload)

	mux.HandleFunc("GET /api/patterns/meta", s.handlePatternsMeta)
	mux.HandleFunc("GET /api/patterns/count", s.handlePatternsCount)
	mux.HandleFunc("GET /api/patterns/ids", s.handlePatternIDs)
	mux.HandleFunc("GET /api/patterns", s.handlePatternsList)
	mux.HandleFunc("GET /api/patterns/{id}", s.handlePatternGet)
	mux.HandleFunc("PUT /api/patterns/{id}", s.handlePatternPut)
	mux.HandleFunc("PATCH /api/patterns/{id}", s.handlePatternPut)
	mux.HandleFunc("DELETE /api/patterns/{id}", s.handlePatternDelete)
	mux.HandleFunc("POST /api/patterns/{id}/feedback", s.handlePatternFeedback)
}

func (s *Server) handleImportList(w http.ResponseWriter, r *http.Request) {
	if s.library == nil {
		writeError(w, http.StatusServiceUnavailable, errLibraryUnavailable)
		return
	}
	limit := queryInt(r, "limit", 50)
	if limit < 1 {
		limit = 1
	}
	if limit > 200 {
		limit = 200
	}

	files, err := s.library.Store().ListFunscriptFiles(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	out := make([]map[string]any, 0, len(files))
	for _, file := range files {
		blockCount, err := s.library.Store().CountMotionBlocksByFileID(r.Context(), file.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		durationSec := 0.0
		if file.DurationMS > 0 {
			durationSec = float64(file.DurationMS) / 1000.0
		}
		out = append(out, map[string]any{
			"file_id":          file.ID,
			"filename":         file.Filename,
			"display_filename": library.ImportDisplayFilename(file.Filename),
			"duration_sec":     durationSec,
			"action_count":     file.ActionCount,
			"block_count":      blockCount,
			"source_format":    nullIfEmpty(file.SourceFormat),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": out})
}

func (s *Server) handleImportUpload(w http.ResponseWriter, r *http.Request) {
	if s.library == nil {
		writeError(w, http.StatusServiceUnavailable, errLibraryUnavailable)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	defer file.Close()

	name := header.Filename
	if strings.TrimSpace(name) == "" {
		name = "upload.funscript"
	}
	suffix := strings.ToLower(filepath.Ext(name))
	if suffix == "" {
		suffix = ".funscript"
	}
	if _, ok := allowedImportSuffixes[suffix]; !ok {
		writeError(w, http.StatusBadRequest, errImportExtension)
		return
	}

	limited := io.LimitReader(file, maxImportUploadBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(data) > maxImportUploadBytes {
		writeError(w, http.StatusBadRequest, errImportTooLarge)
		return
	}

	tmp, err := os.CreateTemp("", "magichandy-import-*"+suffix)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := tmp.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	loaded, err := funscript.LoadActionsFromText(string(data), name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	loaded.SourcePath = tmpPath

	result, err := funscript.Ingest(loaded, name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	persisted, err := s.library.PersistIngestResult(r.Context(), result)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	payload := importResultPayload(result, persisted)
	payload["ok"] = true
	writeJSON(w, http.StatusOK, payload)
}

func importResultPayload(result funscript.IngestResult, persisted library.PersistResult) map[string]any {
	inserted := make(map[string]struct{}, len(persisted.InsertedBlockIDs))
	for _, id := range persisted.InsertedBlockIDs {
		inserted[id] = struct{}{}
	}

	sourceFilename := result.Source.Filename
	blockSummaries := make([]map[string]any, 0, len(result.Blocks))
	var fullSummary map[string]any
	for _, block := range result.Blocks {
		summary := blockImportSummary(block, sourceFilename, inserted)
		if block.IsFullScript || funscript.IsFullScriptBlock(block.ID, block.Zone, block.Tags) {
			fullSummary = summary
			continue
		}
		blockSummaries = append(blockSummaries, summary)
	}

	payload := map[string]any{
		"source":             result.Source,
		"metadata":           result.Metadata,
		"extra_fields":       result.ExtraFields,
		"normalized_actions": result.NormalizedActions,
		"imported_actions":   result.ImportedActions,
		"summary":            result.Summary,
		"blocks":             result.Blocks,
		"persisted": map[string]any{
			"file_id":                    persisted.FileID,
			"blocks_inserted":            persisted.BlocksInserted,
			"blocks_skipped_duplicate":   persisted.BlocksSkippedDuplicate,
			"blocks_skipped_content_hash": persisted.BlocksSkippedContentHash,
			"inserted_block_ids":         persisted.InsertedBlockIDs,
			"full_script_block_id":       persisted.FullBlockID,
			"source_format":              persisted.SourceFormat,
		},
		"imported_blocks": blockSummaries,
	}
	if fullSummary != nil {
		payload["imported_full_block"] = fullSummary
		payload["full_script"] = map[string]any{
			"file_id":      persisted.FileID,
			"block_id":     persisted.FullBlockID,
			"filename":     result.Source.Filename,
			"action_count": result.Summary.ActionCount,
			"duration_ms":  result.Summary.DurationMS,
			"preview":      fullSummary["preview"],
			"actions":      fullSummary["actions"],
			"bpm":          fullSummary["bpm"],
			"pace_label":   fullSummary["pace_label"],
			"script_duration_ms": fullSummary["script_duration_ms"],
			"heatmap_stats": fullSummary["heatmap_stats"],
		}
	}
	return payload
}

func blockImportSummary(block funscript.BlockRecord, sourceFilename string, inserted map[string]struct{}) map[string]any {
	_, wasInserted := inserted[block.ID]
	return patternSummaryFromRecord(block, sourceFilename, wasInserted)
}

func (s *Server) handlePatternsMeta(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, patternLibraryMeta())
}

func (s *Server) handlePatternsList(w http.ResponseWriter, r *http.Request) {
	s.writePatternList(w, r, false)
}

func (s *Server) handlePatternIDs(w http.ResponseWriter, r *http.Request) {
	if s.library == nil {
		writeError(w, http.StatusServiceUnavailable, errLibraryUnavailable)
		return
	}
	filter := patternFilterFromRequest(r)
	filter.Offset = 0
	if filter.Limit <= 0 || filter.Limit > 10000 {
		filter.Limit = 5000
	}
	rows, err := s.library.Store().ListMotionBlocks(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, err := s.library.Store().CountMotionBlocks(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ids":      ids,
		"total":    total,
		"returned": len(ids),
	})
}

func (s *Server) handlePatternsCount(w http.ResponseWriter, r *http.Request) {
	if s.library == nil {
		writeError(w, http.StatusServiceUnavailable, errLibraryUnavailable)
		return
	}
	filter := patternFilterFromRequest(r)
	count, err := s.library.Store().CountMotionBlocks(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"count": count,
		"total": count,
	})
}

func (s *Server) writePatternList(w http.ResponseWriter, r *http.Request, _ bool) {
	if s.library == nil {
		writeError(w, http.StatusServiceUnavailable, errLibraryUnavailable)
		return
	}
	filter := patternFilterFromRequest(r)
	rows, err := s.library.Store().ListMotionBlocks(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, err := s.library.Store().CountMotionBlocks(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	fileNames := s.loadSourceFilenames(r.Context(), rows)
	summaries := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		summaries = append(summaries, patternSummaryFromBlock(row, fileNames[row.SourceFileID]))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"blocks": summaries,
		"items":  summaries,
		"total":  total,
		"offset": filter.Offset,
		"limit":  filter.Limit,
	})
}

func (s *Server) handlePatternGet(w http.ResponseWriter, r *http.Request) {
	if s.library == nil {
		writeError(w, http.StatusServiceUnavailable, errLibraryUnavailable)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	block, err := s.library.Store().GetMotionBlock(r.Context(), id)
	if err != nil {
		if err == library.ErrNotFound {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	fileName := ""
	if file, err := s.library.Store().GetFunscriptFile(r.Context(), block.SourceFileID); err == nil {
		fileName = file.Filename
	}
	writeJSON(w, http.StatusOK, patternSummaryFromBlock(block, fileName))
}

func (s *Server) handlePatternPut(w http.ResponseWriter, r *http.Request) {
	if s.library == nil {
		writeError(w, http.StatusServiceUnavailable, errLibraryUnavailable)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	var body struct {
		Favorite   *int    `json:"favorite,omitempty"`
		Blocked    *int    `json:"blocked,omitempty"`
		UserRating *int    `json:"user_rating,omitempty"`
		TagsJSON   *string `json:"tags_json,omitempty"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var favorite, blocked *bool
	if body.Favorite != nil {
		value := *body.Favorite != 0
		favorite = &value
	}
	if body.Blocked != nil {
		value := *body.Blocked != 0
		blocked = &value
	}
	var tags []string
	if body.TagsJSON != nil && strings.TrimSpace(*body.TagsJSON) != "" {
		if err := decodeStringJSON(*body.TagsJSON, &tags); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	if err := s.library.Store().UpdateMotionBlockFields(r.Context(), id, favorite, blocked, body.UserRating, tags); err != nil {
		if err == library.ErrNotFound {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.handlePatternGet(w, r)
}

func (s *Server) handlePatternDelete(w http.ResponseWriter, r *http.Request) {
	if s.library == nil {
		writeError(w, http.StatusServiceUnavailable, errLibraryUnavailable)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if err := s.library.Store().DeleteMotionBlock(r.Context(), id); err != nil {
		if err == library.ErrNotFound {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": id})
}

func (s *Server) handlePatternFeedback(w http.ResponseWriter, r *http.Request) {
	if s.library == nil {
		writeError(w, http.StatusServiceUnavailable, errLibraryUnavailable)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	var body struct {
		Feedback string `json:"feedback"`
		Note     string `json:"note,omitempty"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	_ = body.Note
	block, err := s.library.ApplyBlockFeedback(r.Context(), id, body.Feedback)
	if err != nil {
		if err == library.ErrNotFound {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"block_id":      id,
		"success_score": block.SuccessScore,
		"favorite":      boolInt(block.Favorite),
		"blocked":       boolInt(block.Blocked),
		"dislike_count": block.DislikeCount,
	})
}

func (s *Server) loadSourceFilenames(ctx context.Context, rows []library.MotionBlock) map[string]string {
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		if _, ok := out[row.SourceFileID]; ok {
			continue
		}
		file, err := s.library.Store().GetFunscriptFile(ctx, row.SourceFileID)
		if err != nil {
			continue
		}
		out[row.SourceFileID] = file.Filename
	}
	return out
}

func patternFilterFromRequest(r *http.Request) library.BlockFilter {
	filter := library.BlockFilter{
		Zone:         r.URL.Query().Get("zone"),
		Speed:        r.URL.Query().Get("speed"),
		Rhythm:       r.URL.Query().Get("rhythm"),
		StrokeLength: r.URL.Query().Get("stroke_length"),
		Query:        r.URL.Query().Get("q"),
		Sort:         r.URL.Query().Get("sort"),
		Offset:       queryInt(r, "offset", 0),
		Limit:        queryInt(r, "limit", 24),
		HideBlocked:  queryBool(r, "hide_blocked", true),
		FavoritesOnly: queryBool(r, "favorites_only", false),
	}
	if v := queryOptionalFloat(r, "min_intensity"); v != nil {
		filter.MinIntensity = v
	}
	if v := queryOptionalFloat(r, "max_intensity"); v != nil {
		filter.MaxIntensity = v
	}
	if v := queryOptionalInt(r, "min_duration_ms"); v != nil {
		filter.MinDurationMS = v
	}
	if v := queryOptionalInt(r, "max_duration_ms"); v != nil {
		filter.MaxDurationMS = v
	}
	if v := queryOptionalInt(r, "min_rating"); v != nil {
		filter.MinRating = v
	}
	return filter
}

func patternLibraryMeta() map[string]any {
	return map[string]any{
		"categories": []map[string]string{},
		"zones": []map[string]string{
			{"id": "top", "label": "Ponta / tip"},
			{"id": "middle", "label": "Meio / mid"},
			{"id": "bottom", "label": "Base / fundo"},
			{"id": "full", "label": "Full stroke"},
			{"id": "mixed", "label": "Misto"},
		},
		"speeds": []map[string]string{
			{"id": "slow", "label": "Slow"},
			{"id": "medium", "label": "Medium"},
			{"id": "fast", "label": "Fast"},
			{"id": "very_fast", "label": "Very fast"},
		},
		"rhythms": []map[string]string{
			{"id": "steady", "label": "Steady"},
			{"id": "pulsed", "label": "Pulsed"},
			{"id": "accelerating", "label": "Accelerating"},
			{"id": "decelerating", "label": "Decelerating"},
			{"id": "chaotic", "label": "Chaotic"},
			{"id": "pause_hold", "label": "Pause / hold"},
		},
		"stroke_lengths": []map[string]string{
			{"id": "micro", "label": "Micro"},
			{"id": "short", "label": "Short"},
			{"id": "medium", "label": "Medium"},
			{"id": "full", "label": "Full"},
		},
	}
}

func patternSummaryFromBlock(block library.MotionBlock, sourceFilename string) map[string]any {
	record := funscript.BlockRecord{
		ID:              block.ID,
		StartMS:         block.StartMS,
		EndMS:           block.EndMS,
		DurationMS:      block.DurationMS,
		MinPos:          block.MinPos,
		MaxPos:          block.MaxPos,
		AvgPos:          block.AvgPos,
		Amplitude:       block.Amplitude,
		Zone:            block.Zone,
		StrokeLength:    block.StrokeLength,
		Speed:           block.Speed,
		Rhythm:          block.Rhythm,
		Intensity:       block.Intensity,
		Tags:            block.Tags,
		Actions:         block.Actions,
		SemanticSummary: block.SemanticSummary,
		IsFullScript:    funscript.IsFullScriptBlock(block.ID, block.Zone, block.Tags),
	}
	summary := patternSummaryFromRecord(record, sourceFilename, false)
	summary["source_file_id"] = block.SourceFileID
	summary["favorite"] = boolInt(block.Favorite)
	summary["blocked"] = boolInt(block.Blocked)
	summary["success_score"] = block.SuccessScore
	summary["times_used"] = block.TimesUsed
	summary["dislike_count"] = block.DislikeCount
	if block.UserRating != nil {
		summary["user_rating"] = *block.UserRating
	}
	if block.CreatedAt != "" {
		summary["created_at"] = block.CreatedAt
	}
	return summary
}

func patternSummaryFromRecord(block funscript.BlockRecord, sourceFilename string, inserted bool) map[string]any {
	actions := storedToActions(block.Actions)
	durationMS := block.DurationMS
	if durationMS <= 0 {
		durationMS = maxInt(0, block.EndMS-block.StartMS)
	}
	bpmData := funscript.ComputeStrokeBPM(actions, durationMS)
	sourceTimeRange := block.SourceTimeRange
	if sourceTimeRange == "" {
		sourceTimeRange = formatSourceRange(block.StartMS, block.EndMS)
	}
	summary := map[string]any{
		"id":                 block.ID,
		"start_ms":           block.StartMS,
		"end_ms":             block.EndMS,
		"source_start_ms":    block.StartMS,
		"source_end_ms":      block.EndMS,
		"source_time_range":  sourceTimeRange,
		"motion_time_range":  firstNonEmpty(block.MotionTimeRange, sourceTimeRange),
		"source_range_slug":  block.SourceRangeSlug,
		"duration_ms":        durationMS,
		"min_pos":            block.MinPos,
		"max_pos":            block.MaxPos,
		"avg_pos":            block.AvgPos,
		"amplitude":          block.Amplitude,
		"zone":               nullIfEmpty(block.Zone),
		"stroke_length":      nullIfEmpty(block.StrokeLength),
		"speed":              nullIfEmpty(block.Speed),
		"rhythm":             nullIfEmpty(block.Rhythm),
		"intensity":          block.Intensity,
		"tags":               block.Tags,
		"action_count":       len(block.Actions),
		"playback_action_count": len(block.Actions),
		"actions":            block.Actions,
		"preview":            buildPreviewCurve(block.Actions),
		"script_duration_ms": durationMS,
		"heatmap_stats": map[string]any{
			"action_count": len(block.Actions),
			"duration_ms":  durationMS,
		},
		"semantic_summary": firstNonEmpty(block.SemanticSummary, funscript.SemanticSummaryFromRecord(block)),
		"display_name":     blockDisplayName(block.ID, sourceFilename, block.Zone, block.Speed, durationMS, sourceTimeRange),
		"is_full_script":   block.IsFullScript || funscript.IsFullScriptBlock(block.ID, block.Zone, block.Tags),
		"is_user_recorded": isUserRecordedBlock(block.Tags),
		"session_roles":    []string{},
	}
	if sourceFilename != "" {
		summary["source_filename"] = sourceFilename
		summary["source_display_name"] = library.ImportDisplayFilename(sourceFilename)
	}
	for key, value := range bpmData {
		summary[key] = value
	}
	if inserted {
		summary["inserted"] = true
	}
	return summary
}

func storedToActions(stored []funscript.StoredAction) []funscript.Action {
	out := make([]funscript.Action, len(stored))
	for i, action := range stored {
		out[i] = funscript.Action{At: action.At, Pos: action.Pos}
	}
	return out
}

func buildPreviewCurve(actions []funscript.StoredAction) []map[string]any {
	if len(actions) == 0 {
		return []map[string]any{}
	}
	const maxPoints = 48
	step := 1
	if len(actions) > maxPoints {
		step = (len(actions) + maxPoints - 1) / maxPoints
	}
	out := make([]map[string]any, 0, maxPoints)
	for i := 0; i < len(actions); i += step {
		action := actions[i]
		out = append(out, map[string]any{
			"t_ms": action.At,
			"pos":  action.Pos,
		})
	}
	last := actions[len(actions)-1]
	if out[len(out)-1]["t_ms"] != last.At {
		out = append(out, map[string]any{"t_ms": last.At, "pos": last.Pos})
	}
	return out
}

func blockDisplayName(id, sourceFilename, zone, speed string, durationMS int, sourceRange string) string {
	if sourceFilename != "" {
		stem := library.ImportDisplayFilename(sourceFilename)
		if zone != "" && speed != "" {
			return stem + " · " + zone + " " + speed
		}
		return stem
	}
	if sourceRange != "" {
		return id + " (" + sourceRange + ")"
	}
	if durationMS > 0 {
		return id
	}
	return id
}

func formatSourceRange(startMS, endMS int) string {
	return strconv.Itoa(startMS) + "-" + strconv.Itoa(endMS) + "ms"
}

func isUserRecordedBlock(tags []string) bool {
	for _, tag := range tags {
		if tag == "user_recorded" || tag == "mouse_recording" {
			return true
		}
	}
	return false
}

func queryInt(r *http.Request, key string, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func queryBool(r *http.Request, key string, fallback bool) bool {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func queryOptionalFloat(r *http.Request, key string) *float64 {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	return &value
}

func queryOptionalInt(r *http.Request, key string) *int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	return &value
}

func nullIfEmpty(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
