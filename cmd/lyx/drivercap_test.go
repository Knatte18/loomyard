// drivercap_test.go pins the loom CLI's exported AutonomousDriveStepCap constant against the number
// stated in the ly-drive skill's own "## Autonomous driver" section, so the two never drift apart
// silently. It copies sandbox_coverage_test.go's runtime.Caller mechanism to resolve the repo root,
// because that is this repo's one working way for a Go test to read a file outside its own package
// tree, and no package under the plugins tree compiles Go.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomcli"
)

// autonomousDriverHeading marks the start of the ly-drive skill's autonomous-mode section.
const autonomousDriverHeading = "## Autonomous driver"

// autonomousStepCapPhrasePattern matches the fixed "autonomous step cap: <N>" line the
// "## Autonomous driver" section carries, capturing the number.
var autonomousStepCapPhrasePattern = regexp.MustCompile(`^autonomous step cap:\s*(\d+)\s*$`)

// TestAutonomousStepCap_MatchesSkill asserts that AutonomousDriveStepCap, the number the Go-composed
// launch prompt interpolates, equals the number the ly-drive skill's own "## Autonomous driver"
// section states, so the launcher and the skill never silently run different budgets.
func TestAutonomousStepCap_MatchesSkill(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location via runtime.Caller")
	}
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(testFile)))

	skillPath := filepath.Join(repoRoot, "plugins", "ly", "skills", "ly-drive", "SKILL.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("could not read %s: %v", skillPath, err)
	}

	section, err := extractAutonomousDriverSection(string(data))
	if err != nil {
		t.Fatalf("%s: %v", skillPath, err)
	}

	skillCap, err := findAutonomousStepCapPhrase(section)
	if err != nil {
		t.Fatalf("%s: %v", skillPath, err)
	}

	if skillCap != loomcli.AutonomousDriveStepCap {
		t.Errorf(
			"autonomous step cap drift: loomcli.AutonomousDriveStepCap (internal/loomcli/driverprompt.go) = %d; %s's %q section states %d -- update whichever side is stale so the launch prompt and the skill agree",
			loomcli.AutonomousDriveStepCap, skillPath, autonomousDriverHeading, skillCap,
		)
	}
}

// extractAutonomousDriverSection returns the body of the skill file's "## Autonomous driver" section
// -- from that heading up to (but excluding) the next "## " heading, or end of file if none follows.
// It fails loudly rather than returning an empty match when the heading is absent, because a skipped
// or vacuously-passing test here is indistinguishable from a passing one and would hide exactly the
// drift this test exists to catch (the section's removal).
func extractAutonomousDriverSection(fileContents string) (string, error) {
	headingIndex := strings.Index(fileContents, autonomousDriverHeading)
	if headingIndex == -1 {
		return "", fmt.Errorf("no %q heading found; the section may have been removed or renamed", autonomousDriverHeading)
	}

	rest := fileContents[headingIndex+len(autonomousDriverHeading):]
	if nextHeadingIndex := strings.Index(rest, "\n## "); nextHeadingIndex != -1 {
		rest = rest[:nextHeadingIndex]
	}
	return rest, nil
}

// findAutonomousStepCapPhrase locates the fixed "autonomous step cap: <N>" line within section and
// returns its number. It matches only within the caller-supplied section, never the whole skill file
// -- the skill also states the unrelated operator-driven cap of 40 and a prose aside about loom's own
// worst case landing near a hundred steps, so a file-wide search would stay green against exactly the
// wrong number.
func findAutonomousStepCapPhrase(section string) (int, error) {
	for _, line := range strings.Split(section, "\n") {
		match := autonomousStepCapPhrasePattern.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil {
			continue
		}
		value, err := strconv.Atoi(match[1])
		if err != nil {
			return 0, fmt.Errorf("matched %q but could not parse its number: %w", strings.TrimSpace(line), err)
		}
		return value, nil
	}
	return 0, fmt.Errorf("no %q phrase found within the %q section", autonomousStepCapPhrasePattern.String(), autonomousDriverHeading)
}
