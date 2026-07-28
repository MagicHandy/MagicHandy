//go:build windows

package media

import (
	"errors"
	"syscall"
	"unsafe"
)

// availableBytes reports free space for the volume holding a directory. Called
// before a conversion starts so a two-hour encode fails immediately rather than
// at ninety percent.
func availableBytes(directory string) (uint64, error) {
	path, err := syscall.UTF16PtrFromString(directory)
	if err != nil {
		return 0, err
	}
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")
	var freeToCaller, totalBytes, totalFree uint64
	// The Win32 call writes through three out-parameters, so unsafe.Pointer is
	// the only way to reach it. All four operands are stack locals of this
	// function with lifetimes that span the call.
	result, _, callErr := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(path)),          // #nosec G103 -- out-parameter for a Win32 call.
		uintptr(unsafe.Pointer(&freeToCaller)), // #nosec G103 -- out-parameter for a Win32 call.
		uintptr(unsafe.Pointer(&totalBytes)),   // #nosec G103 -- out-parameter for a Win32 call.
		uintptr(unsafe.Pointer(&totalFree)),    // #nosec G103 -- out-parameter for a Win32 call.
	)
	if result == 0 {
		if callErr != nil {
			return 0, callErr
		}
		return 0, errors.New("could not read free disk space")
	}
	// freeToCaller respects a per-user quota where one is configured, which is
	// the number that actually limits this write.
	return freeToCaller, nil
}
