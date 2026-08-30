//go:build !windows

package httpapi

import "syscall"

func setupAvailableBytes(directory string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(directory, &stat); err != nil {
		return 0, err
	}
	return uint64(stat.Bavail) * uint64(stat.Bsize), nil //nolint:gosec,unconvert // platform-dependent syscall widths.
}
