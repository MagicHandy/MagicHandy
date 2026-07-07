package funscript

import (
	"path/filepath"
	"regexp"
	"strings"
)

var tmpStemRE = regexp.MustCompile(`(?i)^tmp[a-z0-9_-]{3,16}$`)

func slugifyLabel(value string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 32
	}
	text := strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		if r == '-' || r == ' ' || r == '_' {
			b.WriteByte('-')
		}
	}
	text = strings.Trim(b.String(), "-")
	for strings.Contains(text, "--") {
		text = strings.ReplaceAll(text, "--", "-")
	}
	if text == "" {
		return "script"
	}
	if len(text) > maxLen {
		text = strings.Trim(text[:maxLen], "-")
	}
	if text == "" {
		return "script"
	}
	return text
}

func isTempUploadStem(stem string) bool {
	s := strings.ToLower(strings.TrimSpace(stem))
	if s == "" {
		return true
	}
	return tmpStemRE.MatchString(s) || strings.HasPrefix(s, "tmp")
}

// ImportSourceStem returns the stem used for block ids.
func ImportSourceStem(filename string) string {
	stem := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	if isTempUploadStem(stem) {
		return "import"
	}
	return stem
}

// BlockRecordID builds a stable block id with source time range.
func BlockRecordID(sourceStem string, index int, zone, speed, sourceRangeSlug string) string {
	stem := ImportSourceStem(sourceStem)
	var base string
	if stem == "import" {
		base = slugifyLabel(zone+"-"+speed, 28)
	} else {
		base = slugifyLabel(stem, 28)
	}
	z := slugifyLabel(zone, 10)
	sp := slugifyLabel(speed, 10)
	blockID := sprintf("%s-%s-%s-%02d", base, z, sp, index)
	if sourceRangeSlug != "" {
		blockID = blockID + "-" + sourceRangeSlug
	}
	return blockID
}

// FullBlockRecordID builds a stable id for the full imported script block.
func FullBlockRecordID(sourceStem, fileID, sourceRangeSlug string) string {
	stem := ImportSourceStem(sourceStem)
	base := "import"
	if stem != "import" {
		base = slugifyLabel(stem, 28)
	}
	blockID := base + "-full-script-00"
	if fileID != "" {
		short := strings.ReplaceAll(fileID, "-", "")
		if len(short) > 8 {
			short = short[:8]
		}
		blockID = sprintf("%s-%s-full-script-00", base, short)
	}
	if sourceRangeSlug != "" {
		blockID = blockID + "-" + sourceRangeSlug
	}
	return blockID
}
