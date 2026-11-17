//go:build magichandy_labs

// Package reviewfiles opens explicitly selected local reports for optional review tools.
package reviewfiles

import (
	"os"
	"path/filepath"
)

// Read loads a report inside its selected directory.
func Read(path string) ([]byte, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return root.ReadFile(filepath.Base(path))
}

// Write saves a private report inside its selected directory.
func Write(path string, data []byte) error {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	return root.WriteFile(filepath.Base(path), data, 0600)
}
