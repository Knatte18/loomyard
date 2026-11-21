// triage.go holds the verify gate's flaky-failure notes:
// the warning and the friction note that name the failures of a first verify run that passed on rerun.

package websterengine

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// flakyFailure is one failure of a first verify run whose rerun passed.
type flakyFailure struct {
	failure VerifyFailure
	// log is the first run's verify log.
	log string
}

// commandFailure reports whether f named no test or package, so the verify command itself failed.
func (f flakyFailure) commandFailure() bool {
	return f.failure.Kind == FailureKindOpaque
}

// splitFlaky returns the ids of the flaky test and package failures, and the logs of the flaky verify command failures.
func splitFlaky(flaky []flakyFailure) (ids, commandLogs []string) {
	for _, f := range flaky {
		if f.commandFailure() {
			commandLogs = append(commandLogs, f.log)
			continue
		}
		ids = append(ids, f.failure.ID)
	}
	return ids, commandLogs
}

// triageWarnings returns the warnings naming the flaky failures, or nil when there are none.
// A first run that named no failing test or package is reported as a failure of the verify command.
// It is never reported as a flaky test.
func triageWarnings(flaky []flakyFailure) []string {
	ids, commandLogs := splitFlaky(flaky)
	var warnings []string
	if len(ids) > 0 {
		warnings = append(warnings, "integration verify: flaky failures passed or vanished on rerun: "+strings.Join(ids, ", "))
	}
	for _, log := range commandLogs {
		warnings = append(warnings, "integration verify: the verify command failed naming no failing test and passed on rerun; first run's log: "+log)
	}
	return warnings
}

// writeTriageFrictionNote records the flaky failures as a friction note, each with its first run's log and output tail.
// A failure that vanishes on rerun is environment trouble, which the hub collects from friction notes,
// and the kept output is what lets a reader find its cause.
// It is a no-op when frictionDir is empty or flaky is empty.
func writeTriageFrictionNote(frictionDir string, flaky []flakyFailure) error {
	if frictionDir == "" || len(flaky) == 0 {
		return nil
	}
	friction.EnsureDir(frictionDir)
	path := friction.NotePath(frictionDir, "webster-verify-triage")
	if path == "" {
		return nil
	}

	var tests, commands strings.Builder
	for _, f := range flaky {
		if f.commandFailure() {
			fmt.Fprintf(&commands, "- first run's log %s\n", f.log)
			writeFailureOutput(&commands, f.failure.Tail)
			continue
		}
		fmt.Fprintf(&tests, "- %s, first run's log %s\n", f.failure.ID, f.log)
		writeFailureOutput(&tests, f.failure.Tail)
	}

	var b strings.Builder
	b.WriteString("Integration-suite triage: non-regression verify failures\n\n")
	if tests.Len() > 0 {
		b.WriteString("These identities failed the first verify run and passed or vanished on rerun (flaky):\n\n")
		b.WriteString(tests.String())
	}
	if commands.Len() > 0 {
		b.WriteString("The verify command failed its first run naming no failing test, and passed on rerun; the failure may be the verify step's own, not a test's:\n\n")
		b.WriteString(commands.String())
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("webster: triage: write friction note %s: %w", path, err)
	}
	return nil
}

// writeFailureOutput writes a first run's output tail under its bullet as a fenced block, or says the output was empty.
// The fence is one backtick longer than the longest backtick run in the tail, and never shorter than three.
// A fence line inside the output therefore cannot close the block early.
func writeFailureOutput(b *strings.Builder, tail string) {
	if strings.TrimSpace(tail) == "" {
		b.WriteString("\n  Its output was empty.\n\n")
		return
	}
	fence := strings.Repeat("`", max(3, longestBacktickRun(tail)+1))
	b.WriteString("\n  " + fence + "text\n")
	for _, line := range strings.Split(tail, "\n") {
		b.WriteString("  " + line + "\n")
	}
	b.WriteString("  " + fence + "\n\n")
}

// longestBacktickRun returns the length of the longest run of consecutive backticks in s.
func longestBacktickRun(s string) int {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			longest = max(longest, run)
			continue
		}
		run = 0
	}
	return longest
}

// backgroundShellOutcome records how a run ended with a background shell still outstanding.
// It says what finally ends the shell.
type backgroundShellOutcome struct {
	// description is the run's final outcome.
	description string
	// strandReclaimed is true when the next run reclaims Master's strand; false when shuttle removes it at run end.
	strandReclaimed bool
}

// finishedShellOutcome describes a run that returned a RunResult.
// The shuttle run ended done, so shuttle removes Master's strand when the run finishes.
// The session and the shell end with it.
func finishedShellOutcome(runResult RunResult) backgroundShellOutcome {
	description := runResult.Outcome
	switch runResult.Outcome {
	case outcomeStuck:
		description = "stuck (" + runResult.StuckReason + ")"
	}
	return backgroundShellOutcome{description: description}
}

// errorShellOutcome describes a run that returned err.
// strandReclaimed is true for an error that leaves Master's strand alive (died, timeout).
// It is false for an error after a shuttle-done end, where shuttle still removes the strand.
func errorShellOutcome(err error, strandReclaimed bool) backgroundShellOutcome {
	return backgroundShellOutcome{description: "error (" + err.Error() + ")", strandReclaimed: strandReclaimed}
}

// expiredShellWarning states what happened to one background shell that outlived Master's run.
// It says the run never read the shell's result, and lym did not stop the shell.
// It also gives what ends the shell and the run's final outcome.
// It names the shell's label, the signal that reported it, and how long it was outstanding.
// The run warning and the friction note use this same line, so the two carry the same wording.
func expiredShellWarning(shell shuttleengine.EndedShell, outcome backgroundShellOutcome) string {
	ends := "shuttle removes Master's strand when the run finishes, which ends the session and the shell with it where the shell is in the pane's process tree"
	if outcome.strandReclaimed {
		ends = "Master's strand stays alive until the next `lym webster run` reclaims it at entry, which ends the session and the shell with it where the shell is in the pane's process tree"
	}
	return fmt.Sprintf("background shell `%s` (reported by the %s signal) was still running, outstanding for %s, when the run ended; the run never read the shell's result, and lym did not stop the shell; %s; the run's final outcome: %s", shell.Label, shell.Signal, shell.Outstanding.Round(time.Second), ends, outcome.description)
}

// writeBackgroundShellFrictionNote records the shells still outstanding when Master's run ended.
// It also records the run's final outcome,
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
