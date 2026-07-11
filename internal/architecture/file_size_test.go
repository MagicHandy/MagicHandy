package architecture

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hardCeilingLines is the only failing threshold. Files below it are governed by
// an advisory guideline (a "consider splitting" note), not a hard cap — so the
// size norm stays a maintainability guideline reviewers apply with judgment,
// rather than a rule contributors route around. See AGENTS.md.
const hardCeilingLines = 1500

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
		{
			root:       "web",
			extension:  ".ts",
			defaultMax: 800,
		},
		{
			root:       "web",
			extension:  ".tsx",
			defaultMax: 800,
		},
		{
			root:       "web",
			extension:  ".css",
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
		if entry.IsDir() {
			// Skip generated output and installed dependencies: only
			// hand-written source is size-governed (the React build output and
			// node_modules are neither authored nor shipped as source).
			switch entry.Name() {
			case "node_modules", "dist", ".vite":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != budget.extension {
			return nil
		}
		relative, err := filepath.Rel(repo, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		advisory := budget.defaultMax
		if override, ok := budget.overrides[relative]; ok {
			advisory = override
		}
		lines, err := countLines(path)
		if err != nil {
			return err
		}
		// Guideline, not a rule to game: over the advisory target we log a
		// non-failing "consider splitting" note; only files over the generous
		// hard ceiling fail CI. Reviewers use judgment in between.
		switch {
		case lines > hardCeilingLines:
			t.Errorf("%s has %d lines, over the %d hard ceiling; split it before adding more code", relative, lines, hardCeilingLines)
		case lines > advisory:
			t.Logf("advisory: %s has %d lines (guideline ~%d); consider splitting when you next touch it", relative, lines, advisory)
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
