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
	ID             uint32                     `json:"Id"`
	DeviceIndex    uint32                     `json:"DeviceIndex"`
	DeviceName     string                     `json:"DeviceName"`
	DeviceMessages map[string]json.RawMessage `json:"DeviceMessages"`
}

type buttplugLinearFeature struct {
	FeatureDescriptor string `json:"FeatureDescriptor"`
	ActuatorType      string `json:"ActuatorType"`
	StepCount         uint32 `json:"StepCount"`
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
		DeviceIndex: device.DeviceIndex,
		DeviceName:  device.DeviceName,
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
	return snapshot
}
