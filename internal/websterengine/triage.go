// triage.go holds the verify gate's flaky-failure notes: the warning and the friction note that name the identities that failed once and passed on rerun.

package websterengine

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/friction"
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

// backgroundShellOutcome is how a run ended after Master's wait counted a turn end past a background shell,
// and so what finally ends the shell.
type backgroundShellOutcome struct {
	// description is the run's outcome after the counted turn end.
	description string
	// strandReclaimed is true when Master's strand outlives the run until the next `lyx webster run` reclaims it,
	// and false when shuttle removes the strand as the run finishes.
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
// strandReclaimed is true for an error that leaves Master's strand alive (asking, died, timeout),
// and false for an error after a shuttle-done end, where shuttle still removes the strand.
func errorShellOutcome(err error, strandReclaimed bool) backgroundShellOutcome {
	return backgroundShellOutcome{description: "error (" + err.Error() + ")", strandReclaimed: strandReclaimed}
}

// backgroundShellSentence states what happened to one background shell the Master wait counted a turn end past,
// what ends it, and the run's outcome, so the friction note and the warning carry the same wording.
// waitMin is the bound in minutes; a non-positive value names the key alone.
func backgroundShellSentence(label string, waitMin int, outcome backgroundShellOutcome) string {
	bound := "`background_shell_wait_min`"
	if waitMin > 0 {
		bound = fmt.Sprintf("`background_shell_wait_min` (%d minutes)", waitMin)
	}
	ends := "shuttle removes Master's strand when the run finishes, which ends the session and the shell with it"
	if outcome.strandReclaimed {
		ends = "Master's strand stays alive until the next `lyx webster run` reclaims it at entry, which ends the session and the shell with it"
	}
	return fmt.Sprintf("background shell `%s` ran past %s: the wait stopped waiting and counted Master's turn end, and lyx did not stop the shell; %s; the run's outcome after that turn end: %s", label, bound, ends, outcome.description)
}

// expiredShellWarning returns the warning for one background shell the Master wait counted a turn end past.
func expiredShellWarning(label string, waitMin int, outcome backgroundShellOutcome) string {
	return backgroundShellSentence(label, waitMin, outcome)
}

// writeBackgroundShellFrictionNote records the shells Master's wait counted a turn end past and the run's outcome,
// so reflection has the evidence a hang otherwise leaves only in events.jsonl.
// waitMin is the bound in minutes; a non-positive value names the key alone.
// It is a no-op when frictionDir is empty or labels is empty.
func writeBackgroundShellFrictionNote(frictionDir string, labels []string, waitMin int, outcome backgroundShellOutcome) error {
	if frictionDir == "" || len(labels) == 0 {
		return nil
	}
	friction.EnsureDir(frictionDir)
	path := friction.NotePath(frictionDir, "webster-background-shell")
	if path == "" {
		return nil
	}

	var b strings.Builder
	b.WriteString("Master's wait counted a turn end past a background shell\n\n")
	for _, l := range labels {
		b.WriteString("- " + backgroundShellSentence(l, waitMin, outcome) + "\n")
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
