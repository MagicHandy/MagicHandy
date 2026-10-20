// Package startup owns local launch coordination and bounded, credential-free
// startup diagnostics. It does not depend on the datastore, HTTP API or motion.
package startup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const maxDocumentBytes = 16 << 10

func directory(dataDir string) string { return filepath.Join(dataDir, "startup") }

func prepareDirectory(dataDir string) (string, error) {
	root := directory(dataDir)
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("startup storage must be a real directory")
	}
	if err := restrictPath(root); err != nil {
		return "", fmt.Errorf("startup storage requires private permissions; choose a user-owned local -data-dir: %w", err)
	}
	return root, nil
}

func writeDocument(root, name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > maxDocumentBytes {
		return errors.New("startup document exceeds its size limit")
	}
	file, err := os.CreateTemp(root, ".startup-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if err = restrictPath(file.Name()); err == nil {
		_, err = file.Write(append(data, '\n'))
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return replaceDocument(file.Name(), filepath.Join(root, name))
}

func readDocument(dataDir, name string, value any) error {
	path := filepath.Join(directory(dataDir), name)
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxDocumentBytes {
		return errors.New("invalid startup document")
	}
	file, err := os.Open(path) // #nosec G304 -- fixed internal filename under the selected profile.
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(io.LimitReader(file, maxDocumentBytes+1))
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("invalid trailing startup data")
	}
	return nil
}
