package architecture

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceFileLineBudgets(t *testing.T) {
	root := repoRoot(t)
	budgets := []sourceBudget{
		{
			root:       "cmd",
			extension:  ".go",
			defaultMax: 800,
		},
		{
			root:       "internal",
			extension:  ".go",
			defaultMax: 800,
		},
		{
			root:       "web",
			extension:  ".js",
			defaultMax: 800,
		},
	}

	for _, budget := range budgets {
		checkSourceBudget(t, root, budget)
	}
}

type sourceBudget struct {
	root       string
	extension  string
	defaultMax int
	overrides  map[string]int
}

func checkSourceBudget(t *testing.T, repo string, budget sourceBudget) {
	t.Helper()

	start := filepath.Join(repo, budget.root)
	err := filepath.WalkDir(start, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != budget.extension {
			return nil
		}
		relative, err := filepath.Rel(repo, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		maxLines := budget.defaultMax
		if override, ok := budget.overrides[relative]; ok {
			maxLines = override
		}
		lines, err := countLines(path)
		if err != nil {
			return err
		}
		if lines > maxLines {
			t.Errorf("%s has %d lines, budget is %d; split the file before adding more code", relative, lines, maxLines)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s files under %s: %v", strings.TrimPrefix(budget.extension, "."), budget.root, err)
	}
}

func countLines(path string) (lines int, err error) {
	file, err := os.Open(path) // #nosec G304 -- tests only read repository source files.
	if err != nil {
		return 0, err
	}
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines++
	}
	return lines, scanner.Err()
}
