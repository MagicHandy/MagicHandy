package updatecheck

import (
	"strconv"
	"strings"
)

type version struct {
	major uint64
	minor uint64
	patch uint64
	pre   []string
}

func parseVersion(value string) (version, bool) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "v")
	if value == "" {
		return version{}, false
	}
	if buildIndex := strings.IndexByte(value, '+'); buildIndex >= 0 {
		build := value[buildIndex+1:]
		if build == "" {
			return version{}, false
		}
		for _, identifier := range strings.Split(build, ".") {
			if !validIdentifier(identifier) {
				return version{}, false
			}
		}
		value = value[:buildIndex]
	}
	core := value
	var pre []string
	if prereleaseIndex := strings.IndexByte(value, '-'); prereleaseIndex >= 0 {
		core = value[:prereleaseIndex]
		prerelease := value[prereleaseIndex+1:]
		if prerelease == "" {
			return version{}, false
		}
		pre = strings.Split(prerelease, ".")
		for _, identifier := range pre {
			if !validPrereleaseIdentifier(identifier) {
				return version{}, false
			}
		}
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return version{}, false
	}
	numbers := [3]uint64{}
	for index, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return version{}, false
		}
		parsed, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return version{}, false
		}
		numbers[index] = parsed
	}
	return version{major: numbers[0], minor: numbers[1], patch: numbers[2], pre: pre}, true
}

func validPrereleaseIdentifier(value string) bool {
	if !validIdentifier(value) {
		return false
	}
	numeric := true
	for _, char := range value {
		if char < '0' || char > '9' {
			numeric = false
		}
	}
	return !numeric || len(value) == 1 || value[0] != '0'
}

func validIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') && char != '-' {
			return false
		}
	}
	return true
}

func (v version) String() string {
	value := strconv.FormatUint(v.major, 10) + "." + strconv.FormatUint(v.minor, 10) + "." + strconv.FormatUint(v.patch, 10)
	if len(v.pre) > 0 {
		value += "-" + strings.Join(v.pre, ".")
	}
	return value
}

func (v version) Compare(other version) int {
	for _, pair := range [][2]uint64{{v.major, other.major}, {v.minor, other.minor}, {v.patch, other.patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	if len(v.pre) == 0 && len(other.pre) == 0 {
		return 0
	}
	if len(v.pre) == 0 {
		return 1
	}
	if len(other.pre) == 0 {
		return -1
	}
	for index := 0; index < len(v.pre) && index < len(other.pre); index++ {
		comparison := comparePrereleaseIdentifier(v.pre[index], other.pre[index])
		if comparison != 0 {
			return comparison
		}
	}
	if len(v.pre) < len(other.pre) {
		return -1
	}
	if len(v.pre) > len(other.pre) {
		return 1
	}
	return 0
}

func comparePrereleaseIdentifier(left, right string) int {
	leftNumeric := numericIdentifier(left)
	rightNumeric := numericIdentifier(right)
	if leftNumeric && rightNumeric {
		if len(left) < len(right) {
			return -1
		}
		if len(left) > len(right) {
			return 1
		}
		return strings.Compare(left, right)
	}
	if leftNumeric {
		return -1
	}
	if rightNumeric {
		return 1
	}
	return strings.Compare(left, right)
}

func numericIdentifier(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return value != ""
}
