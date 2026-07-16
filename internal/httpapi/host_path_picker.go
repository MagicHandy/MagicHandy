package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

var errHostPathPickerUnsupported = errors.New("the host path picker is not supported on this platform")

type hostPathPickerSpec struct {
	Title     string
	Filter    string
	Directory bool
}

type hostPathPicker func(context.Context, hostPathPickerSpec, string) (path string, canceled bool, err error)

func pathPickerSpec(kind string) (hostPathPickerSpec, bool) {
	switch kind {
	case "executable":
		return hostPathPickerSpec{Title: "Choose an executable", Filter: "Executables (*.exe)|*.exe|All files (*.*)|*.*"}, true
	case "gguf":
		return hostPathPickerSpec{Title: "Choose a GGUF model", Filter: "GGUF models (*.gguf)|*.gguf|All files (*.*)|*.*"}, true
	case "wav":
		return hostPathPickerSpec{Title: "Choose a WAV recording", Filter: "WAV audio (*.wav)|*.wav|All files (*.*)|*.*"}, true
	case "npy":
		return hostPathPickerSpec{Title: "Choose NeuCodec reference codes", Filter: "NumPy files (*.npy)|*.npy|All files (*.*)|*.*"}, true
	case "neutts_codes":
		return hostPathPickerSpec{Title: "Choose NeuTTS reference codes", Filter: "NeuTTS code tensors (*.pt;*.npy)|*.pt;*.npy|Torch tensors (*.pt)|*.pt|NumPy files (*.npy)|*.npy|All files (*.*)|*.*"}, true
	case "file":
		return hostPathPickerSpec{Title: "Choose a file", Filter: "All files (*.*)|*.*"}, true
	case "directory":
		return hostPathPickerSpec{Title: "Choose a folder", Directory: true}, true
	default:
		return hostPathPickerSpec{}, false
	}
}

func (s *Server) handleHostPathPicker(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRemote(r.RemoteAddr) || !isLoopbackHost(r.Host) || !isSameOriginBrowserRequest(r) {
		writeError(w, http.StatusForbidden, errors.New("the host path picker is available only from the computer running MagicHandy"))
		return
	}
	if mediaType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0])); mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, errors.New("the host path picker requires application/json"))
		return
	}
	// Unlike general API compatibility, this native-UI endpoint deliberately
	// rejects query-string controller IDs so a cross-origin simple request cannot
	// claim an expired lease and open a dialog.
	if !s.requireControllerID(w, strings.TrimSpace(r.Header.Get(controllerHeaderName))) {
		return
	}
	var body struct {
		Kind    string `json:"kind"`
		Current string `json:"current"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	spec, ok := pathPickerSpec(strings.TrimSpace(body.Kind))
	if !ok {
		writeError(w, http.StatusBadRequest, errors.New("path picker kind must be executable, gguf, wav, npy, neutts_codes, file, or directory"))
		return
	}
	path, canceled, err := s.hostPathPicker(r.Context(), spec, strings.TrimSpace(body.Current))
	if errors.Is(err, errHostPathPickerUnsupported) {
		writeError(w, http.StatusNotImplemented, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("the host path picker could not be opened"))
		return
	}
	if canceled {
		writeJSON(w, http.StatusOK, map[string]any{"path": "", "canceled": true})
		return
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("the selected path is invalid"))
		return
	}
	info, err := os.Stat(absPath)
	if err != nil || (spec.Directory && !info.IsDir()) || (!spec.Directory && !info.Mode().IsRegular()) {
		writeError(w, http.StatusBadRequest, errors.New("the selected path is no longer available"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": filepath.Clean(absPath), "canceled": false})
}

func isLoopbackHost(hostPort string) bool {
	host := strings.TrimSpace(hostPort)
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	host = strings.Trim(strings.ToLower(host), "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isSameOriginBrowserRequest(r *http.Request) bool {
	if site := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))); site != "" && site != "same-origin" && site != "none" {
		return false
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return parsed.Scheme == scheme && strings.EqualFold(parsed.Host, r.Host)
}

func isLoopbackRemote(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		host = strings.Trim(strings.TrimSpace(remoteAddr), "[]")
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
