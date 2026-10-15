package transport

import (
	"encoding/json"
	"fmt"
)

const buttplugMessageVersion = 3

type buttplugMessage struct {
	kind    string
	payload json.RawMessage
	id      uint32
}

type buttplugServerInfo struct {
	ID             uint32 `json:"Id"`
	MessageVersion int    `json:"MessageVersion"`
	MaxPingTime    int64  `json:"MaxPingTime"`
}

type buttplugDeviceList struct {
	ID      uint32           `json:"Id"`
	Devices []buttplugDevice `json:"Devices"`
}

type buttplugDevice struct {
	ID                     uint32                     `json:"Id"`
	DeviceIndex            uint32                     `json:"DeviceIndex"`
	DeviceName             string                     `json:"DeviceName"`
	DeviceMessageTimingGap uint32                     `json:"DeviceMessageTimingGap"`
	DeviceMessages         map[string]json.RawMessage `json:"DeviceMessages"`
}

type buttplugLinearFeature struct {
	FeatureDescriptor string `json:"FeatureDescriptor"`
	ActuatorType      string `json:"ActuatorType"`
	StepCount         uint32 `json:"StepCount"`
}

// buttplugScalarFeature is one ScalarCmd attribute (spec v3). ScalarCmd
// addresses it by its position in the device's full ScalarCmd list.
type buttplugScalarFeature struct {
	FeatureDescriptor string `json:"FeatureDescriptor"`
	ActuatorType      string `json:"ActuatorType"`
	StepCount         uint32 `json:"StepCount"`
}

// buttplugScalarCmd is the spec v3 level command. Spec v4 replaces it with
// OutputCmd; encodeScalarCmd is the one place that encoding would change.
const buttplugScalarCmd = "ScalarCmd"

// scalarLevel is one actuator's level: its index in the device's full
// ScalarCmd list and a fraction already quantized to its steps.
type scalarLevel struct {
	index uint32
	level float64
}

// encodeScalarCmd encodes one ScalarCmd for actuators of one type.
func encodeScalarCmd(deviceIndex uint32, actuatorType string, levels []scalarLevel) map[string]any {
	scalars := make([]map[string]any, 0, len(levels))
	for _, level := range levels {
		scalars = append(scalars, map[string]any{"Index": level.index, "Scalar": level.level, "ActuatorType": actuatorType})
	}
	return map[string]any{"DeviceIndex": deviceIndex, "Scalars": scalars}
}

type buttplugDeviceRemoved struct {
	DeviceIndex uint32 `json:"DeviceIndex"`
}

type buttplugError struct {
	ErrorCode    int    `json:"ErrorCode"`
	ErrorMessage string `json:"ErrorMessage"`
}

func decodeButtplugMessages(data []byte) ([]buttplugMessage, error) {
	var envelopes []map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelopes); err != nil {
		return nil, fmt.Errorf("decode Buttplug message array: %w", err)
	}
	if len(envelopes) == 0 {
		return nil, fmt.Errorf("buttplug message array is empty")
	}

	messages := make([]buttplugMessage, 0, len(envelopes))
	for _, envelope := range envelopes {
		if len(envelope) != 1 {
			return nil, fmt.Errorf("buttplug message envelope must contain one message")
		}
		for kind, payload := range envelope {
			var header struct {
				ID uint32 `json:"Id"`
			}
			if err := json.Unmarshal(payload, &header); err != nil {
				return nil, fmt.Errorf("decode Buttplug %s header: %w", kind, err)
			}
			messages = append(messages, buttplugMessage{kind: kind, payload: payload, id: header.ID})
		}
	}
	return messages, nil
}

func intifaceDeviceFromProtocol(device buttplugDevice) IntifaceDevice {
	snapshot := IntifaceDevice{
		DeviceIndex:                  device.DeviceIndex,
		DeviceName:                   device.DeviceName,
		DeviceMessageTimingGapMillis: device.DeviceMessageTimingGap,
		LinearActuators:              []IntifaceLinearActuator{},
		ScalarActuators:              []IntifaceScalarActuator{},
	}
	var features []buttplugLinearFeature
	if raw := device.DeviceMessages["LinearCmd"]; len(raw) != 0 {
		_ = json.Unmarshal(raw, &features)
	}
	for index, feature := range features {
		snapshot.LinearActuators = append(snapshot.LinearActuators, IntifaceLinearActuator{
			Index:             uint32(index),
			FeatureDescriptor: feature.FeatureDescriptor,
			ActuatorType:      feature.ActuatorType,
			StepCount:         feature.StepCount,
		})
	}
	var scalars []buttplugScalarFeature
	if raw := device.DeviceMessages["ScalarCmd"]; len(raw) != 0 {
		_ = json.Unmarshal(raw, &scalars)
	}
	for index, feature := range scalars {
		snapshot.ScalarActuators = append(snapshot.ScalarActuators, IntifaceScalarActuator{
			Index:             uint32(index),
			FeatureDescriptor: feature.FeatureDescriptor,
			ActuatorType:      feature.ActuatorType,
			StepCount:         feature.StepCount,
		})
	}
	return snapshot
}
