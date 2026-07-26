// roundfiles.go names and locates a round's artifact files inside a block's
// run dir, and builds the burlerengine.Profile each round hands to the
// Burler seam. Every block artifact lives flat in the run dir (the "round
// artifact naming and attempt suffixes" decision) — this file is the single
// place that turns a (round, attempt) pair into concrete paths.

package perchengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/burlerengine"
)

// roundToken returns the artifact-file token for a round/attempt pair: the
// bare round number for the first attempt ("3"), and the round number with
// a letter suffix for each retry after a died/timeout outcome ("3b" for
// attempt 2, "3c" for attempt 3, ...). The letter suffix is what lets a
// retried attempt claim fresh output-file paths — shuttle rejects a run
// whose OutputFiles already exist, so a same-round retry can never reuse
// its failed predecessor's paths.
func roundToken(round, attempt int) string {
	if attempt <= 1 {
		return fmt.Sprintf("%d", round)
	}
	// attempt 2 -> 'b', attempt 3 -> 'c', ... (attempt 1 has no letter, so
	// the first letter used is 'b', not 'a').
	letter := rune('a' + attempt - 1)
	return fmt.Sprintf("%d%c", round, letter)
}

// roundArtifactPaths is the set of file paths a single round/attempt may
// produce inside a run dir. Not every field is written every round: Judge
// is written only when the progress judge runs, Gate only when a command
// gate fails, Triage only when asking-triage runs.
type roundArtifactPaths struct {
	Review      string
	FixerReport string
	Judge       string
	Gate        string
	Triage      string
}

// artifactPaths returns the roundArtifactPaths for round/attempt inside
// runDir, using roundToken to name each file round-<token>-<kind>.md.
func artifactPaths(runDir string, round, attempt int) roundArtifactPaths {
	token := roundToken(round, attempt)
	return roundArtifactPaths{
		Review:      filepath.Join(runDir, fmt.Sprintf("round-%s-review.md", token)),
		FixerReport: filepath.Join(runDir, fmt.Sprintf("round-%s-fixer-report.md", token)),
		Judge:       filepath.Join(runDir, fmt.Sprintf("round-%s-judge.md", token)),
		Gate:        filepath.Join(runDir, fmt.Sprintf("round-%s-gate.md", token)),
		Triage:      filepath.Join(runDir, fmt.Sprintf("round-%s-triage.md", token)),
	}
}

// buildRoundProfile composes the burlerengine.Profile for one round: p's
// content fields carried through 1:1, this round's output paths from paths,
// and the accumulated prior-round hydration lists supplied by the loop.
// buildRoundProfile never invents priorReviews/priorFixerReports entries
// itself (e.g. appending a prior round's gate-output file) — that
// accumulation is the loop's responsibility; this function only maps
// already-decided inputs onto burlerengine's field names.
func buildRoundProfile(p Profile, paths roundArtifactPaths, priorReviews, priorFixerReports []string) burlerengine.Profile {
	return burlerengine.Profile{
		Target:            p.Target,
		Fasit:             p.Fasit,
		Rubric:            p.Rubric,
		FixScope:          p.FixScope,
		ToolUse:           p.ToolUse,
		ClusterFan:        p.ClusterFan,
		ReviewPath:        paths.Review,
		FixerReportPath:   paths.FixerReport,
		PriorReviews:      priorReviews,
		PriorFixerReports: priorFixerReports,
	}
}
