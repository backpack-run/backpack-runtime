package updater

import (
	"fmt"
	"strconv"
	"strings"
)

type Version struct {
	Major, Minor, Patch int
	Prerelease          []string
}

func ParseVersion(value string) (Version, error) {
	value = strings.TrimSpace(value)
	if fields := strings.Fields(value); len(fields) > 0 {
		value = fields[0]
	}
	value = strings.TrimPrefix(value, "v")
	value = strings.SplitN(value, "+", 2)[0]
	parts := strings.SplitN(value, "-", 2)
	core := strings.Split(parts[0], ".")
	if len(core) != 3 {
		return Version{}, fmt.Errorf("invalid semantic version %q", value)
	}
	numbers := make([]int, 3)
	for i, item := range core {
		if item == "" || (len(item) > 1 && item[0] == '0') {
			return Version{}, fmt.Errorf("invalid semantic version %q", value)
		}
		n, err := strconv.Atoi(item)
		if err != nil || n < 0 {
			return Version{}, fmt.Errorf("invalid semantic version %q", value)
		}
		numbers[i] = n
	}
	result := Version{Major: numbers[0], Minor: numbers[1], Patch: numbers[2]}
	if len(parts) == 2 {
		if parts[1] == "" {
			return Version{}, fmt.Errorf("invalid semantic version %q", value)
		}
		result.Prerelease = strings.Split(parts[1], ".")
		for _, identifier := range result.Prerelease {
			if identifier == "" {
				return Version{}, fmt.Errorf("invalid semantic version %q", value)
			}
			for _, char := range identifier {
				if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') && char != '-' {
					return Version{}, fmt.Errorf("invalid semantic version %q", value)
				}
			}
		}
	}
	return result, nil
}

func (v Version) Compare(other Version) int {
	left := []int{v.Major, v.Minor, v.Patch}
	right := []int{other.Major, other.Minor, other.Patch}
	for i := range left {
		if left[i] < right[i] {
			return -1
		}
		if left[i] > right[i] {
			return 1
		}
	}
	if len(v.Prerelease) == 0 && len(other.Prerelease) == 0 {
		return 0
	}
	if len(v.Prerelease) == 0 {
		return 1
	}
	if len(other.Prerelease) == 0 {
		return -1
	}
	for i := 0; i < len(v.Prerelease) && i < len(other.Prerelease); i++ {
		if comparison := compareIdentifier(v.Prerelease[i], other.Prerelease[i]); comparison != 0 {
			return comparison
		}
	}
	if len(v.Prerelease) < len(other.Prerelease) {
		return -1
	}
	if len(v.Prerelease) > len(other.Prerelease) {
		return 1
	}
	return 0
}

func compareIdentifier(left, right string) int {
	leftNumber, leftErr := strconv.Atoi(left)
	rightNumber, rightErr := strconv.Atoi(right)
	if leftErr == nil && rightErr == nil {
		if leftNumber < rightNumber {
			return -1
		}
		if leftNumber > rightNumber {
			return 1
		}
		return 0
	}
	if leftErr == nil {
		return -1
	}
	if rightErr == nil {
		return 1
	}
	return strings.Compare(left, right)
}

func normalizeTag(value string) (string, error) {
	if _, err := ParseVersion(value); err != nil {
		return "", err
	}
	value = strings.Fields(strings.TrimSpace(value))[0]
	if !strings.HasPrefix(value, "v") {
		value = "v" + value
	}
	return value, nil
}
