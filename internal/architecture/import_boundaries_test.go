package architecture

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const modulePath = "github.com/mapledaemon/MagicHandy"

type goPackage struct {
	ImportPath   string
	Imports      []string
	TestImports  []string
	XTestImports []string
}

func TestInternalImportBoundaries(t *testing.T) {
	packages := listInternalPackages(t)
	internal := modulePath + "/internal/"

	rules := []struct {
		name      string
		appliesTo func(string) bool
		forbidden []string
	}{
		{
			name: "httpapi is an edge adapter",
			appliesTo: func(importPath string) bool {
				return !strings.HasPrefix(importPath, internal+"httpapi")
			},
			forbidden: []string{internal + "httpapi"},
		},
		{
			name: "semantic clients do not import transport",
			appliesTo: func(importPath string) bool {
				return hasAnyPrefix(importPath, internal+"chat", internal+"llm", internal+"modes")
			},
			forbidden: []string{internal + "transport"},
		},
		{
			name: "motion stays above HTTP, chat, LLM, modes, and transport adapters",
			appliesTo: func(importPath string) bool {
				return strings.HasPrefix(importPath, internal+"motion")
			},
			forbidden: []string{
				internal + "chat",
				internal + "httpapi",
				internal + "llm",
				internal + "modes",
				internal + "transport",
			},
		},
		{
			name: "transport does not depend on planners or edge adapters",
			appliesTo: func(importPath string) bool {
				return strings.HasPrefix(importPath, internal+"transport")
			},
			forbidden: []string{
				internal + "chat",
				internal + "httpapi",
				internal + "llm",
				internal + "modes",
				internal + "motion",
			},
		},
	}

	for _, pkg := range packages {
		imports := uniqueImports(pkg)
		for _, rule := range rules {
			if !rule.appliesTo(pkg.ImportPath) {
				continue
			}
			for _, imported := range imports {
				for _, forbidden := range rule.forbidden {
					if imported == forbidden || strings.HasPrefix(imported, forbidden+"/") {
						t.Errorf("%s: package %s imports forbidden package %s", rule.name, pkg.ImportPath, imported)
					}
				}
			}
		}
	}
}

func listInternalPackages(t *testing.T) []goPackage {
	t.Helper()

	cmd := exec.Command("go", "list", "-json", "./internal/...")
	cmd.Dir = repoRoot(t)

	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			t.Fatalf("go list failed: %v\n%s", err, exitErr.Stderr)
		}
		t.Fatalf("go list failed: %v", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(output))
	var packages []goPackage
	for {
		var pkg goPackage
		if err := decoder.Decode(&pkg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("decode go list output: %v", err)
		}
		packages = append(packages, pkg)
	}
	return packages
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller is unavailable")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func uniqueImports(pkg goPackage) []string {
	seen := make(map[string]struct{})
	for _, imported := range append(append(pkg.Imports, pkg.TestImports...), pkg.XTestImports...) {
		seen[imported] = struct{}{}
	}

	imports := make([]string, 0, len(seen))
	for imported := range seen {
		imports = append(imports, imported)
	}
	return imports
}

func hasAnyPrefix(value string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}
