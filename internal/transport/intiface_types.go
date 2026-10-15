package transport

import "net/http"

// IntifaceOptions configures the connection to a user-owned Intiface server.
type IntifaceOptions struct {
	Address       string
	ClientName    string
	QueueCapacity int
	HTTPClient    *http.Client
}

// IntifaceLinearActuator describes one selectable Buttplug LinearCmd feature.
type IntifaceLinearActuator struct {
	Index             uint32 `json:"index"`
	FeatureDescriptor string `json:"feature_descriptor,omitempty"`
	ActuatorType      string `json:"actuator_type,omitempty"`
	StepCount         uint32 `json:"step_count,omitempty"`
}

// IntifaceScalarActuator describes one Buttplug ScalarCmd feature. Index is
// its position in the device's full ScalarCmd list, which is how ScalarCmd
// addresses it, so it is kept when callers filter by actuator type.
type IntifaceScalarActuator struct {
	Index             uint32 `json:"index"`
	FeatureDescriptor string `json:"feature_descriptor,omitempty"`
	ActuatorType      string `json:"actuator_type"`
	StepCount         uint32 `json:"step_count,omitempty"`
}

// IntifaceDevice is a safe snapshot of a discovered Buttplug device. Both
// actuator lists are always present, empty when the device has none.
type IntifaceDevice struct {
	DeviceIndex                  uint32                   `json:"device_index"`
	DeviceName                   string                   `json:"device_name"`
	DeviceMessageTimingGapMillis uint32                   `json:"device_message_timing_gap_ms"`
	LinearActuators              []IntifaceLinearActuator `json:"linear_actuators"`
	ScalarActuators              []IntifaceScalarActuator `json:"scalar_actuators"`
}

// IntifaceDispatchStatus is a bounded, protocol-safe paced-dispatch record.
type IntifaceDispatchStatus struct {
	DeviceIndex                 uint32 `json:"device_index"`
	ActuatorIndex               uint32 `json:"actuator_index"`
	StartupAnchor               bool   `json:"startup_anchor,omitempty"`
	RelativeScheduledTimeMillis int64  `json:"relative_scheduled_time_ms"`
	ActualSendTime              string `json:"actual_send_time"`
	LatenessMillis              int64  `json:"lateness_ms"`
	EffectiveDurationMillis     int64  `json:"effective_duration_ms"`
	ACKLatencyMillis            *int64 `json:"ack_latency_ms,omitempty"`
	Status                      string `json:"status"`
}

// IntifaceStatus is a safe connection, selection, and pacer snapshot.
type IntifaceStatus struct {
	Connected                 bool                     `json:"connected"`
	Scanning                  bool                     `json:"scanning"`
	PlaybackState             string                   `json:"playback_state"`
	MaxPingTimeMillis         int64                    `json:"max_ping_time_ms"`
	QueueDepth                int                      `json:"queue_depth"`
	QueueCoverageMillis       int64                    `json:"queue_coverage_ms"`
	PendingACKs               int                      `json:"pending_acks"`
	LinearSentCount           uint64                   `json:"linear_sent_count"`
	LinearACKedCount          uint64                   `json:"linear_acked_count"`
	LinearRejectedCount       uint64                   `json:"linear_rejected_count"`
	LinearTimeoutCount        uint64                   `json:"linear_timeout_count"`
	LastACKLatencyMillis      int64                    `json:"last_ack_latency_ms"`
	MaxACKLatencyMillis       int64                    `json:"max_ack_latency_ms"`
	LastSendLatenessMillis    int64                    `json:"last_send_lateness_ms"`
	MaxSendLatenessMillis     int64                    `json:"max_send_lateness_ms"`
	CoalescedSegments         uint64                   `json:"coalesced_segments"`
	RecentDispatchesDropped   uint64                   `json:"recent_dispatches_dropped"`
	LastWireDurationMillis    int64                    `json:"last_wire_duration_ms"`
	SelectedResolutionPercent float64                  `json:"selected_resolution_percent"`
	LastPacerFailure          string                   `json:"last_pacer_failure,omitempty"`
	RecentDispatches          []IntifaceDispatchStatus `json:"recent_dispatches"`
	SelectedDeviceIndex       *uint32                  `json:"selected_device_index,omitempty"`
	SelectedActuatorIndex     *uint32                  `json:"selected_actuator_index,omitempty"`
	Devices                   []IntifaceDevice         `json:"devices"`
}
