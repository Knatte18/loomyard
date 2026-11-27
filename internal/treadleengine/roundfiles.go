// roundfiles.go names and locates a round's artifact files inside a block's run directory.
// Every block artifact lives flat in the run directory.
// This file is the one place that turns a round and attempt into concrete paths.
// Mapping those paths and a Profile's content onto a RoundRunner's input type is the runner adapter's job.

package treadleengine

import (
	"fmt"
	"path/filepath"
)

// roundToken returns the artifact-file token for a round and attempt.
// The first attempt gets the bare round number, and each retry gets a letter suffix ("3b", "3c").
func roundToken(round, attempt int) string {
	if attempt <= 1 {
		return fmt.Sprintf("%d", round)
	}
	// Attempt 2 maps to 'b' and attempt 3 to 'c'.
	// Attempt 1 takes no letter, so the suffixes start at 'b', not 'a'.
	letter := rune('a' + attempt - 1)
	return fmt.Sprintf("%d%c", round, letter)
}

// roundArtifactPaths holds the file paths one round and attempt may produce.
// Not every field is written every round.
// Judge is written only when the judge runs, and Gate only when a command gate fails.
type roundArtifactPaths struct {
	Review      string
	FixerReport string
	Judge       string
	Handoff     string
	Gate        string
	Seed        string
}

// artifactPaths returns the artifact file paths for a round and attempt inside runDir.
func artifactPaths(runDir string, round, attempt int) roundArtifactPaths {
	token := roundToken(round, attempt)
	return roundArtifactPaths{
		Review:      filepath.Join(runDir, fmt.Sprintf("round-%s-review.md", token)),
		FixerReport: filepath.Join(runDir, fmt.Sprintf("round-%s-fixer-report.md", token)),
		Judge:       filepath.Join(runDir, fmt.Sprintf("round-%s-judge.md", token)),
		Handoff:     filepath.Join(runDir, fmt.Sprintf("round-%s-handoff.md", token)),
		Gate:        filepath.Join(runDir, fmt.Sprintf("round-%s-gate.md", token)),
		Seed:        filepath.Join(runDir, fmt.Sprintf("round-%s-seed.md", token)),
	}
}
