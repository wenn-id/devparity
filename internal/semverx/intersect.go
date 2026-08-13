package semverx

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	semver "github.com/Masterminds/semver/v3"
)

var prereleasePattern = regexp.MustCompile(`\d+(?:\.[0-9A-Za-zxX*]+){0,2}-[0-9A-Za-z]`)
var versionPattern = regexp.MustCompile(`(?:\d+)(?:\.(?:\d+|[xX*])){0,2}`)
var bareMajorPattern = regexp.MustCompile(`^\d+$`)

// Normalize makes the Node constraint spellings used by beta stable enough
// for semver.NewConstraint and deterministic reports.
func Normalize(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty version constraint")
	}
	if prereleasePattern.MatchString(raw) {
		return "", fmt.Errorf("prerelease constraints are unsupported")
	}

	parts := strings.Fields(raw)
	for i, part := range parts {
		if part == "||" || part == "-" {
			continue
		}
		part = stripLeadingVAfterOperator(part)
		if bareMajorPattern.MatchString(part) || isComparatorWithBareMajor(part) {
			part += ".x"
		}
		parts[i] = part
	}
	normalized := strings.Join(parts, " ")
	if _, err := semver.NewConstraint(normalized); err != nil {
		return "", err
	}
	return normalized, nil
}

func IntersectsAll(raw []string) (bool, error) {
	if len(raw) == 0 {
		return false, fmt.Errorf("no constraints")
	}
	normalized := make([]string, len(raw))
	for i, value := range raw {
		var err error
		normalized[i], err = Normalize(value)
		if err != nil {
			return false, err
		}
	}
	constraints := make([]*semver.Constraints, len(normalized))
	for i, value := range normalized {
		var err error
		constraints[i], err = semver.NewConstraint(value)
		if err != nil {
			return false, err
		}
	}
	versions, err := candidates(normalized)
	if err != nil {
		return false, err
	}
	for _, version := range versions {
		ok := true
		for _, constraint := range constraints {
			if !constraint.Check(version) {
				ok = false
				break
			}
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func candidates(raw []string) ([]*semver.Version, error) {
	seen := make(map[string]struct{})
	var versions []*semver.Version
	add := func(major, minor, patch uint64) {
		version := semver.New(major, minor, patch, "", "")
		key := version.String()
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		versions = append(versions, version)
	}
	add(0, 0, 0)

	for _, constraint := range raw {
		for _, match := range versionPattern.FindAllString(constraint, -1) {
			major, minor, patch, err := partialVersion(match)
			if err != nil {
				return nil, err
			}
			add(major, minor, patch)
			if patch > 0 {
				add(major, minor, patch-1)
			}
			add(major, minor, patch+1)
			add(major, minor+1, 0)
			add(major+1, 0, 0)
		}
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i].LessThan(versions[j]) })
	return versions, nil
}

func partialVersion(raw string) (uint64, uint64, uint64, error) {
	parts := strings.Split(strings.ReplaceAll(strings.ReplaceAll(raw, "X", "0"), "x", "0"), ".")
	for i := range parts {
		parts[i] = strings.ReplaceAll(parts[i], "*", "0")
	}
	for len(parts) < 3 {
		parts = append(parts, "0")
	}
	major, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	minor, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	patch, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	return major, minor, patch, nil
}

func stripLeadingVAfterOperator(value string) string {
	const operators = "<>=!~^"
	index := 0
	for index < len(value) && strings.ContainsRune(operators, rune(value[index])) {
		index++
	}
	if index < len(value) && (value[index] == 'v' || value[index] == 'V') {
		return value[:index] + value[index+1:]
	}
	return value
}

func isComparatorWithBareMajor(value string) bool {
	for _, prefix := range []string{"<=", ">=", "!=", "<", ">", "=", "~", "^"} {
		if strings.HasPrefix(value, prefix) {
			return bareMajorPattern.MatchString(strings.TrimPrefix(value, prefix))
		}
	}
	return false
}
