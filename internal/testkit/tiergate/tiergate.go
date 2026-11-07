// Package tiergate selects at run time which tiers' tests a test binary runs.
// One test binary per package serves every tier.
//
// The selection comes from the LYM_TEST_TIER environment variable, which the hub's gate config sets on each tier:
// one tier number, such as `1`, or an inclusive range, such as `0-2`.
// An unset or empty variable selects tier0 alone, the offline tier a plain `go test` runs.
// Every Test, Benchmark and Fuzz function opens with Require, which skips it when the selection does not hold its tier.
// A test reading the variable keys the Go test cache on its value, so each tier's run of a binary is cached apart.
package tiergate

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// EnvVar names the environment variable that carries the tier selection.
const EnvVar = "LYM_TEST_TIER"

// Selection is the inclusive range of tier numbers whose tests a run executes.
type Selection struct {
	// Low is the lowest selected tier.
	Low int
	// High is the highest selected tier, equal to Low for a single tier.
	High int
}

// Holds reports whether tier lies in the selection.
func (s Selection) Holds(tier int) bool {
	return tier >= s.Low && tier <= s.High
}

// String renders the selection the way LYM_TEST_TIER spells it.
func (s Selection) String() string {
	if s.Low == s.High {
		return strconv.Itoa(s.Low)
	}
	return fmt.Sprintf("%d-%d", s.Low, s.High)
}

// Selected returns the selection LYM_TEST_TIER names, tier0 alone when it is unset or empty.
// A value that is neither a tier number nor a range `<low>-<high>` with low at most high is an error.
// The error names the variable and its value.
func Selected() (Selection, error) {
	value := strings.TrimSpace(os.Getenv(EnvVar))
	if value == "" {
		return Selection{}, nil
	}
	lowText, highText, isRange := strings.Cut(value, "-")
	if !isRange {
		highText = lowText
	}
	low, lowErr := strconv.Atoi(lowText)
	high, highErr := strconv.Atoi(highText)
	if lowErr != nil || highErr != nil || low < 0 || high < low {
		return Selection{}, fmt.Errorf("tiergate: %s=%q; want a tier number such as 1 or a range such as 0-2", EnvVar, value)
	}
	return Selection{Low: low, High: high}, nil
}

// Require skips t unless the selection holds tier, and fails it when LYM_TEST_TIER does not parse.
func Require(t testing.TB, tier int) {
	t.Helper()
	selection, err := Selected()
	if err != nil {
		t.Fatal(err)
	}
	if !selection.Holds(tier) {
		t.Skipf("tier%d test; %s selects %s", tier, EnvVar, selection)
	}
}
