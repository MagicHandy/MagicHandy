package httpapi

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

var errNoPlaybackDevices = errors.New("include a device in playback before starting motion")

// deviceLabel names a playback device in linked-session errors and warnings.
func deviceLabel(device string) string {
	switch device {
	case config.DeviceOutputHandy:
		return "Handy"
	case config.DeviceOutputCoyote:
		return "Coyote"
	default:
		return device
	}
}

// motionOwnerKey names what an engine was built for: the playback set and,
// when the Handy plays, its dispatch owner. Any change replaces the engine.
func motionOwnerKey(device config.DeviceSettings) string {
	devices := device.PlaybackSet()
	key := strings.Join(devices, "+")
	if key == "" {
		key = config.DeviceOutputNone
	}
	if slices.Contains(devices, config.DeviceOutputHandy) {
		key += "/" + device.HSPDispatchOwner
	}
	return key
}

// engineOwnerPlays reports whether the engine built for owner drives device.
func engineOwnerPlays(owner, device string) bool {
	devices, _, _ := strings.Cut(owner, "/")
	return slices.Contains(strings.Split(devices, "+"), device)
}

// newSelectedMotionTransport builds what the playback set drives: one device
// directly, or several through one linked session whose first device is the
// run clock. Companions are prepared before the clock device, so a companion
// that is not ready fails before the Handy's transport is touched.
func (s *Server) newSelectedMotionTransport() (transport.Transport, error) {
	settings, _ := s.store.Snapshot()
	devices := settings.Device.PlaybackSet()
	if len(devices) == 0 {
		return nil, errNoPlaybackDevices
	}
	participants := make([]transport.LinkedParticipant, len(devices))
	for index := len(devices) - 1; index >= 0; index-- {
		owner, err := s.playbackDeviceTransport(settings, devices[index])
		if err != nil {
			return nil, err
		}
		participants[index] = transport.LinkedParticipant{ID: devices[index], Label: deviceLabel(devices[index]), Transport: owner}
	}
	if len(participants) == 1 {
		return participants[0].Transport, nil
	}
	return transport.NewLinkedSession(participants, settings.Device.LinkedFailurePolicy == config.LinkedFailureContinueHealthy), nil
}

func (s *Server) playbackDeviceTransport(settings config.Settings, device string) (transport.Transport, error) {
	switch device {
	case config.DeviceOutputHandy:
		return s.newHandyMotionTransport(settings)
	case config.DeviceOutputCoyote:
		return s.coyoteMotionTransport()
	default:
		return nil, fmt.Errorf("unknown playback device %q", device)
	}
}

// newSelectedStopTransport reaches every configured device, whether or not
// it plays, so Stop without an engine still silences a prepared device.
func (s *Server) newSelectedStopTransport() (transport.Transport, error) {
	settings, _ := s.store.Snapshot()
	var participants []transport.LinkedParticipant
	if settings.Device.HasDevice(config.DeviceOutputHandy) {
		handy, err := s.newHandyStopTransport(settings)
		if err != nil {
			return nil, err
		}
		participants = append(participants, transport.LinkedParticipant{ID: config.DeviceOutputHandy, Label: deviceLabel(config.DeviceOutputHandy), Transport: handy})
	}
	if settings.Device.HasDevice(config.DeviceOutputCoyote) {
		participants = append(participants, transport.LinkedParticipant{ID: config.DeviceOutputCoyote, Label: deviceLabel(config.DeviceOutputCoyote), Transport: s.coyote.owner})
	}
	switch len(participants) {
	case 0:
		return nil, nil
	case 1:
		return participants[0].Transport, nil
	default:
		return transport.NewLinkedSession(participants, false), nil
	}
}
