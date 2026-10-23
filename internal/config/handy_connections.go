package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// MaxHandys bounds the saved inventory and concurrent transport resources.
// The original Handy remains the compatibility instance named "handy".
const MaxHandys = 8

// HandyConnection is an additional, independently connected Handy. The
// primary instance retains the original settings and API for compatibility.
type HandyConnection struct {
	ID                     string `json:"id"`
	Name                   string `json:"name"`
	Method                 string `json:"method"`
	HandyModel             string `json:"handy_model"`
	ConnectionKey          string `json:"connection_key,omitempty"`
	NativeBluetoothAddress string `json:"native_bluetooth_address,omitempty"`
}

// PublicHandyConnection exposes connection settings without credentials or hardware IDs.
type PublicHandyConnection struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Method           string `json:"method"`
	HandyModel       string `json:"handy_model"`
	ConnectionKeySet bool   `json:"connection_key_set"`
	RememberedDevice bool   `json:"remembered_device"`
}

// HandyConnectionPatch names only the fields changed by the host.
type HandyConnectionPatch struct {
	Name               *string `json:"name,omitempty"`
	Method             *string `json:"method,omitempty"`
	HandyModel         *string `json:"handy_model,omitempty"`
	ConnectionKey      *string `json:"connection_key,omitempty"`
	ClearConnectionKey bool    `json:"clear_connection_key,omitempty"`
}

// IsHandyConnectionID recognizes opaque instance IDs, never physical IDs.
func IsHandyConnectionID(id string) bool {
	value, ok := strings.CutPrefix(id, "handy-")
	if !ok || len(value) != 12 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// HandyConnection finds an additional instance by its opaque ID.
func (d DeviceSettings) HandyConnection(id string) (HandyConnection, bool) {
	for _, c := range d.HandyConnections {
		if c.ID == id {
			return c, true
		}
	}
	return HandyConnection{}, false
}

func (d DeviceSettings) publicHandyConnections() []PublicHandyConnection {
	result := make([]PublicHandyConnection, 0, len(d.HandyConnections))
	for _, c := range d.HandyConnections {
		result = append(result, PublicHandyConnection{ID: c.ID, Name: c.Name, Method: c.Method,
			HandyModel: c.HandyModel, ConnectionKeySet: c.ConnectionKey != "", RememberedDevice: c.NativeBluetoothAddress != ""})
	}
	return result
}

// AddHandyConnection saves an excluded, disconnected instance.
func (s Settings) AddHandyConnection(id string, patch HandyConnectionPatch) (Settings, error) {
	next := cloneSettings(s)
	if len(next.Device.HandyConnections) >= MaxHandys-1 {
		return s, fmt.Errorf("at most %d Handys can be saved", MaxHandys)
	}
	if !IsHandyConnectionID(id) {
		return s, errors.New("invalid Handy connection ID")
	}
	if _, exists := next.Device.HandyConnection(id); exists {
		return s, errors.New("handy connection already exists")
	}
	c := HandyConnection{ID: id, Name: fmt.Sprintf("The Handy %d", len(next.Device.HandyConnections)+2),
		Method: DispatchOwnerCloudREST, HandyModel: HandyModelOriginal}
	c = applyHandyConnectionPatch(c, patch)
	next.Device = next.Device.materialized()
	next.Device.HandyConnections = append(next.Device.HandyConnections, c)
	next.Device.ConfiguredDevices = append(next.Device.ConfiguredDevices, id)
	return NormalizeSettings(next)
}

// PatchHandyConnection updates an instance without mutating its input.
func (s Settings) PatchHandyConnection(id string, patch HandyConnectionPatch) (Settings, error) {
	if patch == (HandyConnectionPatch{}) {
		return s, errors.New("the Handy change names no setting")
	}
	next := cloneSettings(s)
	for i, c := range next.Device.HandyConnections {
		if c.ID == id {
			next.Device.HandyConnections[i] = applyHandyConnectionPatch(c, patch)
			return NormalizeSettings(next)
		}
	}
	return s, errors.New("handy connection does not exist")
}

func applyHandyConnectionPatch(c HandyConnection, patch HandyConnectionPatch) HandyConnection {
	patchString(&c.Name, patch.Name)
	patchString(&c.Method, patch.Method)
	patchString(&c.HandyModel, patch.HandyModel)
	patchString(&c.ConnectionKey, patch.ConnectionKey)
	if patch.ClearConnectionKey {
		c.ConnectionKey = ""
	}
	return c
}

// RemoveHandyConnection removes an instance from inventory and playback.
func (s Settings) RemoveHandyConnection(id string) (Settings, error) {
	if _, ok := s.Device.HandyConnection(id); !ok {
		return s, errors.New("handy connection does not exist")
	}
	next := cloneSettings(s)
	next.Device = next.Device.materialized()
	next.Device.HandyConnections = slices.DeleteFunc(next.Device.HandyConnections, func(c HandyConnection) bool { return c.ID == id })
	next.Device.ConfiguredDevices = slices.DeleteFunc(next.Device.ConfiguredDevices, func(v string) bool { return v == id })
	next.Device = next.Device.WithPlaybackDevices(slices.DeleteFunc(next.Device.PlaybackDevices, func(v string) bool { return v == id }))
	return NormalizeSettings(next)
}

func validateHandyConnections(d DeviceSettings) error {
	if len(d.HandyConnections) >= MaxHandys {
		return fmt.Errorf("at most %d Handys can be saved", MaxHandys)
	}
	ids, keys, addresses := map[string]bool{}, map[string]bool{}, map[string]bool{}
	if d.HandyConnectionKey != "" {
		keys[d.HandyConnectionKey] = true
	}
	if d.NativeBluetoothAddress != "" {
		addresses[strings.ToLower(d.NativeBluetoothAddress)] = true
	}
	for _, c := range d.HandyConnections {
		if !IsHandyConnectionID(c.ID) || ids[c.ID] || !slices.Contains(d.ConfiguredDevices, c.ID) {
			return errors.New("invalid or duplicate Handy connection")
		}
		ids[c.ID] = true
		if err := validateHandyConnection(c); err != nil {
			return err
		}
		if c.ConnectionKey != "" {
			if keys[c.ConnectionKey] {
				return errors.New("this Handy connection key is already saved for another device")
			}
			keys[c.ConnectionKey] = true
		}
		if c.NativeBluetoothAddress != "" {
			address := strings.ToLower(c.NativeBluetoothAddress)
			if addresses[address] {
				return errors.New("this Bluetooth device is already saved for another Handy")
			}
			addresses[address] = true
		}
	}
	for _, id := range d.ConfiguredDevices {
		if IsHandyConnectionID(id) && !ids[id] {
			return errors.New("configured Handy connection is missing its settings")
		}
	}
	return nil
}

// EffectiveMotion uses the most conservative normalized travel-speed profile
// of participating Handys. Calibration is shared because there is one timeline:
// Standard (125mm/400mm/s), Pro (125mm/450mm/s), Original (110mm/400mm/s).
// This ordering also holds at the profiles' minimum speeds.
func (s Settings) EffectiveMotion() MotionSettings {
	result := s.Motion
	rank := map[string]int{HandyModel2Standard: 0, HandyModel2Pro: 1, HandyModelOriginal: 2}
	selected := 3
	for _, id := range s.Device.PlaybackSet() {
		model := ""
		if id == DeviceOutputHandy {
			model = s.Motion.HandyModel
		} else if c, ok := s.Device.HandyConnection(id); ok {
			model = c.HandyModel
		}
		if value, ok := rank[model]; ok && value < selected {
			selected, result.HandyModel = value, model
		}
	}
	return result
}

// PrivateDeviceValues provides credentials and hardware IDs to redactors.
func (d DeviceSettings) PrivateDeviceValues() []string {
	values := []string{d.HandyConnectionKey, d.NativeBluetoothAddress}
	for _, c := range d.HandyConnections {
		values = append(values, c.ConnectionKey, c.NativeBluetoothAddress)
	}
	return values
}

func validateHandyConnection(c HandyConnection) error {
	if strings.TrimSpace(c.Name) == "" || len([]rune(c.Name)) > 80 || strings.ContainsFunc(c.Name, unicode.IsControl) {
		return errors.New("handy name must be 1 to 80 characters without control characters")
	}
	if !oneOf(c.Method, DispatchOwnerCloudREST, DispatchOwnerNativeBluetooth, DispatchOwnerIntiface) {
		return errors.New("additional Handys use Cloud, Native Bluetooth or Intiface")
	}
	if !oneOf(c.HandyModel, HandyModelOriginal, HandyModel2Standard, HandyModel2Pro) {
		return errors.New("unknown Handy model")
	}
	if len(c.ConnectionKey) > 128 || strings.ContainsFunc(c.ConnectionKey, unicode.IsControl) {
		return errors.New("invalid Handy connection key")
	}
	return nil
}
