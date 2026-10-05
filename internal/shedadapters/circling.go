// circling.go defines the operator's recorded decision on an escalated round:
// a small file the `lyx loom circling` verbs write and the Bouncer reads.
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
type circlingDecisionHeader struct {
	Round    int    `yaml:"round"`
	Decision string `yaml:"decision"`
	Cause    string `yaml:"cause"`
	Settled  bool   `yaml:"settled"`
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
	round, err := ResolveRound(runDir, func(n int) string { return filepath.Base(roundReviewPath(runDir, n)) })
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
		if verdict, judged := recordedVerdict(runDir, round); !judged || verdict != verdictCircling {
			return 0, "", ErrNotEscalated
		}
		cause = EscalationCircling
	}

	path := circlingDecisionPath(runDir, round)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return 0, "", ErrCirclingDecided
		}
		return 0, "", fmt.Errorf("shedadapters: create circling decision %s: %w", path, err)
	}
	content, err := renderCirclingDecision(round, d, cause, false)
	if err == nil {
		_, err = f.Write(content)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return 0, "", fmt.Errorf("shedadapters: write circling decision %s: %w", path, err)
	}
	return round, cause, nil
}

// readCirclingDecision reads round's decision file, returning the decision, its escalation cause, whether it is settled, and whether a file exists.
// A file without a cause reads as EscalationCircling.
// A present file that is malformed is an error:
// the round must match the filename, the decision and the cause must each be one of their two values, and settled is legal only on an accept.
func readCirclingDecision(runDir string, round int) (CirclingDecision, EscalationCause, bool, bool, error) {
	path := circlingDecisionPath(runDir, round)
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", "", false, false, nil
		}
		return "", "", false, false, fmt.Errorf("shedadapters: read circling decision %s: %w", path, err)
	}
	header, err := splitFrontmatter(raw, "circling decision")
	if err != nil {
		return "", "", false, true, err
	}
	var parsed circlingDecisionHeader
	if err := yaml.Unmarshal([]byte(header), &parsed); err != nil {
		return "", "", false, true, fmt.Errorf("bouncer: circling decision frontmatter is not valid YAML: %w", err)
	}
	if parsed.Round != round {
		return "", "", false, true, fmt.Errorf("bouncer: circling decision round %d disagrees with its filename's round %d", parsed.Round, round)
	}
	decision := CirclingDecision(parsed.Decision)
	if decision != CirclingAccept && decision != CirclingContinue {
		return "", "", false, true, fmt.Errorf("bouncer: circling decision must be %q or %q, got %q", CirclingAccept, CirclingContinue, parsed.Decision)
	}
	cause := EscalationCause(parsed.Cause)
	if parsed.Cause == "" {
		cause = EscalationCircling
	}
	if cause != EscalationCircling && cause != EscalationBudget {
		return "", "", false, true, fmt.Errorf("bouncer: circling decision cause must be %q or %q, got %q", EscalationCircling, EscalationBudget, parsed.Cause)
	}
	if parsed.Settled && decision != CirclingAccept {
		return "", "", false, true, fmt.Errorf("bouncer: circling decision %q cannot be settled; only an accept settles", parsed.Decision)
	}
	return decision, cause, parsed.Settled, true, nil
}

// settleCirclingAccept rewrites round's pending accept as settled.
// It errors when the decision is absent or is not an accept.
func settleCirclingAccept(runDir string, round int) error {
	decision, cause, _, exists, err := readCirclingDecision(runDir, round)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("shedadapters: no circling decision recorded for round %d", round)
	}
	if decision != CirclingAccept {
		return fmt.Errorf("shedadapters: round %d circling decision is %q; only an accept settles", round, decision)
	}
	content, err := renderCirclingDecision(round, decision, cause, true)
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
func renderCirclingDecision(round int, d CirclingDecision, cause EscalationCause, settled bool) ([]byte, error) {
	headerBytes, err := yaml.Marshal(circlingDecisionHeader{Round: round, Decision: string(d), Cause: string(cause), Settled: settled})
	if err != nil {
		return nil, fmt.Errorf("bouncer: render circling decision frontmatter: %w", err)
	}
	return append(append([]byte("---\n"), headerBytes...), []byte("---\n")...), nil
}
