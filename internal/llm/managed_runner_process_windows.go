//go:build windows

package llm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/windows"
)

const managedRunnerProcessPathCapacity = 32768

// FindManagedRunnerProcesses returns only processes whose executable path is an
// exact match for runnerPath. Name-only matches are intentionally ignored.
func FindManagedRunnerProcesses(runnerPath string, excludePID int) ([]ManagedRunnerProcess, error) {
	target, err := normalizeManagedRunnerPath(runnerPath)
	if err != nil {
		return nil, err
	}
	ids, err := windowsProcessIDs()
	if err != nil {
		return nil, fmt.Errorf("enumerate Windows processes: %w", err)
	}

	processes := make([]ManagedRunnerProcess, 0)
	for _, id := range ids {
		pid := int(id)
		if pid == 0 || pid == os.Getpid() || pid == excludePID {
			continue
		}
		path, pathErr := windowsProcessExecutable(id, windows.PROCESS_QUERY_LIMITED_INFORMATION)
		if pathErr != nil {
			continue
		}
		normalized, pathErr := normalizeManagedRunnerPath(path)
		if pathErr != nil || !strings.EqualFold(normalized, target) {
			continue
		}
		processes = append(processes, ManagedRunnerProcess{
			PID:        pid,
			Executable: filepath.Base(path),
		})
	}
	sort.Slice(processes, func(i, j int) bool { return processes[i].PID < processes[j].PID })
	return processes, nil
}

// TerminateManagedRunnerProcess terminates pid only after re-verifying that it
// still runs the exact configured managed executable.
func TerminateManagedRunnerProcess(runnerPath string, pid int) error {
	if pid <= 0 || pid == os.Getpid() {
		return errors.New("invalid managed runner process ID")
	}
	target, err := normalizeManagedRunnerPath(runnerPath)
	if err != nil {
		return err
	}
	windowsPID, err := checkedWindowsProcessID(pid)
	if err != nil {
		return err
	}
	handle, err := windows.OpenProcess(
		windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE,
		false,
		windowsPID,
	)
	if err != nil {
		return fmt.Errorf("open managed runner process %d: %w", pid, err)
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	path, err := windowsProcessExecutableFromHandle(handle)
	if err != nil {
		return fmt.Errorf("verify managed runner process %d: %w", pid, err)
	}
	normalized, err := normalizeManagedRunnerPath(path)
	if err != nil || !strings.EqualFold(normalized, target) {
		return fmt.Errorf("process %d no longer matches the configured managed llama.cpp executable", pid)
	}
	if err := windows.TerminateProcess(handle, 1); err != nil {
		return fmt.Errorf("terminate managed runner process %d: %w", pid, err)
	}
	event, err := windows.WaitForSingleObject(handle, 5000)
	if err != nil {
		return fmt.Errorf("wait for managed runner process %d: %w", pid, err)
	}
	if event != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("managed runner process %d did not exit within five seconds", pid)
	}
	return nil
}

func windowsProcessIDs() ([]uint32, error) {
	capacity := 256
	for {
		ids := make([]uint32, capacity)
		var bytesReturned uint32
		if err := windows.EnumProcesses(ids, &bytesReturned); err != nil {
			return nil, err
		}
		count := int(bytesReturned) / 4
		if count < len(ids) {
			return ids[:count], nil
		}
		capacity *= 2
	}
}

func windowsProcessExecutable(pid uint32, access uint32) (string, error) {
	handle, err := windows.OpenProcess(access, false, pid)
	if err != nil {
		return "", err
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	return windowsProcessExecutableFromHandle(handle)
}

func windowsProcessExecutableFromHandle(handle windows.Handle) (string, error) {
	buffer := make([]uint16, managedRunnerProcessPathCapacity)
	size := uint32(managedRunnerProcessPathCapacity)
	if err := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buffer[:size]), nil
}

func checkedWindowsProcessID(pid int) (uint32, error) {
	const maxWindowsProcessID = int64(1<<32 - 1)
	value := int64(pid)
	if value <= 0 || value > maxWindowsProcessID {
		return 0, fmt.Errorf("invalid Windows process ID %d", pid)
	}
	// #nosec G115 -- the explicit range check above proves this conversion.
	return uint32(value), nil
}

func normalizeManagedRunnerPath(path string) (string, error) {
	path = strings.TrimSpace(strings.TrimPrefix(path, `\\?\`))
	if path == "" {
		return "", errors.New("managed llama.cpp executable path is empty")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}
