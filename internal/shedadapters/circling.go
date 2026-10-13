// circling.go defines the operator's recorded decision on an escalated round:
// a small file the `lyx loom circling` verbs write and the Bouncer reads, and `lyx loom resume` checks for a pending one through PendingCirclingDecision.
// The writer owns the mechanical precondition (the latest round is escalated, and no decision exists for it yet);
// the run-state half (the run is awaiting at a Bouncer row) belongs to the verbs.

package shedadapters

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// CirclingDecision is the operator's answer to an escalated round.
type CirclingDecision string

// The two legal CirclingDecision spellings.
const (
	CirclingAccept   CirclingDecision = "accept"
	CirclingContinue CirclingDecision = "continue"
)

var (
	// ErrNotEscalated reports that the run directory's latest round is not escalated.
	ErrNotEscalated = errors.New("shedadapters: the latest round is not escalated")
	// ErrCirclingDecided reports that a decision is already recorded for the escalated round.
	ErrCirclingDecided = errors.New("shedadapters: a circling decision is already recorded for this round")
)

// circlingDecisionHeader mirrors a circling decision file's YAML frontmatter.
// A file an older binary wrote carries no cause and reads as EscalationCircling.
// Automatic is true for a decision the Bouncer recorded itself: an auto-accept, or the soft-cap rule's accept or continue; a parent's decision never sets it.
type circlingDecisionHeader struct {
	Round     int    `yaml:"round"`
	Decision  string `yaml:"decision"`
	Cause     string `yaml:"cause"`
	Settled   bool   `yaml:"settled"`
	Automatic bool   `yaml:"automatic"`
}

// LatestRound returns the run directory's latest round, 0 when it holds none.
// It is the round RecordCirclingDecision records its decision for.
func LatestRound(runDir string) (int, error) {
	return ResolveRound(runDir, func(n int) string { return filepath.Base(roundReviewPath(runDir, n)) })
}

// RecordCirclingDecision records d as pending for the run directory's latest round and returns that round and its escalation cause.
// It returns ErrNotEscalated unless that round carries an escalation record,
// or, for a run directory an older binary wrote, a recorded CIRCLING verdict;
// the decision file records the escalation's cause, EscalationCircling for the legacy case.
// It returns ErrCirclingDecided when a decision file for the round already exists;
// the existing file is left untouched.
func RecordCirclingDecision(runDir string, d CirclingDecision) (int, EscalationCause, error) {
	if d != CirclingAccept && d != CirclingContinue {
		return 0, "", fmt.Errorf("shedadapters: circling decision must be %q or %q, got %q", CirclingAccept, CirclingContinue, d)
	}
	round, err := LatestRound(runDir)
	if err != nil {
		return 0, "", err
	}
	if round == 0 {
		return 0, "", ErrNotEscalated
	}
	cause, _, escalated, err := readEscalation(runDir, round)
	if err != nil {
		return 0, "", err
	}
	if !escalated {
		if verdict, _, judged := recordedVerdict(runDir, round); !judged || verdict != verdictCircling {
			return 0, "", ErrNotEscalated
		}
		cause = EscalationCircling
	}

	if err := createCirclingDecision(runDir, round, d, cause, false); err != nil {
		return 0, "", err
	}
	return round, cause, nil
}

// recordAutoAccept records round's circling decision as an automatic, not yet settled accept with cause budget.
// Its callers are an auto-accept perch's harvest at the soft cap and the soft-cap rule's accept over a ledger with no design-wrong entry.
// It returns ErrCirclingDecided when a decision file for the round already exists, leaving it untouched.
func recordAutoAccept(runDir string, round int) error {
	return createCirclingDecision(runDir, round, CirclingAccept, EscalationBudget, true)
}

// recordAutoContinue records round's circling decision as an automatic, not yet settled continue with cause budget.
// It returns ErrCirclingDecided when a decision file for the round already exists, leaving it untouched.
func recordAutoContinue(runDir string, round int) error {
	return createCirclingDecision(runDir, round, CirclingContinue, EscalationBudget, true)
}

// createCirclingDecision writes round's pending decision file, refusing with ErrCirclingDecided to overwrite one.
func createCirclingDecision(runDir string, round int, d CirclingDecision, cause EscalationCause, automatic bool) error {
	path := circlingDecisionPath(runDir, round)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return ErrCirclingDecided
		}
		return fmt.Errorf("shedadapters: create circling decision %s: %w", path, err)
	}
	content, err := renderCirclingDecision(round, d, cause, false, automatic)
	if err == nil {
		_, err = f.Write(content)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("shedadapters: write circling decision %s: %w", path, err)
	}
	return nil
}

// PendingCirclingDecision reports whether the run directory's latest round carries a circling decision file that is not settled.
// The round resolves as RecordCirclingDecision resolves it; a run directory with no round has no pending decision.
// Every error names the path it concerns: the run directory or report file, or the decision file.
func PendingCirclingDecision(runDir string) (bool, error) {
	round, err := ResolveRound(runDir, func(n int) string { return filepath.Base(roundReviewPath(runDir, n)) })
	if err != nil {
		return false, err
	}
	if round == 0 {
		return false, nil
	}
	_, _, settled, exists, err := readCirclingDecision(runDir, round)
	if err != nil {
		if exists {
			return false, fmt.Errorf("shedadapters: circling decision %s: %w", circlingDecisionPath(runDir, round), err)
		}
		return false, err
	}
	return exists && !settled, nil
}

// PendingEscalation reports whether the run directory's latest round carries a Bouncer-written escalation that no circling decision has answered yet.
// When pending, it returns that round and the path of its escalation brief.
// The round resolves as PendingCirclingDecision resolves it.
// A run directory with no round, a round with no escalation record and a round whose decision file exists, settled or not, are not pending;
// a legacy round escalated only by a recorded CIRCLING verdict carries no escalation record, so it is not pending either.
// A malformed escalation record is returned as its error, wrapping ErrEscalationMalformed.
func PendingEscalation(runDir string) (round int, briefPath string, pending bool, err error) {
	round, err = ResolveRound(runDir, func(n int) string { return filepath.Base(roundReviewPath(runDir, n)) })
	if err != nil || round == 0 {
		return 0, "", false, err
	}
	_, _, escalated, err := readEscalation(runDir, round)
	if err != nil || !escalated {
		return 0, "", false, err
	}
	_, _, _, decided, err := readCirclingDecision(runDir, round)
	if err != nil {
		if decided {
			return 0, "", false, fmt.Errorf("shedadapters: circling decision %s: %w", circlingDecisionPath(runDir, round), err)
		}
		return 0, "", false, err
	}
	if decided {
		return 0, "", false, nil
	}
	return round, escalationPath(runDir, round), true, nil
}

// readCirclingDecision reads round's decision file, returning the decision, its escalation cause, whether it is settled, and whether a file exists.
// A file without a cause reads as EscalationCircling.
// A present file that is malformed is an error:
// the round must match the filename, the decision and the cause must each be one of their values, and settled is legal only on an accept.
func readCirclingDecision(runDir string, round int) (CirclingDecision, EscalationCause, bool, bool, error) {
	decision, cause, settled, _, exists, err := readCirclingFile(runDir, round)
	return decision, cause, settled, exists, err
}

// readCirclingFile reads round's decision file as readCirclingDecision does, and also returns whether the accept was recorded automatically.
func readCirclingFile(runDir string, round int) (decision CirclingDecision, cause EscalationCause, settled, automatic, exists bool, err error) {
	path := circlingDecisionPath(runDir, round)
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", "", false, false, false, nil
		}
		return "", "", false, false, false, fmt.Errorf("shedadapters: read circling decision %s: %w", path, err)
	}
	header, err := splitFrontmatter(raw, "circling decision")
	if err != nil {
		return "", "", false, false, true, err
	}
	var parsed circlingDecisionHeader
	if err := yaml.Unmarshal([]byte(header), &parsed); err != nil {
		return "", "", false, false, true, fmt.Errorf("bouncer: circling decision frontmatter is not valid YAML: %w", err)
	}
	if parsed.Round != round {
		return "", "", false, false, true, fmt.Errorf("bouncer: circling decision round %d disagrees with its filename's round %d", parsed.Round, round)
	}
	decision = CirclingDecision(parsed.Decision)
	if decision != CirclingAccept && decision != CirclingContinue {
		return "", "", false, false, true, fmt.Errorf("bouncer: circling decision must be %q or %q, got %q", CirclingAccept, CirclingContinue, parsed.Decision)
	}
	cause = EscalationCause(parsed.Cause)
	if parsed.Cause == "" {
		cause = EscalationCircling
	}
	if !cause.valid() {
		return "", "", false, false, true, fmt.Errorf("bouncer: circling decision cause must be %q, %q or %q, got %q", EscalationCircling, EscalationBudget, EscalationHardCap, parsed.Cause)
	}
	if parsed.Settled && decision != CirclingAccept {
		return "", "", false, false, true, fmt.Errorf("bouncer: circling decision %q cannot be settled; only an accept settles", parsed.Decision)
	}
	return decision, cause, parsed.Settled, parsed.Automatic, true, nil
}

// settleCirclingAccept rewrites round's pending accept as settled.
// It errors when the decision is absent or is not an accept, and keeps the record's automatic value.
func settleCirclingAccept(runDir string, round int) error {
	decision, cause, _, automatic, exists, err := readCirclingFile(runDir, round)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("shedadapters: no circling decision recorded for round %d", round)
	}
	if decision != CirclingAccept {
		return fmt.Errorf("shedadapters: round %d circling decision is %q; only an accept settles", round, decision)
	}
	content, err := renderCirclingDecision(round, decision, cause, true, automatic)
	if err != nil {
		return err
	}
	path := circlingDecisionPath(runDir, round)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("shedadapters: write circling decision %s: %w", path, err)
	}
	return nil
}

// renderCirclingDecision marshals a decision file's bytes: the YAML frontmatter alone.
func renderCirclingDecision(round int, d CirclingDecision, cause EscalationCause, settled, automatic bool) ([]byte, error) {
	headerBytes, err := yaml.Marshal(circlingDecisionHeader{Round: round, Decision: string(d), Cause: string(cause), Settled: settled, Automatic: automatic})
	if err != nil {
		return nil, fmt.Errorf("bouncer: render circling decision frontmatter: %w", err)
	}
	return append(append([]byte("---\n"), headerBytes...), []byte("---\n")...), nil
}
