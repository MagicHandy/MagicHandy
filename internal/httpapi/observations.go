package httpapi

import (
	"sync"
	"sync/atomic"
	"time"
)

// Revisions order backend observations, not client arrival times. Independent
// capture locks cover only in-memory snapshots; no network/database operation
// or response write may retain one of these locks.
type observationRuntime struct {
	settingsMu sync.Mutex
	motionMu   sync.Mutex
	sequence   atomic.Uint64
}

type observationStamp struct {
	Epoch      string    `json:"epoch"`
	Revision   uint64    `json:"revision"`
	ObservedAt time.Time `json:"observed_at"`
}

func (s *Server) observationStamp() observationStamp {
	return observationStamp{Epoch: s.controller.epoch, Revision: s.observations.sequence.Add(1), ObservedAt: time.Now().UTC()}
}
