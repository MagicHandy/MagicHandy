//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const (
	installerShutdownPoll             = 100 * time.Millisecond
	installerShutdownPollMilliseconds = uint32(100)
)

func installerShutdownEventName() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", fmt.Errorf("resolve absolute executable path: %w", err)
	}
	identity := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(executable))))
	return `Local\MagicHandy-InstallerShutdown-` + hex.EncodeToString(identity[:12]), nil
}

func listenForInstallerShutdown() (<-chan struct{}, func(), error) {
	name, err := installerShutdownEventName()
	if err != nil {
		return nil, nil, err
	}
	nameUTF16, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, nil, fmt.Errorf("encode shutdown event name: %w", err)
	}
	handle, err := windows.CreateEvent(nil, 1, 0, nameUTF16)
	if err != nil {
		return nil, nil, fmt.Errorf("create shutdown event: %w", err)
	}

	requests := make(chan struct{})
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			result, waitErr := windows.WaitForSingleObject(handle, installerShutdownPollMilliseconds)
			if waitErr != nil {
				return
			}
			if result == windows.WAIT_OBJECT_0 {
				close(requests)
				return
			}
			select {
			case <-stop:
				return
			default:
			}
		}
	}()

	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			close(stop)
			<-done
			_ = windows.CloseHandle(handle)
		})
	}
	return requests, cleanup, nil
}

func requestInstallerShutdown(timeout time.Duration) (bool, error) {
	name, err := installerShutdownEventName()
	if err != nil {
		return false, err
	}
	nameUTF16, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false, fmt.Errorf("encode shutdown event name: %w", err)
	}
	handle, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, nameUTF16)
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open shutdown event: %w", err)
	}
	if err := windows.SetEvent(handle); err != nil {
		_ = windows.CloseHandle(handle)
		return false, fmt.Errorf("signal shutdown event: %w", err)
	}
	_ = windows.CloseHandle(handle)

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		probe, probeErr := windows.OpenEvent(windows.SYNCHRONIZE, false, nameUTF16)
		if errors.Is(probeErr, syscall.ERROR_FILE_NOT_FOUND) {
			return true, nil
		}
		if probeErr != nil {
			return true, fmt.Errorf("check shutdown completion: %w", probeErr)
		}
		_ = windows.CloseHandle(probe)
		time.Sleep(installerShutdownPoll)
	}
	return true, fmt.Errorf("running instance did not stop within %s", timeout)
}
