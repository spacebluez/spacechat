package update

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type Version struct {
	Major uint64
	Minor uint64
	Patch uint64
}

func ParseVersion(text string) (Version, error) {
	parts := versionPattern.FindStringSubmatch(text)
	if parts == nil {
		return Version{}, errors.New("version must use MAJOR.MINOR.PATCH without leading zeroes")
	}
	values := make([]uint64, 3)
	for index := range values {
		value, err := strconv.ParseUint(parts[index+1], 10, 64)
		if err != nil {
			return Version{}, fmt.Errorf("invalid version component: %w", err)
		}
		values[index] = value
	}
	return Version{Major: values[0], Minor: values[1], Patch: values[2]}, nil
}

func (version Version) String() string {
	return fmt.Sprintf("%d.%d.%d", version.Major, version.Minor, version.Patch)
}

func (version Version) Compare(other Version) int {
	left := [...]uint64{version.Major, version.Minor, version.Patch}
	right := [...]uint64{other.Major, other.Minor, other.Patch}
	for index := range left {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	return 0
}

type Decision uint8

const (
	DecisionCurrent Decision = iota
	DecisionOptional
	DecisionRequired
	DecisionAhead
)

func DecisionFor(current, latest, minimum Version) Decision {
	if current.Compare(minimum) < 0 {
		return DecisionRequired
	}
	if current.Compare(latest) < 0 {
		return DecisionOptional
	}
	if current.Compare(latest) > 0 {
		return DecisionAhead
	}
	return DecisionCurrent
}
