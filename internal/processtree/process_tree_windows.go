//go:build windows

// Package processtree owns platform-specific process-group lifecycle helpers.
package processtree

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Configure prepares command so it can be stopped without leaving descendants.
func Configure(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

// Attach assigns command to a kill-on-close Windows job object.
func Attach(command *exec.Cmd) (*Handle, error) {
	if command == nil || command.Process == nil || command.Process.Pid <= 0 {
		return nil, os.ErrProcessDone
	}
	pid := int64(command.Process.Pid)
	if pid > int64(^uint32(0)) {
		return nil, fmt.Errorf("invalid Windows process ID %d", command.Process.Pid)
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	closeJob := true
	defer func() {
		if closeJob {
			_ = windows.CloseHandle(job)
		}
	}()

	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	// #nosec G103 -- Windows requires a pointer to this initialized fixed-layout
	// structure for the duration of SetInformationJobObject.
	limitsPointer := uintptr(unsafe.Pointer(&limits))
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		limitsPointer,
		uint32(unsafe.Sizeof(limits)),
	); err != nil {
		return nil, err
	}
	process, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(pid), // #nosec G115 -- range checked above.
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = windows.CloseHandle(process) }()
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		return nil, err
	}
	closeJob = false
	return newHandle(func() error { return windows.CloseHandle(job) }), nil
}

// Kill terminates command and every process it started.
func Kill(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return os.ErrProcessDone
	}
	taskkill := exec.Command("taskkill.exe", "/PID", strconv.Itoa(command.Process.Pid), "/T", "/F") // #nosec G204 -- PID is numeric and app-owned.
	taskkill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := taskkill.Run(); err == nil {
		return nil
	}
	err := command.Process.Kill()
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}
