package httpapi

import (
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var (
	setupANSI          = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	setupSensitiveLine = regexp.MustCompile(`(?im)^.*(?:authorization|cookie|password|passwd|api[-_ ]?key|access[-_ ]?token|hf_token|secret|connection[-_ ]?key|credential)[\w '"-]*[:=].*$`)
	setupBearer        = regexp.MustCompile(`(?i)\b(?:bearer|basic)\s+[^\s'"<>]+`)
	setupToken         = regexp.MustCompile(`\b(?:hf_|sk-)[A-Za-z0-9_-]{8,}\b`)
	setupURL           = regexp.MustCompile(`(?i)\b(?:https?|ftp|file)://[^\s'"<>]+`)
	// PowerShell wraps quoted paths/URLs across terminal lines; remove the
	// entire quoted location before handling ordinary single-line locations.
	setupQuotedLocation = regexp.MustCompile(`(?i)"(?:[a-z]:[\\/]|\\\\|(?:https?|file|ftp)://|/(?:home|Users|tmp|private|var|opt|usr|mnt|media|root)/)[^"]*"|'(?:[a-z]:[\\/]|\\\\|(?:https?|file|ftp)://|/(?:home|Users|tmp|private|var|opt|usr|mnt|media|root)/)[^']*'`)
	setupWindowsPath    = regexp.MustCompile(`(?i)(?:\b[a-z]:[\\/]|\\\\)[^\r\n"<>|:]*`)
	setupUnixPath       = regexp.MustCompile(`(?:^|[\s'"=:(])/(?:home|Users|tmp|private|var|opt|usr|mnt|media|root)/[^\r\n"<>|:]*`)
	setupSecretEnv      = regexp.MustCompile(`(?i)(?:^|_)(?:token|password|passwd|secret|key|credential)(?:$|_)`)
)

func (m *setupManager) setupReportRedactor() func(string) string {
	var secrets []string
	if m.reportSecrets != nil {
		secrets = append(secrets, m.reportSecrets()...)
	}
	// Inspect only secret-shaped names for replacement; never serialize the
	// process environment or credential values into the report.
	for _, entry := range os.Environ() {
		name, value, _ := strings.Cut(entry, "=")
		if setupSecretEnv.MatchString(name) {
			secrets = append(secrets, value)
		}
	}
	var replacements []string
	var wrappedSecrets []*regexp.Regexp
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		for _, value := range []string{secret, url.QueryEscape(secret), url.PathEscape(secret)} {
			replacements = append(replacements, value, "[redacted]")
			if len(value) >= 4 && len(value) <= 2048 {
				characters := strings.Split(value, "")
				for index := range characters {
					characters[index] = regexp.QuoteMeta(characters[index])
				}
				wrappedSecrets = append(wrappedSecrets, regexp.MustCompile(strings.Join(characters, `(?:\n[\t ]*)?`)))
			}
		}
	}
	replace := strings.NewReplacer(replacements...)
	return func(value string) string {
		value = strings.ReplaceAll(value, "\r\n", "\n")
		value = setupANSI.ReplaceAllString(value, "")
		value = strings.Map(func(character rune) rune {
			if unicode.IsControl(character) && character != '\n' && character != '\t' {
				return -1
			}
			return character
		}, strings.ToValidUTF8(value, "�"))
		if strings.Contains(value, "\n") {
			for _, wrapped := range wrappedSecrets {
				value = wrapped.ReplaceAllString(value, "[redacted]")
			}
		}
		value = replace.Replace(value)
		value = setupSensitiveLine.ReplaceAllString(value, "[redacted sensitive line]")
		value = setupBearer.ReplaceAllString(value, "[redacted authorization]")
		value = setupToken.ReplaceAllString(value, "[redacted token]")
		value = setupQuotedLocation.ReplaceAllString(value, "[redacted location]")
		value = setupURL.ReplaceAllString(value, "[redacted URL]")
		value = setupWindowsPath.ReplaceAllString(value, "[local path]")
		return setupUnixPath.ReplaceAllString(value, " [local path]")
	}
}
