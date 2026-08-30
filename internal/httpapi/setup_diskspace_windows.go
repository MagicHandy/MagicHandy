//go:build windows

package httpapi

import (
	"errors"
	"syscall"
	"unsafe"
)

func setupAvailableBytes(directory string) (uint64, error) {
	path, err := syscall.UTF16PtrFromString(directory)
	if err != nil {
		return 0, err
	}
	getDiskFreeSpaceEx := syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")
	var freeToCaller, totalBytes, totalFree uint64
	result, _, callErr := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(path)),          // #nosec G103 -- Win32 input pointer.
		uintptr(unsafe.Pointer(&freeToCaller)), // #nosec G103 -- Win32 out-parameter.
		uintptr(unsafe.Pointer(&totalBytes)),   // #nosec G103 -- Win32 out-parameter.
		uintptr(unsafe.Pointer(&totalFree)),    // #nosec G103 -- Win32 out-parameter.
	)
	if result == 0 {
		if callErr != nil {
			return 0, callErr
		}
		return 0, errors.New("could not read free disk space")
	}
	return freeToCaller, nil
}
