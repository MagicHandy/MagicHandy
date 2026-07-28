//go:build !windows

package media

import "syscall"

// availableBytes reports free space for the filesystem holding a directory.
func availableBytes(directory string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(directory, &stat); err != nil {
		return 0, err
	}
	// Bavail rather than Bfree: the reserved blocks Bfree includes are not
	// available to an unprivileged writer.
	return uint64(stat.Bavail) * uint64(stat.Bsize), nil //nolint:gosec,unconvert // widths are platform-dependent.
}
