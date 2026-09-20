//go:build !windows

package netaccess

import "os"

func restrictCertificatePath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return os.Chmod(path, 0700) // #nosec G302 -- directory-only branch: owner needs traversal; group/other have no access.
	}
	return os.Chmod(path, 0600)
}
