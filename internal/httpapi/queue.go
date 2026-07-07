package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

type manualQueueItem struct {
	BlockID     string  `json:"block_id"`
	DurationSec float64 `json:"duration_sec"`
	Loop        bool    `json:"loop"`
}

type manualQueueRuntime struct {
	mu       sync.Mutex
	items    []manualQueueItem
	playing  bool
	paused   bool
	autoloop bool
}

func (s *Server) registerManualQueueRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/manual-queue", s.handleManualQueueGet)
	mux.HandleFunc("PUT /api/manual-queue", s.handleManualQueuePut)
	mux.HandleFunc("GET /api/manual-queue/preview", s.handleManualQueuePreview)
	mux.HandleFunc("POST /api/manual-queue/items", s.handleManualQueueAddItem)
	mux.HandleFunc("PATCH /api/manual-queue/items/{index}", s.handleManualQueuePatchItem)
	mux.HandleFunc("DELETE /api/manual-queue/items/{index}", s.handleManualQueueDeleteItem)
	mux.HandleFunc("POST /api/manual-queue/reorder", s.handleManualQueueReorder)
	mux.HandleFunc("POST /api/manual-queue/clear", s.handleManualQueueClear)
	mux.HandleFunc("POST /api/manual-queue/play", s.handleManualQueuePlay)
	mux.HandleFunc("POST /api/manual-queue/player/pause", s.handleManualQueuePause)
	mux.HandleFunc("POST /api/manual-queue/player/resume", s.handleManualQueueResume)
	mux.HandleFunc("POST /api/manual-queue/player/stop", s.handleManualQueueStop)
	mux.HandleFunc("POST /api/manual-queue/player/skip", s.handleManualQueueSkip)
}

func (s *Server) handleManualQueueGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.manualQueueDraft())
}

func (s *Server) handleManualQueuePut(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []manualQueueItem `json:"items"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	normalized := make([]manualQueueItem, 0, len(body.Items))
	for _, item := range body.Items {
		resolved, err := s.resolveManualQueueItem(r, item.BlockID, item.DurationSec)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		resolved.Loop = item.Loop
		normalized = append(normalized, resolved)
	}
	s.manualQueue.mu.Lock()
	s.manualQueue.items = normalized
	s.manualQueue.mu.Unlock()
	writeJSON(w, http.StatusOK, s.manualQueueDraft())
}

func (s *Server) handleManualQueueAddItem(w http.ResponseWriter, r *http.Request) {
	var body manualQueueItem
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resolved, err := s.resolveManualQueueItem(r, body.BlockID, body.DurationSec)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resolved.Loop = body.Loop
	s.manualQueue.mu.Lock()
	s.manualQueue.items = append(s.manualQueue.items, resolved)
	s.manualQueue.mu.Unlock()
	writeJSON(w, http.StatusOK, s.manualQueueDraft())
}

func (s *Server) handleManualQueuePatchItem(w http.ResponseWriter, r *http.Request) {
	index, err := manualQueueIndex(r)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var body struct {
		DurationSec *float64 `json:"duration_sec"`
		Loop        *bool    `json:"loop"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.manualQueue.mu.Lock()
	defer s.manualQueue.mu.Unlock()
	if index < 0 || index >= len(s.manualQueue.items) {
		writeError(w, http.StatusNotFound, errors.New("invalid queue index"))
		return
	}
	if body.DurationSec != nil {
		resolved, err := s.resolveManualQueueItem(r, s.manualQueue.items[index].BlockID, *body.DurationSec)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		s.manualQueue.items[index].DurationSec = resolved.DurationSec
	}
	if body.Loop != nil {
		s.manualQueue.items[index].Loop = *body.Loop
	}
	writeJSON(w, http.StatusOK, s.manualQueueDraftLocked())
}

func (s *Server) handleManualQueueDeleteItem(w http.ResponseWriter, r *http.Request) {
	index, err := manualQueueIndex(r)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	s.manualQueue.mu.Lock()
	defer s.manualQueue.mu.Unlock()
	if index < 0 || index >= len(s.manualQueue.items) {
		writeError(w, http.StatusNotFound, errors.New("invalid queue index"))
		return
	}
	s.manualQueue.items = append(s.manualQueue.items[:index], s.manualQueue.items[index+1:]...)
	writeJSON(w, http.StatusOK, s.manualQueueDraftLocked())
}

func (s *Server) handleManualQueueReorder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FromIndex int `json:"from_index"`
		ToIndex   int `json:"to_index"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.manualQueue.mu.Lock()
	defer s.manualQueue.mu.Unlock()
	items := s.manualQueue.items
	if body.FromIndex < 0 || body.FromIndex >= len(items) || body.ToIndex < 0 || body.ToIndex >= len(items) {
		writeError(w, http.StatusBadRequest, errors.New("invalid reorder indices"))
		return
	}
	item := items[body.FromIndex]
	items = append(items[:body.FromIndex], items[body.FromIndex+1:]...)
	if body.ToIndex > body.FromIndex {
		body.ToIndex--
	}
	items = append(items[:body.ToIndex], append([]manualQueueItem{item}, items[body.ToIndex:]...)...)
	s.manualQueue.items = items
	writeJSON(w, http.StatusOK, s.manualQueueDraftLocked())
}

func (s *Server) handleManualQueueClear(w http.ResponseWriter, _ *http.Request) {
	s.manualQueue.mu.Lock()
	s.manualQueue.items = nil
	s.manualQueue.mu.Unlock()
	writeJSON(w, http.StatusOK, s.manualQueueDraft())
}

func (s *Server) handleManualQueuePreview(w http.ResponseWriter, r *http.Request) {
	draft := s.manualQueueDraft()
	if draft["count"].(int) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":                 true,
			"preview":            []map[string]any{},
			"segments":           []map[string]any{},
			"duration_ms":        0,
			"total_duration_sec": 0.0,
			"action_count":       0,
		})
		return
	}
	preview, segments, durationMS, actionCount := s.buildManualQueuePreview(r, draft["items"].([]map[string]any))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                 true,
		"preview":            preview,
		"segments":           segments,
		"duration_ms":        durationMS,
		"total_duration_sec": float64(durationMS) / 1000.0,
		"action_count":       actionCount,
		"script_duration_ms": durationMS,
	})
}

func (s *Server) handleManualQueuePlay(w http.ResponseWriter, _ *http.Request) {
	draft := s.manualQueueDraft()
	if draft["count"].(int) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("manual queue is empty"))
		return
	}
	s.manualQueue.mu.Lock()
	s.manualQueue.playing = true
	s.manualQueue.paused = false
	s.manualQueue.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"started":  true,
		"items":    draft["count"],
		"autoloop": s.manualQueue.autoloop,
	})
}

func (s *Server) handleManualQueuePause(w http.ResponseWriter, _ *http.Request) {
	s.manualQueue.mu.Lock()
	defer s.manualQueue.mu.Unlock()
	if !s.manualQueue.playing {
		writeError(w, http.StatusConflict, errors.New("manual queue player is not active"))
		return
	}
	s.manualQueue.paused = true
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "paused": true})
}

func (s *Server) handleManualQueueResume(w http.ResponseWriter, _ *http.Request) {
	s.manualQueue.mu.Lock()
	defer s.manualQueue.mu.Unlock()
	if !s.manualQueue.playing {
		writeError(w, http.StatusConflict, errors.New("manual queue player is not active"))
		return
	}
	s.manualQueue.paused = false
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "paused": false})
}

func (s *Server) handleManualQueueStop(w http.ResponseWriter, r *http.Request) {
	s.manualQueue.mu.Lock()
	s.manualQueue.playing = false
	s.manualQueue.paused = false
	s.manualQueue.mu.Unlock()
	if commandTransport, err := s.newSelectedMotionTransport(); err == nil {
		_, _ = commandTransport.Stop(r.Context(), transport.StopCommand{Reason: "manual_queue_stop"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "stopped": true})
}

func (s *Server) handleManualQueueSkip(w http.ResponseWriter, _ *http.Request) {
	s.manualQueue.mu.Lock()
	defer s.manualQueue.mu.Unlock()
	if !s.manualQueue.playing {
		writeError(w, http.StatusConflict, errors.New("manual queue player is not active"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "skip_to_ms": 0})
}

func (s *Server) manualQueueDraft() map[string]any {
	s.manualQueue.mu.Lock()
	defer s.manualQueue.mu.Unlock()
	return s.manualQueueDraftLocked()
}

func (s *Server) manualQueueDraftLocked() map[string]any {
	items := make([]map[string]any, 0, len(s.manualQueue.items))
	total := 0.0
	for _, item := range s.manualQueue.items {
		displayName := item.BlockID
		if s.library != nil {
			if block, err := s.library.Store().GetMotionBlock(contextBackground(), item.BlockID); err == nil {
				if file, err := s.library.Store().GetFunscriptFile(contextBackground(), block.SourceFileID); err == nil {
					displayName = blockDisplayName(block.ID, file.Filename, block.Zone, block.Speed, block.DurationMS, "")
				}
			}
		}
		items = append(items, map[string]any{
			"block_id":     item.BlockID,
			"duration_sec": item.DurationSec,
			"loop":         item.Loop,
			"display_name": displayName,
		})
		total += item.DurationSec
	}
	return map[string]any{
		"items":              items,
		"count":              len(items),
		"total_duration_sec": round2(total),
	}
}

func (s *Server) resolveManualQueueItem(_ *http.Request, blockID string, durationSec float64) (manualQueueItem, error) {
	return s.resolveManualQueueItemCtx(blockID, durationSec)
}

func (s *Server) buildManualQueuePreview(r *http.Request, items []map[string]any) ([]map[string]any, []map[string]any, int, int) {
	preview := make([]map[string]any, 0, 128)
	segments := make([]map[string]any, 0, len(items))
	offsetMS := 0
	actionCount := 0
	for index, item := range items {
		blockID, _ := item["block_id"].(string)
		durationSec, _ := item["duration_sec"].(float64)
		durationMS := int(durationSec * 1000)
		if s.library == nil {
			continue
		}
		block, err := s.library.Store().GetMotionBlock(r.Context(), blockID)
		if err != nil {
			continue
		}
		displayName := blockDisplayName(block.ID, "", block.Zone, block.Speed, block.DurationMS, "")
		segments = append(segments, map[string]any{
			"index":        index,
			"start_ms":     offsetMS,
			"duration_ms":  durationMS,
			"display_name": displayName,
			"block_id":     blockID,
		})
		for _, action := range block.Actions {
			preview = append(preview, map[string]any{
				"t_ms": offsetMS + action.At,
				"pos":  action.Pos,
			})
		}
		actionCount += len(block.Actions)
		offsetMS += durationMS
	}
	return preview, segments, offsetMS, actionCount
}

func manualQueueIndex(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.PathValue("index"))
	if raw == "" {
		return 0, errors.New("invalid queue index")
	}
	index, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("invalid queue index")
	}
	return index, nil
}

func round2(value float64) float64 {
	return float64(int(value*100+0.5)) / 100
}

func contextBackground() context.Context {
	return context.Background()
}

func (s *Server) enqueueChatLibraryBlock(blockID string) error {
	blockID = strings.TrimSpace(blockID)
	if blockID == "" {
		return errors.New("library_block_id is required")
	}
	item, err := s.resolveManualQueueItemCtx(blockID, 0)
	if err != nil {
		return err
	}
	s.manualQueue.mu.Lock()
	s.manualQueue.items = append(s.manualQueue.items, item)
	s.manualQueue.playing = true
	s.manualQueue.paused = false
	s.manualQueue.mu.Unlock()
	return nil
}

func (s *Server) resolveManualQueueItemCtx(blockID string, durationSec float64) (manualQueueItem, error) {
	blockID = strings.TrimSpace(blockID)
	if blockID == "" {
		return manualQueueItem{}, errors.New("block_id is required")
	}
	if s.library == nil {
		return manualQueueItem{}, errLibraryUnavailable
	}
	block, err := s.library.Store().GetMotionBlock(contextBackground(), blockID)
	if err != nil {
		return manualQueueItem{}, err
	}
	if durationSec <= 0 {
		durationSec = float64(block.DurationMS) / 1000.0
		if durationSec <= 0 {
			durationSec = 5
		}
	}
	return manualQueueItem{
		BlockID:     blockID,
		DurationSec: durationSec,
	}, nil
}
