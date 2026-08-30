package architecture

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const (
	conciseReleaseNotesStart = 39
	conciseReleaseMaxLines   = 60
	conciseReleaseMaxWords   = 350
	conciseReleaseMaxBullets = 12
)

var releaseNoteName = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)(?:-(alpha|beta|rc)\.(\d+))?\.md$`)

func TestReleaseNotesStayConcise(t *testing.T) {
	root := filepath.Join(repoRoot(t), "docs", "releases")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !releaseUsesConciseNotes(entry.Name()) {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			checkConciseReleaseNotes(t, filepath.Join(root, entry.Name()), strings.TrimSuffix(entry.Name(), ".md"))
		})
	}
}

func releaseUsesConciseNotes(name string) bool {
	match := releaseNoteName.FindStringSubmatch(name)
	if match == nil {
		return false
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	ordinal, _ := strconv.Atoi(match[5])
	return major != 0 || minor != 1 || patch != 0 || match[4] != "alpha" || ordinal >= conciseReleaseNotesStart
}

func checkConciseReleaseNotes(t *testing.T, path, version string) {
	t.Helper()
	file, err := os.Open(path) // #nosec G304 -- test reads a repository-owned path.
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	lineCount := 0
	wordCount := 0
	bulletCount := 0
	sectionCount := 0
	inBullet := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lineCount++
		line := scanner.Text()
		wordCount += len(strings.Fields(line))
		switch {
		case lineCount == 1:
			want := "# MagicHandy " + version
			if line != want {
				t.Errorf("title = %q, want %q", line, want)
			}
		case line == "":
			inBullet = false
		case line == "## Changelog":
			sectionCount++
			inBullet = false
		case strings.HasPrefix(line, "- "):
			bulletCount++
			inBullet = true
		case inBullet && strings.HasPrefix(line, "  "):
		default:
			t.Errorf("line %d is outside the single bullet changelog: %q", lineCount, line)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if sectionCount != 1 {
		t.Errorf("Changelog sections = %d, want exactly 1", sectionCount)
	}
	if bulletCount < 1 || bulletCount > conciseReleaseMaxBullets {
		t.Errorf("top-level bullets = %d, want 1-%d", bulletCount, conciseReleaseMaxBullets)
	}
	if lineCount > conciseReleaseMaxLines {
		t.Errorf("lines = %d, limit %d", lineCount, conciseReleaseMaxLines)
	}
	if wordCount > conciseReleaseMaxWords {
		t.Errorf("words = %d, limit %d", wordCount, conciseReleaseMaxWords)
	}
	if t.Failed() {
		t.Log("keep public notes concise; put implementation, diagnosis, validation, and measurements in the release PR")
	}
}
