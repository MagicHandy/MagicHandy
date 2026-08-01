//go:build windows

package llm

import (
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type managedLlamaProcessLifetime interface {
	Close() error
}

type managedLlamaWindowsJob struct {
	handle windows.Handle
}

func configureManagedLlamaProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

func attachManagedLlamaProcess(command *exec.Cmd) (managedLlamaProcessLifetime, error) {
	pid, err := checkedWindowsProcessID(command.Process.Pid)
	if err != nil {
		return nil, err
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
	// #nosec G103 -- SetInformationJobObject requires a pointer to this
	// initialized fixed-layout Windows structure for the duration of the call.
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
		pid,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = windows.CloseHandle(process) }()
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		return nil, err
	}
	closeJob = false
	return &managedLlamaWindowsJob{handle: job}, nil
}

func (j *managedLlamaWindowsJob) Close() error {
	if j == nil || j.handle == 0 {
		return nil
	}
	err := windows.CloseHandle(j.handle)
	j.handle = 0
	return err
}
