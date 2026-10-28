package transport

import (
	"slices"
	"time"
)

const vibratorEventCapacity = 128

// VibratorEvent records only bounded operational facts, never model text,
// device names/addresses, credentials, or raw server errors. Command is a local
// sequence joining a scalar write to its acknowledgement.
type VibratorEvent struct {
	At                string               `json:"at"`
	Kind              string               `json:"kind"`
	Command           uint64               `json:"command,omitempty"`
	Generation        uint64               `json:"generation"`
	ModelLevelPercent int                  `json:"model_level_percent"`
	CeilingPercent    int                  `json:"ceiling_percent"`
	Suspended         bool                 `json:"suspended,omitempty"`
	State             string               `json:"state"`
	Levels            []VibratorMotorLevel `json:"levels,omitempty"`
}

// VibratorMotorLevel is a commanded, quantized estimate, not motor telemetry.
type VibratorMotorLevel struct {
	Index   uint32  `json:"index"`
	Percent float64 `json:"percent"`
}

func (v *IntifaceVibrator) addEventLocked(kind string, command uint64, steps []int) {
	event := VibratorEvent{At: v.clock().UTC().Format(time.RFC3339Nano), Kind: kind,
		Command: command, Generation: v.stream.generation, ModelLevelPercent: percent(v.effectiveLevelLocked()),
		CeilingPercent: percent(v.config.Ceiling), Suspended: v.suspended, State: v.playbackStateLocked()}
	for index, step := range steps {
		if index < len(v.selection.motors) {
			motor := v.selection.motors[index]
			event.Levels = append(event.Levels, VibratorMotorLevel{Index: motor.Index, Percent: 100 * float64(step) / float64(motor.StepCount)})
		}
	}
	if len(v.events) == vibratorEventCapacity {
		copy(v.events, v.events[1:])
		v.events[len(v.events)-1] = event
		v.eventsDropped++
	} else {
		v.events = append(v.events, event)
	}
}

// Events returns a detached snapshot for diagnostic exports. Ordinary status
// polling only carries the current levels, not this history.
func (v *IntifaceVibrator) Events() ([]VibratorEvent, uint64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	events := slices.Clone(v.events)
	for index := range events {
		events[index].Levels = slices.Clone(events[index].Levels)
	}
	return events, v.eventsDropped
}
