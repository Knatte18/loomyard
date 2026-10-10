// triage.go holds the verify gate's flaky-failure notes: the warning and the friction note that name the identities that failed once and passed on rerun.

package websterengine

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// triageWarnings returns the warning naming the flaky identities, or nil when there are none.
func triageWarnings(flaky []string) []string {
	if len(flaky) == 0 {
		return nil
	}
	return []string{"integration verify: flaky failures passed or vanished on rerun: " + strings.Join(flaky, ", ")}
}

// writeTriageFrictionNote records the flaky identities as a friction note,
// since a failure that vanishes on rerun is environment trouble the hub collects from friction notes.
// It is a no-op when frictionDir is empty or flaky is empty.
func writeTriageFrictionNote(frictionDir string, flaky []string) error {
	if frictionDir == "" || len(flaky) == 0 {
		return nil
	}
	friction.EnsureDir(frictionDir)
	path := friction.NotePath(frictionDir, "webster-verify-triage")
	if path == "" {
		return nil
	}

	var b strings.Builder
	b.WriteString("Integration-suite triage: non-regression verify failures\n\n")
	b.WriteString("These identities failed the first verify run and passed or vanished on rerun (flaky):\n\n")
	for _, id := range flaky {
		b.WriteString("- " + id + "\n")
	}
	b.WriteString("\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("webster: triage: write friction note %s: %w", path, err)
	}
	return nil
}

// backgroundShellOutcome is how a run ended with a background shell still outstanding, and so what finally ends the shell.
type backgroundShellOutcome struct {
	// description is the run's final outcome.
	description string
	// strandReclaimed is true when Master's strand outlives the run until the next `lyx webster run` reclaims it, and false when shuttle removes the strand as the run finishes.
	strandReclaimed bool
}

// finishedShellOutcome describes a run that returned a RunResult:
// the shuttle run ended done, so shuttle removes Master's strand when the run finishes, and the session and the shell end with it.
func finishedShellOutcome(runResult RunResult) backgroundShellOutcome {
	description := runResult.Outcome
	switch runResult.Outcome {
	case outcomeStuck:
		description = "stuck (" + runResult.StuckReason + ")"
	case outcomePaused:
		description = "paused"
	}
	return backgroundShellOutcome{description: description}
}

// errorShellOutcome describes a run that returned err.
// strandReclaimed is true for an error that leaves Master's strand alive (died, timeout), and false for an error after a shuttle-done end, where shuttle still removes the strand.
func errorShellOutcome(err error, strandReclaimed bool) backgroundShellOutcome {
	return backgroundShellOutcome{description: "error (" + err.Error() + ")", strandReclaimed: strandReclaimed}
}

// expiredShellWarning states what happened to one background shell Master's run ended over, what ends it, and the run's final outcome.
// It names the shell's label, the signal that reported it and how long it was outstanding.
// It is both the run warning and the friction note's line for that shell, so the two carry the same wording.
func expiredShellWarning(shell shuttleengine.EndedShell, outcome backgroundShellOutcome) string {
	ends := "shuttle removes Master's strand when the run finishes, which ends the session and the shell with it where the shell is in the pane's process tree"
	if outcome.strandReclaimed {
		ends = "Master's strand stays alive until the next `lyx webster run` reclaims it at entry, which ends the session and the shell with it where the shell is in the pane's process tree"
	}
	return fmt.Sprintf("background shell `%s` (reported by the %s signal) was still running, outstanding for %s, when the run ended, and lyx did not stop the shell; %s; the run's final outcome: %s", shell.Label, shell.Signal, shell.Outstanding.Round(time.Second), ends, outcome.description)
}

// writeBackgroundShellFrictionNote records the shells Master's run ended over and the run's final outcome,
// so reflection has the evidence a hang otherwise leaves only in events.jsonl.
// It is a no-op when frictionDir is empty or shells is empty.
func writeBackgroundShellFrictionNote(frictionDir string, shells []shuttleengine.EndedShell, outcome backgroundShellOutcome) error {
	if frictionDir == "" || len(shells) == 0 {
		return nil
	}
	friction.EnsureDir(frictionDir)
	path := friction.NotePath(frictionDir, "webster-background-shell")
	if path == "" {
		return nil
	}

	var b strings.Builder
	b.WriteString("Master's run ended with a background shell outstanding\n\n")
	for _, shell := range shells {
		b.WriteString("- " + expiredShellWarning(shell, outcome) + "\n")
	}
	b.WriteString("\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("webster: background shells: write friction note %s: %w", path, err)
	}
	return nil
}

// failureIDs returns the ids of fs in order.
func failureIDs(fs []VerifyFailure) []string {
	ids := make([]string, 0, len(fs))
	for _, f := range fs {
		ids = append(ids, f.ID)
	}
	return ids
}
