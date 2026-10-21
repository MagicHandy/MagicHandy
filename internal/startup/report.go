package startup

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

// Attempt describes a bind without retaining an address, path, raw error or key.
type Attempt struct {
	Listener   string `json:"listener"`
	Port       int    `json:"port"`
	ErrorCode  string `json:"error_code,omitempty"`
	NativeCode int    `json:"native_code,omitempty"`
}

// Report is an allowlist of startup facts, safe to retain before the API exists.
type Report struct {
	SchemaVersion int       `json:"schema_version"`
	AppVersion    string    `json:"app_version"`
	Commit        string    `json:"commit"`
	Platform      string    `json:"platform"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
	Stage         string    `json:"stage"`
	Outcome       string    `json:"outcome"`
	Policy        string    `json:"port_policy,omitempty"`
	ErrorCode     string    `json:"error_code,omitempty"`
	NativeCode    int       `json:"native_code,omitempty"`
	Attempts      []Attempt `json:"attempts"`
}

// Reports keeps the latest failure and recovery across ordinary clean launches.
type Reports struct {
	Current      *Report `json:"current"`
	LastFailure  *Report `json:"last_failure"`
	LastRecovery *Report `json:"last_recovery"`
}

// Recorder is used only by the startup goroutine. Readers use atomic disk files.
type Recorder struct {
	Report  Report
	dataDir string
}

// NewRecorder does not open the datastore or require a writable data directory.
func NewRecorder(dataDir, version, commit string) *Recorder {
	return &Recorder{dataDir: dataDir, Report: Report{
		SchemaVersion: 1, AppVersion: version, Commit: commit, Platform: runtime.GOOS + "/" + runtime.GOARCH,
		StartedAt: time.Now().UTC(), Stage: "instance", Attempts: []Attempt{},
	}}
}

// Bind records each attempt and returns whether a different port could help.
func (r *Recorder) Bind(listener, address string, err error) bool {
	_, port, _ := net.SplitHostPort(address)
	number, _ := strconv.Atoi(port)
	code, native := errorCode(err)
	r.Report.Attempts = append(r.Report.Attempts, Attempt{Listener: listener, Port: number, ErrorCode: code, NativeCode: native})
	return code == "port_in_use" || code == "permission_denied"
}

func errorCode(err error) (string, int) {
	if err == nil {
		return "", 0
	}
	var native syscall.Errno
	_ = errors.As(err, &native)
	switch {
	case errors.Is(err, syscall.EADDRINUSE), native == 10048:
		return "port_in_use", int(native)
	case errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM), native == 10013:
		return "permission_denied", int(native)
	default:
		return "startup_failed", int(native)
	}
}

// Finish writes bounded documents. A storage failure falls back to a private
// temporary directory; the returned path can still be shown in the console.
func (r *Recorder) Finish(err error) (string, error) {
	r.Report.FinishedAt = time.Now().UTC()
	r.Report.Outcome = "ready"
	if err != nil {
		r.Report.Outcome = "failed"
		r.Report.ErrorCode, r.Report.NativeCode = errorCode(err)
	} else {
		r.Report.Stage = "ready"
		for _, attempt := range r.Report.Attempts {
			if attempt.ErrorCode != "" {
				r.Report.Outcome = "recovered"
			}
		}
	}
	root, prepareErr := prepareDirectory(r.dataDir)
	if prepareErr == nil {
		if err := r.save(root); err == nil {
			return filepath.Join(root, "current.json"), nil
		}
	}
	root, err = os.MkdirTemp("", "magichandy-startup-")
	if err != nil {
		return "", err
	}
	if err := restrictPath(root); err != nil {
		return "", err
	}
	return filepath.Join(root, "current.json"), r.save(root)
}

func (r *Recorder) save(root string) error {
	if err := writeDocument(root, "current.json", r.Report); err != nil {
		return err
	}
	if r.Report.Outcome == "failed" {
		return writeDocument(root, "last-failure.json", r.Report)
	}
	if r.Report.Outcome == "recovered" {
		return writeDocument(root, "last-recovery.json", r.Report)
	}
	return nil
}

// ReadReports excludes the instance record and its private identity token.
func ReadReports(dataDir string) Reports {
	return Reports{Current: readReport(dataDir, "current.json"), LastFailure: readReport(dataDir, "last-failure.json"), LastRecovery: readReport(dataDir, "last-recovery.json")}
}

func readReport(dataDir, name string) *Report {
	var report Report
	if readDocument(dataDir, name, &report) != nil || report.SchemaVersion != 1 {
		return nil
	}
	return &report
}

// PortHint preserves a successful local origin when the preferred port was
// unavailable. Preference keys prevent a changed configuration inheriting it.
type PortHint struct {
	Preference string `json:"preference"`
	Main       int    `json:"main"`
	Remote     int    `json:"remote"`
}

// ReadPortHint treats missing or malformed hints as an ordinary first launch.
func ReadPortHint(dataDir, preference string) PortHint {
	var hint PortHint
	if readDocument(dataDir, "ports.json", &hint) != nil || hint.Preference != preference || hint.Main < 1 || hint.Main > 65535 || hint.Remote < 0 || hint.Remote > 65535 {
		return PortHint{}
	}
	return hint
}

// SavePortHint never changes the configured preferred address.
func SavePortHint(dataDir string, hint PortHint) error {
	root, err := prepareDirectory(dataDir)
	if err != nil {
		return err
	}
	return writeDocument(root, "ports.json", hint)
}
