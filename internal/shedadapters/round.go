// round.go builds every round-numbered artifact path this package reads or writes.
// Every such path is built here, never assembled ad hoc at a call site.
// The Bouncer keeps no attempt number: a retry is a fresh Shed bounce after Stuck, not an in-producer retry,
// so a round number alone names every file.

package shedadapters

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// ResolveRound returns the highest round N for which reportName(N) names an existing file inside runDir.
// It scans upward from 1 and stops at the first absent round.
// It returns 0 when reportName(1) does not exist, which is a non-error answer meaning no round has been run yet.
// The return value is the round to judge, and the round to write is that value plus one.
func ResolveRound(runDir string, reportName func(round int) string) (int, error) {
	info, err := os.Stat(runDir)
	if err != nil {
		return 0, fmt.Errorf("shedadapters: stat run dir %s: %w", runDir, err)
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("shedadapters: run dir %s is not a directory", runDir)
	}

	for n := 1; ; n++ {
		path := filepath.Join(runDir, reportName(n))
		if _, err := os.Stat(path); err != nil {
			// Only fs.ErrNotExist means this round's report is absent.
			// Any other stat error (permission, I/O) must be returned, not read as absence.
			// Reading it as absence would truncate the scan mid-sequence or re-seed a segment that has already judged rounds.
			if errors.Is(err, fs.ErrNotExist) {
				return n - 1, nil
			}
			return 0, fmt.Errorf("shedadapters: stat report file %s: %w", path, err)
		}
	}
}

// verdictPath returns the path of the bouncer verdict file for round inside runDir.
func verdictPath(runDir string, round int) string {
	return filepath.Join(runDir, fmt.Sprintf("round-%d-bouncer-verdict.md", round))
}

// ledgerPath returns the path of the bouncer ledger file for round inside runDir.
func ledgerPath(runDir string, round int) string {
	return filepath.Join(runDir, fmt.Sprintf("round-%d-bouncer-ledger.md", round))
}

// factsPath returns the path of the per-round facts file for round inside runDir.
// The file is an input the judge reads, regenerated before every judge call,
// so it is not one of the judge's declared outputs.
func factsPath(runDir string, round int) string {
	return filepath.Join(runDir, fmt.Sprintf("round-%d-facts.md", round))
}

// circlingDecisionPath returns the path of the circling decision file for round inside runDir.
// The file lives in the run directory, so it is committed with the run.
func circlingDecisionPath(runDir string, round int) string {
	return filepath.Join(runDir, fmt.Sprintf("round-%d-circling-decision.md", round))
}

// escalationPath returns the path of the escalation brief file for round inside runDir.
// The file lives in the run directory, so it is committed with the run.
func escalationPath(runDir string, round int) string {
	return filepath.Join(runDir, fmt.Sprintf("round-%d-escalation.md", round))
}

// decisionSnapshotPath returns the path of the decision snapshot file for round inside runDir.
// The file holds the titles of the decision record's `Added after Discussion` entries.
// They are taken when the round was first escalated with cause decision.
func decisionSnapshotPath(runDir string, round int) string {
	return filepath.Join(runDir, fmt.Sprintf("round-%d-decision-snapshot.yaml", round))
}

// parentNoticePath returns the path of the one-line parent notice file for round inside runDir.
func parentNoticePath(runDir string, round int) string {
	return filepath.Join(runDir, fmt.Sprintf("round-%d-parent-notice.md", round))
}

// judgeOutputs returns the three files one judge pass for round declares as its shuttle run's OutputFiles:
// the verdict, the ledger, and the next round's focus file, in the order the judge prompt's markers are filled from.
// The set is built in one place because shuttleengine.Attach set-matches a persisted run.json against it.
// The judge spawn and every probe for a live judge must name byte-identical paths.
// Otherwise a probe silently matches nothing, which at the call site looks the same as having no live agent.
func judgeOutputs(runDir string, round int) []string {
	return []string{
		verdictPath(runDir, round),
		ledgerPath(runDir, round),
		focusPath(runDir, round+1),
	}
}

// focusPath returns the path of the focus file for round inside runDir.
// round is the round the file targets, not the round that wrote it.
// A judge call for round N writes round-<N+1>-focus.md,
// so the round producer reads its own round's focus file with no off-by-one reasoning.
func focusPath(runDir string, round int) string {
	return filepath.Join(runDir, fmt.Sprintf("round-%d-focus.md", round))
}

// The outcomes a review segment's last round can end in.
const (
	// RoundOutcomeAccepted is a last round with neither a circling decision nor an escalation file.
	RoundOutcomeAccepted = "accepted"
	// RoundOutcomeEscalated is a last round holding an escalation file and no circling decision.
	RoundOutcomeEscalated = "escalated"
	// RoundOutcomeCirclingDecided is a last round holding a circling decision, with or without an escalation file.
	RoundOutcomeCirclingDecided = "circling-decided"
)

// RoundSummary summarizes one review segment's rounds from the names of its files.
// Outcome is empty when Rounds is 0.
type RoundSummary struct {
	Rounds          int
	Outcome         string
	EscalatedRounds []int
}

// parseRoundFileName splits a `round-<N>-<kind>` file name into N and kind.
// It reports false for a name of another shape, or whose N is not a positive integer.
func parseRoundFileName(name string) (round int, kind string, ok bool) {
	rest, ok := strings.CutPrefix(name, "round-")
	if !ok {
		return 0, "", false
	}
	digits, kind, ok := strings.Cut(rest, "-")
	if !ok {
		return 0, "", false
	}
	round, err := strconv.Atoi(digits)
	if err != nil || round < 1 {
		return 0, "", false
	}
	return round, kind, true
}

// OutputRound returns the round number that a review segment's output path names, and true.
// It reads the base name of path as `round-<N>-<kind>`.
// It returns 0 and false for an empty path, a name without that shape, or an N that is not a positive integer.
func OutputRound(path string) (int, bool) {
	round, _, ok := parseRoundFileName(filepath.Base(path))
	if !ok {
		return 0, false
	}
	return round, true
}

// ReviewRounds summarizes one review segment's directory from the file names directly in it.
// A round is a round number that has a `round-<N>-review.md`.
// A numbered round holding only a focus file does not count.
// Outcome is the last such round's, and EscalatedRounds lists, ascending, every round number holding an escalation file.
// A name it does not recognise is ignored.
func ReviewRounds(names []string) RoundSummary {
	reviewed := map[int]bool{}
	circled := map[int]bool{}
	escalated := map[int]bool{}
	for _, name := range names {
		n, kind, ok := parseRoundFileName(name)
		if !ok {
			continue
		}
		switch kind {
		case "review.md":
			reviewed[n] = true
		case "circling-decision.md":
			circled[n] = true
		case "escalation.md":
			escalated[n] = true
		}
	}

	var summary RoundSummary
	last := 0
	for n := range reviewed {
		summary.Rounds++
		last = max(last, n)
	}
	for n := range escalated {
		summary.EscalatedRounds = append(summary.EscalatedRounds, n)
	}
	slices.Sort(summary.EscalatedRounds)
	switch {
	case summary.Rounds == 0:
	case circled[last]:
		summary.Outcome = RoundOutcomeCirclingDecided
	case escalated[last]:
		summary.Outcome = RoundOutcomeEscalated
	default:
		summary.Outcome = RoundOutcomeAccepted
	}
	return summary
}
