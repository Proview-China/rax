package core

import (
	"strconv"
	"strings"
)

const MaxSemanticVersionLength = 128

type SemanticVersion struct {
	Major      uint64   `json:"major"`
	Minor      uint64   `json:"minor"`
	Patch      uint64   `json:"patch"`
	Prerelease []string `json:"prerelease"`
	Build      []string `json:"build"`
}

// ParseSemanticVersion implements strict SemVer 2.0.0 parsing. Build metadata
// is preserved and participates in binding identity, while range precedence
// deliberately ignores it as required by SemVer.
func ParseSemanticVersion(value string) (SemanticVersion, error) {
	if value == "" || len(value) > MaxSemanticVersionLength || strings.TrimSpace(value) != value {
		return SemanticVersion{}, NewError(ErrorInvalidArgument, ReasonInvalidSemanticVersion, "semantic version is empty, padded or too long")
	}
	if strings.Count(value, "+") > 1 {
		return SemanticVersion{}, NewError(ErrorInvalidArgument, ReasonInvalidSemanticVersion, "semantic version contains multiple build separators")
	}
	versionAndBuild := strings.SplitN(value, "+", 2)
	versionAndPrerelease := strings.SplitN(versionAndBuild[0], "-", 2)
	coreParts := strings.Split(versionAndPrerelease[0], ".")
	if len(coreParts) != 3 {
		return SemanticVersion{}, NewError(ErrorInvalidArgument, ReasonInvalidSemanticVersion, "semantic version requires major.minor.patch")
	}
	parsed := SemanticVersion{}
	values := []*uint64{&parsed.Major, &parsed.Minor, &parsed.Patch}
	for index, part := range coreParts {
		if !validNumericIdentifier(part) {
			return SemanticVersion{}, NewError(ErrorInvalidArgument, ReasonInvalidSemanticVersion, "numeric semantic version identifier is invalid")
		}
		number, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return SemanticVersion{}, NewError(ErrorInvalidArgument, ReasonInvalidSemanticVersion, "semantic version numeric identifier overflows")
		}
		*values[index] = number
	}
	if len(versionAndPrerelease) == 2 {
		parsed.Prerelease = strings.Split(versionAndPrerelease[1], ".")
		if !validSemanticIdentifiers(parsed.Prerelease, true) {
			return SemanticVersion{}, NewError(ErrorInvalidArgument, ReasonInvalidSemanticVersion, "semantic version prerelease identifiers are invalid")
		}
	}
	if len(versionAndBuild) == 2 {
		parsed.Build = strings.Split(versionAndBuild[1], ".")
		if !validSemanticIdentifiers(parsed.Build, false) {
			return SemanticVersion{}, NewError(ErrorInvalidArgument, ReasonInvalidSemanticVersion, "semantic version build identifiers are invalid")
		}
	}
	return parsed, nil
}

func (v SemanticVersion) String() string {
	value := strconv.FormatUint(v.Major, 10) + "." + strconv.FormatUint(v.Minor, 10) + "." + strconv.FormatUint(v.Patch, 10)
	if len(v.Prerelease) != 0 {
		value += "-" + strings.Join(v.Prerelease, ".")
	}
	if len(v.Build) != 0 {
		value += "+" + strings.Join(v.Build, ".")
	}
	return value
}

func CompareSemanticVersion(left, right SemanticVersion) int {
	for _, pair := range [][2]uint64{{left.Major, right.Major}, {left.Minor, right.Minor}, {left.Patch, right.Patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	if len(left.Prerelease) == 0 && len(right.Prerelease) == 0 {
		return 0
	}
	if len(left.Prerelease) == 0 {
		return 1
	}
	if len(right.Prerelease) == 0 {
		return -1
	}
	for index := 0; index < len(left.Prerelease) && index < len(right.Prerelease); index++ {
		comparison := compareSemanticIdentifier(left.Prerelease[index], right.Prerelease[index])
		if comparison != 0 {
			return comparison
		}
	}
	if len(left.Prerelease) < len(right.Prerelease) {
		return -1
	}
	if len(left.Prerelease) > len(right.Prerelease) {
		return 1
	}
	return 0
}

func validNumericIdentifier(value string) bool {
	if value == "" || (len(value) > 1 && value[0] == '0') {
		return false
	}
	for _, character := range []byte(value) {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func validSemanticIdentifiers(values []string, rejectNumericLeadingZero bool) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if value == "" {
			return false
		}
		numeric := true
		for _, character := range []byte(value) {
			if !((character >= '0' && character <= '9') || (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || character == '-') {
				return false
			}
			if character < '0' || character > '9' {
				numeric = false
			}
		}
		if rejectNumericLeadingZero && numeric && len(value) > 1 && value[0] == '0' {
			return false
		}
	}
	return true
}

func compareSemanticIdentifier(left, right string) int {
	leftNumeric := validNumericIdentifier(left)
	rightNumeric := validNumericIdentifier(right)
	if leftNumeric && rightNumeric {
		if len(left) < len(right) {
			return -1
		}
		if len(left) > len(right) {
			return 1
		}
		if left < right {
			return -1
		}
		if left > right {
			return 1
		}
		return 0
	}
	if leftNumeric != rightNumeric {
		if leftNumeric {
			return -1
		}
		return 1
	}
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}
