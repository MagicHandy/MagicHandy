package httpapi

import (
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func (s *Server) clientTransportDiagnostics(r *http.Request, status transport.TransportDiagnostics) transport.TransportDiagnostics {
	if s.capabilities(r).ConfigureHost {
		return status
	}
	return transport.TransportDiagnostics{
		Name: status.Name, Connected: status.Connected, PlaybackState: status.PlaybackState,
		CommandCount: status.CommandCount, LastLatencyMillis: status.LastLatencyMillis,
		LastError: clientFailure(status.LastError),
	}
}

func (s *Server) clientBluetoothSnapshot(r *http.Request) transport.BrowserBluetoothBridgeSnapshot {
	status := s.bluetoothSnapshot()
	if s.capabilities(r).ConfigureHost {
		return status
	}
	connectionState := "disconnected"
	switch {
	case status.Stale:
		connectionState = "stale"
	case status.Ready:
		connectionState = "ready"
	case status.Connected:
		connectionState = "connected"
	case !status.Supported:
		connectionState = "unsupported"
	}
	return transport.BrowserBluetoothBridgeSnapshot{
		Transport: status.Transport, Connected: status.Connected, Ready: status.Ready,
		Supported: status.Supported, Stale: status.Stale, Status: connectionState,
		ClientID: status.ClientID, Pending: status.Pending, Inflight: status.Inflight,
		LastSeenAgeMillis: status.LastSeenAgeMillis, LastError: clientFailure(status.LastError),
	}
}

func (s *Server) clientIntifaceSnapshot(r *http.Request) intifaceSnapshot {
	view := s.intifaceSnapshot()
	if s.capabilities(r).ConfigureHost {
		return view
	}
	status := view.Status
	view.Address = ""
	view.Diagnostics = s.clientTransportDiagnostics(r, view.Diagnostics)
	view.Status = transport.IntifaceStatus{
		Connected: status.Connected, Scanning: status.Scanning, PlaybackState: status.PlaybackState,
		MaxPingTimeMillis: status.MaxPingTimeMillis, QueueDepth: status.QueueDepth, QueueCoverageMillis: status.QueueCoverageMillis,
		PendingACKs: status.PendingACKs, LinearSentCount: status.LinearSentCount, LinearACKedCount: status.LinearACKedCount,
		LinearRejectedCount: status.LinearRejectedCount, LinearTimeoutCount: status.LinearTimeoutCount,
		LastACKLatencyMillis: status.LastACKLatencyMillis, MaxACKLatencyMillis: status.MaxACKLatencyMillis,
		LastSendLatenessMillis: status.LastSendLatenessMillis, MaxSendLatenessMillis: status.MaxSendLatenessMillis,
		SelectedDeviceIndex: status.SelectedDeviceIndex, SelectedActuatorIndex: status.SelectedActuatorIndex,
		SelectedResolutionPercent: status.SelectedResolutionPercent, LastPacerFailure: clientFailure(status.LastPacerFailure),
		Devices: []transport.IntifaceDevice{}, RecentDispatches: []transport.IntifaceDispatchStatus{},
	}
	return view
}
