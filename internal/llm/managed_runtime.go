package llm

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	// ManagedLlamaVersion is the source release built and owned by MagicHandy.
	ManagedLlamaVersion = "b9966"
	// ManagedLlamaCommit pins the exact upstream source represented by ManagedLlamaVersion.
	ManagedLlamaCommit = "c749cb041706647f460bb918cccc9d91995205ab"

	managedRuntimeManifestVersion = 1
	managedRuntimeManifestLimit   = 32 * 1024
	managedRuntimeOutputLimit     = 12 * 1024
)

// Managed llama.cpp runtime states reported to the API and UI.
const (
	ManagedRuntimeStateMissing  = "missing"
	ManagedRuntimeStateReady    = "ready"
	ManagedRuntimeStateOutdated = "outdated"
	ManagedRuntimeStateInvalid  = "invalid"
)

// Managed llama.cpp source-build states reported to the API and UI.
const (
	RuntimeBuildStatusQueued    = "queued"
	RuntimeBuildStatusBuilding  = "building"
	RuntimeBuildStatusComplete  = "complete"
	RuntimeBuildStatusFailed    = "failed"
	RuntimeBuildStatusCancelled = "cancelled"
)

//go:embed runtimeassets/build-managed-llama.ps1
var managedRuntimeAssets embed.FS

// ManagedLlamaRuntimeStatus is the backend-authoritative app-owned runner state.
type ManagedLlamaRuntimeStatus struct {
	State             string   `json:"state"`
	Installed         bool     `json:"installed"`
	Current           bool     `json:"current"`
	BuildSupported    bool     `json:"build_supported"`
	SupportedBackends []string `json:"supported_backends"`
	ExpectedVersion   string   `json:"expected_version"`
	Version           string   `json:"version,omitempty"`
	Commit            string   `json:"commit,omitempty"`
	Backend           string   `json:"backend,omitempty"`
	Source            string   `json:"source,omitempty"`
	BuiltAt           string   `json:"built_at,omitempty"`
	Message           string   `json:"message"`
	RunnerPath        string   `json:"-"`
}

// ManagedLlamaRuntimeBuild is one in-process source-build job.
type ManagedLlamaRuntimeBuild struct {
	ID        string `json:"id"`
	Backend   string `json:"backend"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	Output    string `json:"output,omitempty"`
	StartedAt string `json:"started_at"`
	UpdatedAt string `json:"updated_at"`
}

// ManagedLlamaRuntimeSnapshot combines installed state with the current build.
type ManagedLlamaRuntimeSnapshot struct {
	Runtime ManagedLlamaRuntimeStatus `json:"runtime"`
	Build   *ManagedLlamaRuntimeBuild `json:"build,omitempty"`
}

type managedRuntimeManifest struct {
	SchemaVersion int    `json:"schema_version"`
	Runtime       string `json:"runtime"`
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	Backend       string `json:"backend"`
	Runner        string `json:"runner"`
	Source        string `json:"source"`
	BuiltAt       string `json:"built_at"`
}

// ManagedLlamaRuntimeRoot returns the private directory for app-owned runner builds.
func ManagedLlamaRuntimeRoot(dataDir string) string {
	return filepath.Join(dataDir, "runtimes", "llama.cpp")
}

// InspectManagedLlamaRuntime validates the active runtime manifest and runner path.
func InspectManagedLlamaRuntime(dataDir string) ManagedLlamaRuntimeStatus {
	status := ManagedLlamaRuntimeStatus{
		State:             ManagedRuntimeStateMissing,
		BuildSupported:    runtime.GOOS == "windows" && runtime.GOARCH == "amd64",
		SupportedBackends: []string{"auto", "cpu", "cuda"},
		ExpectedVersion:   ManagedLlamaVersion,
		Message:           "Managed llama.cpp is not built yet.",
	}
	root := ManagedLlamaRuntimeRoot(dataDir)
	manifestPath := filepath.Join(root, "active.json")
	file, err := os.Open(manifestPath) // #nosec G304 -- path is fixed beneath the app-owned data directory.
	if errors.Is(err, os.ErrNotExist) {
		return status
	}
	if err != nil {
		status.State = ManagedRuntimeStateInvalid
		status.Message = fmt.Sprintf("Managed llama.cpp manifest is unavailable: %v", err)
		return status
	}
	defer func() { _ = file.Close() }()

	payload, err := io.ReadAll(io.LimitReader(file, managedRuntimeManifestLimit+1))
	if err != nil || len(payload) > managedRuntimeManifestLimit {
		status.State = ManagedRuntimeStateInvalid
		status.Message = "Managed llama.cpp manifest is unreadable or too large."
		return status
	}
	payload = bytes.TrimPrefix(payload, []byte{0xEF, 0xBB, 0xBF})
	var manifest managedRuntimeManifest
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		status.State = ManagedRuntimeStateInvalid
		status.Message = fmt.Sprintf("Managed llama.cpp manifest is invalid: %v", err)
		return status
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		status.State = ManagedRuntimeStateInvalid
		status.Message = "Managed llama.cpp manifest contains trailing data."
		return status
	}
	if err := validateManagedRuntimeManifest(manifest); err != nil {
		status.State = ManagedRuntimeStateInvalid
		status.Message = err.Error()
		return status
	}

	runnerPath := filepath.Clean(filepath.Join(root, filepath.FromSlash(manifest.Runner)))
	if filepath.IsAbs(manifest.Runner) || !pathWithin(root, runnerPath) {
		status.State = ManagedRuntimeStateInvalid
		status.Message = "Managed llama.cpp runner path escapes the app-owned runtime directory."
		return status
	}
	info, err := os.Lstat(runnerPath)
	if err != nil || !info.Mode().IsRegular() {
		status.State = ManagedRuntimeStateInvalid
		status.Message = "Managed llama.cpp runner is missing from its app-owned install."
		return status
	}

	status.Installed = true
	status.Current = manifest.Version == ManagedLlamaVersion && strings.EqualFold(manifest.Commit, ManagedLlamaCommit)
	status.Version = manifest.Version
	status.Commit = manifest.Commit
	status.Backend = manifest.Backend
	status.Source = manifest.Source
	status.BuiltAt = manifest.BuiltAt
	status.RunnerPath = runnerPath
	if status.Current {
		status.State = ManagedRuntimeStateReady
		status.Message = fmt.Sprintf("Managed llama.cpp %s (%s) is installed.", manifest.Version, manifest.Backend)
	} else {
		status.State = ManagedRuntimeStateOutdated
		status.Message = fmt.Sprintf("Managed llama.cpp %s is installed; %s is the current pinned build.", manifest.Version, ManagedLlamaVersion)
	}
	return status
}

func validateManagedRuntimeManifest(manifest managedRuntimeManifest) error {
	if manifest.SchemaVersion != managedRuntimeManifestVersion || manifest.Runtime != "llama.cpp" {
		return errors.New("managed llama.cpp manifest has an unsupported schema or runtime type")
	}
	if strings.TrimSpace(manifest.Version) == "" || len(manifest.Commit) != 40 || !isLowerHex(manifest.Commit) {
		return errors.New("managed llama.cpp manifest has invalid version metadata")
	}
	if manifest.Backend != "cpu" && manifest.Backend != "cuda" {
		return errors.New("managed llama.cpp manifest has an unsupported backend")
	}
	if strings.TrimSpace(manifest.Runner) == "" || strings.TrimSpace(manifest.Source) != "built_from_source" {
		return errors.New("managed llama.cpp manifest does not describe an app-built runner")
	}
	if _, err := time.Parse(time.RFC3339Nano, manifest.BuiltAt); err != nil {
		return errors.New("managed llama.cpp manifest has an invalid build timestamp")
	}
	return nil
}

func isLowerHex(value string) bool {
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

type managedRuntimeBuildState struct {
	ManagedLlamaRuntimeBuild
	cancel context.CancelFunc
}

// ManagedLlamaRuntimeManager owns explicit source-build jobs for the managed runner.
type ManagedLlamaRuntimeManager struct {
	dataDir string
	root    string
	ctx     context.Context
	cancel  context.CancelFunc

	mu     sync.Mutex
	build  *managedRuntimeBuildState
	closed bool
	wg     sync.WaitGroup
}

// OpenManagedLlamaRuntimeManager prepares app-owned runtime storage.
func OpenManagedLlamaRuntimeManager(dataDir string) (*ManagedLlamaRuntimeManager, error) {
	root := ManagedLlamaRuntimeRoot(dataDir)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create managed llama.cpp runtime directory: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &ManagedLlamaRuntimeManager{dataDir: dataDir, root: root, ctx: ctx, cancel: cancel}, nil
}

// Snapshot returns installed runtime state and the current/recent build job.
func (m *ManagedLlamaRuntimeManager) Snapshot() ManagedLlamaRuntimeSnapshot {
	snapshot := ManagedLlamaRuntimeSnapshot{Runtime: InspectManagedLlamaRuntime(m.dataDir)}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.build != nil {
		build := m.build.ManagedLlamaRuntimeBuild
		snapshot.Build = &build
	}
	return snapshot
}

// StartBuild starts an explicit pinned source build without accepting paths or URLs.
func (m *ManagedLlamaRuntimeManager) StartBuild(backend string) (ManagedLlamaRuntimeBuild, error) {
	backend = strings.ToLower(strings.TrimSpace(backend))
	if backend == "" {
		backend = "auto"
	}
	if backend != "auto" && backend != "cpu" && backend != "cuda" {
		return ManagedLlamaRuntimeBuild{}, errors.New("managed llama.cpp backend must be auto, cpu, or cuda")
	}
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return ManagedLlamaRuntimeBuild{}, errors.New("managed llama.cpp source builds are currently supported on Windows/amd64 only")
	}
	id, err := randomImportID()
	if err != nil {
		return ManagedLlamaRuntimeBuild{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ManagedLlamaRuntimeBuild{}, errors.New("managed llama.cpp runtime manager is closed")
	}
	if m.build != nil && (m.build.Status == RuntimeBuildStatusQueued || m.build.Status == RuntimeBuildStatusBuilding) {
		return ManagedLlamaRuntimeBuild{}, errors.New("a managed llama.cpp build is already running")
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.build = &managedRuntimeBuildState{
		ManagedLlamaRuntimeBuild: ManagedLlamaRuntimeBuild{
			ID: id, Backend: backend, Status: RuntimeBuildStatusQueued,
			Message: "Queued managed llama.cpp source build.", StartedAt: now, UpdatedAt: now,
		},
		cancel: cancel,
	}
	result := m.build.ManagedLlamaRuntimeBuild
	m.wg.Add(1)
	go m.runBuild(ctx, id)
	return result, nil
}

// CancelBuild cancels the active source build and its process tree.
func (m *ManagedLlamaRuntimeManager) CancelBuild() (ManagedLlamaRuntimeBuild, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.build == nil || (m.build.Status != RuntimeBuildStatusQueued && m.build.Status != RuntimeBuildStatusBuilding) {
		return ManagedLlamaRuntimeBuild{}, errors.New("no managed llama.cpp build is running")
	}
	m.build.cancel()
	return m.build.ManagedLlamaRuntimeBuild, nil
}

// Close cancels and waits for a source build before app shutdown completes.
func (m *ManagedLlamaRuntimeManager) Close() {
	m.mu.Lock()
	if !m.closed {
		m.closed = true
		m.cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
}

func (m *ManagedLlamaRuntimeManager) runBuild(ctx context.Context, id string) {
	defer m.wg.Done()
	m.updateBuild(id, RuntimeBuildStatusBuilding, "Building managed llama.cpp from pinned source.", "")

	scriptPath, err := m.installBuildScript()
	if err != nil {
		m.finishBuild(ctx, id, err)
		return
	}
	powerShell, err := findPowerShell()
	if err != nil {
		m.finishBuild(ctx, id, err)
		return
	}
	backend := m.buildBackend(id)
	// #nosec G204 -- executable is a discovered PowerShell binary; script bytes are embedded and all arguments are app-controlled.
	command := exec.CommandContext(ctx, powerShell,
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-File", scriptPath, "-DataDir", m.dataDir, "-Backend", backend,
	)
	command.Cancel = func() error { return cancelManagedBuildProcess(command) }
	command.WaitDelay = 10 * time.Second
	writer := &managedRuntimeBuildWriter{manager: m, id: id}
	command.Stdout = writer
	command.Stderr = writer
	err = command.Run()
	if err == nil {
		status := InspectManagedLlamaRuntime(m.dataDir)
		if !status.Installed || !status.Current {
			err = errors.New("source build finished without installing the pinned managed runtime")
		}
	}
	m.finishBuild(ctx, id, err)
}

func (m *ManagedLlamaRuntimeManager) finishBuild(ctx context.Context, id string, err error) {
	switch {
	case ctx.Err() != nil:
		m.updateBuild(id, RuntimeBuildStatusCancelled, "Managed llama.cpp build cancelled.", "")
	case err != nil:
		m.updateBuild(id, RuntimeBuildStatusFailed, fmt.Sprintf("Managed llama.cpp build failed: %v", err), "")
	default:
		status := InspectManagedLlamaRuntime(m.dataDir)
		m.updateBuild(id, RuntimeBuildStatusComplete, status.Message, "")
	}
}

func (m *ManagedLlamaRuntimeManager) installBuildScript() (string, error) {
	payload, err := managedRuntimeAssets.ReadFile("runtimeassets/build-managed-llama.ps1")
	if err != nil {
		return "", fmt.Errorf("read embedded llama.cpp build helper: %w", err)
	}
	directory := filepath.Join(m.root, ".tools")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", fmt.Errorf("create llama.cpp build tools directory: %w", err)
	}
	path := filepath.Join(directory, "build-managed-llama.ps1")
	if existing, readErr := os.ReadFile(path); readErr == nil && bytes.Equal(existing, payload) { // #nosec G304 -- fixed app-owned path.
		return path, nil
	}
	partial := path + ".partial"
	if err := os.WriteFile(partial, payload, 0o600); err != nil {
		return "", fmt.Errorf("write llama.cpp build helper: %w", err)
	}
	_ = os.Remove(path)
	if err := os.Rename(partial, path); err != nil {
		_ = os.Remove(partial)
		return "", fmt.Errorf("install llama.cpp build helper: %w", err)
	}
	return path, nil
}

func findPowerShell() (string, error) {
	for _, name := range []string{"powershell.exe", "pwsh.exe", "pwsh"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", errors.New("PowerShell is required to build managed llama.cpp")
}

func (m *ManagedLlamaRuntimeManager) buildBackend(id string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.build == nil || m.build.ID != id {
		return "auto"
	}
	return m.build.Backend
}

func (m *ManagedLlamaRuntimeManager) updateBuild(id, status, message, output string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.build == nil || m.build.ID != id {
		return
	}
	m.build.Status = status
	if message != "" {
		m.build.Message = message
	}
	if output != "" {
		m.build.Output = trimRuntimeBuildOutput(m.build.Output + output)
		if line := lastNonBlankLine(output); line != "" {
			m.build.Message = line
		}
	}
	m.build.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
}

func trimRuntimeBuildOutput(output string) string {
	if len(output) <= managedRuntimeOutputLimit {
		return output
	}
	start := len(output) - managedRuntimeOutputLimit
	for start < len(output) && !utf8.RuneStart(output[start]) {
		start++
	}
	return output[start:]
}

func lastNonBlankLine(output string) string {
	lines := strings.Split(strings.ReplaceAll(output, "\r", ""), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		if line := strings.TrimSpace(lines[index]); line != "" {
			runes := []rune(line)
			if len(runes) > 240 {
				return string(runes[:240])
			}
			return line
		}
	}
	return ""
}

type managedRuntimeBuildWriter struct {
	manager *ManagedLlamaRuntimeManager
	id      string
}

func (w *managedRuntimeBuildWriter) Write(data []byte) (int, error) {
	w.manager.updateBuild(w.id, RuntimeBuildStatusBuilding, "", string(data))
	return len(data), nil
}
