package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func importLSOActivePersonaID(source *sql.DB) (string, error) {
	app, err := importLSOAppSettings(source)
	if err != nil || app == nil {
		return "", err
	}
	raw, _ := app["active_persona_id"].(string)
	return strings.TrimSpace(raw), nil
}

func importLSOOperationMode(source *sql.DB) string {
	app, err := importLSOAppSettings(source)
	if err != nil || app == nil {
		return ""
	}
	raw, _ := app["operation_mode"].(string)
	return normalizeOperationMode(raw)
}

func importLSOAppSettings(source *sql.DB) (map[string]interface{}, error) {
	if !lsoTableExists(source, "settings") {
		return nil, nil
	}
	var valueJSON string
	err := source.QueryRow(`SELECT value_json FROM settings WHERE key = 'app'`).Scan(&valueJSON)
	if err != nil {
		return nil, nil
	}
	var app map[string]interface{}
	if err := json.Unmarshal([]byte(valueJSON), &app); err != nil {
		return nil, nil
	}
	return app, nil
}

func copyPersonaMedia(lsoDataDir, targetDataDir string) (int, error) {
	srcRoot := filepath.Join(lsoDataDir, "personas")
	if _, err := os.Stat(srcRoot); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("stat LSO personas dir: %w", err)
	}

	copied := 0
	err := filepath.WalkDir(srcRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(targetDataDir, "personas", rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return err
		}
		if err := copyFile(path, dest); err != nil {
			return err
		}
		copied++
		return nil
	})
	if err != nil {
		return copied, fmt.Errorf("copy persona media: %w", err)
	}
	return copied, nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src) // #nosec G304 -- controlled import source path.
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) // #nosec G304 -- destination is under app data dir.
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

// DefaultLSODatabaseCandidates returns likely LSO app.sqlite locations.
func DefaultLSODatabaseCandidates() []string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	exe, err := os.Executable()
	if err != nil {
		exe = ""
	}
	roots := []string{cwd, filepath.Dir(exe)}
	var candidates []string
	seen := map[string]struct{}{}
	for _, root := range roots {
		if root == "" {
			continue
		}
		for _, rel := range []string{
			filepath.Join("..", "LSO---Local_Stroke_Orchestrator", "data", "app.sqlite"),
			filepath.Join("..", "local-stroke-orchestrator", "data", "app.sqlite"),
			filepath.Join("..", "..", "LSO---Local_Stroke_Orchestrator", "data", "app.sqlite"),
			filepath.Join("..", "..", "local-stroke-orchestrator", "data", "app.sqlite"),
		} {
			abs := filepath.Clean(filepath.Join(root, rel))
			if _, ok := seen[abs]; ok {
				continue
			}
			seen[abs] = struct{}{}
			if _, err := os.Stat(abs); err == nil {
				candidates = append(candidates, abs)
			}
		}
	}
	return candidates
}

// ResolveLSODataDir returns the LSO data directory for a given app.sqlite path.
func ResolveLSODataDir(lsoDBPath string) string {
	abs, err := filepath.Abs(lsoDBPath)
	if err != nil {
		return ""
	}
	dir := filepath.Dir(abs)
	if strings.EqualFold(filepath.Base(dir), "data") {
		return dir
	}
	return dir
}

// AutoImportLSOIfEmpty imports personas and related library rows when the
// target database has no personas yet.
func AutoImportLSOIfEmpty(target *DB) (LSOImportResult, string, error) {
	count, err := target.CountPersonas()
	if err != nil {
		return LSOImportResult{}, "", err
	}
	if count > 0 {
		return LSOImportResult{}, "", nil
	}

	lsoPath := strings.TrimSpace(os.Getenv("MAGICHANDY_LSO_DB"))
	if lsoPath == "" {
		candidates := DefaultLSODatabaseCandidates()
		if len(candidates) == 0 {
			return LSOImportResult{}, "", nil
		}
		lsoPath = candidates[0]
	}

	result, err := ImportFromLSOWithOptions(target, lsoPath, LSOImportOptions{
		LSODataDir: ResolveLSODataDir(lsoPath),
		TargetDir:  target.DataDir(),
	})
	return result, lsoPath, err
}
