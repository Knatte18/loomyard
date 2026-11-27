// circling.go defines the operator's recorded decision on an escalated round, kept in a small decision file.
// The `lym loom circling` verbs write the file, and the Bouncer reads it.
// The writer checks the mechanical precondition: the latest round is escalated,
// and no decision exists for it yet.
// The run-state check, that the run is awaiting at a Bouncer row, belongs to the verbs.
// A continue may rule decision-class findings in scope, which the next round's fixer takes.

package shedadapters

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/burlerengine"
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
	// ErrInScopeCause reports an in-scope ruling on a round whose escalation cause is not decision.
	ErrInScopeCause = errors.New("shedadapters: an in-scope ruling answers only a decision escalation")
	// ErrInScopeFinding reports an in-scope ruling naming an id that is no decision-class finding of the round's review.
	ErrInScopeFinding = errors.New("shedadapters: an in-scope ruling names no decision-class finding of the round's review")
)

// circlingDecisionHeader mirrors a circling decision file's YAML frontmatter.
// Automatic is true for a decision the Bouncer recorded itself:
// an auto-accept, or the soft-cap rule's accept or continue.
// A parent's decision never sets it.
// InScope lists the decision-class findings a continue rules in scope, which the next round's fixer takes.
type circlingDecisionHeader struct {
	Round     int      `yaml:"round"`
	Decision  string   `yaml:"decision"`
	Cause     string   `yaml:"cause"`
	Settled   bool     `yaml:"settled"`
	Automatic bool     `yaml:"automatic"`
	InScope   []string `yaml:"in_scope,omitempty"`
}

// LatestRound returns the run directory's latest round, 0 when it holds none.
// The latest round is the one a circling decision answers.
func LatestRound(runDir string) (int, error) {
	return ResolveRound(runDir, func(n int) string { return filepath.Base(roundReviewPath(runDir, n)) })
}

// RecordCirclingDecision records d as pending for the run directory's latest round.
// On success it returns that round and its escalation cause.
// It returns ErrNotEscalated unless that round carries an escalation record.
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
		return 0, "", ErrNotEscalated
	}

	if err := createCirclingDecision(runDir, round, d, cause, false, nil); err != nil {
		return 0, "", err
	}
	return round, cause, nil
}

// RecordInScopeContinue records a pending continue for the run directory's latest round that rules ids in scope.
// Each id is a decision-class finding of that round's review the task's record already asks for,
// so the next round carries it as fixer work instead of a question for the parent.
// On success it returns that round.
// It returns ErrNotEscalated unless that round carries an escalation record.
// It returns ErrInScopeCause unless that record's cause is decision.
// It returns ErrInScopeFinding, naming the ids, when an id is no decision-class finding of the round's review.
// It returns ErrCirclingDecided when a decision file for the round already exists, leaving it untouched.
func RecordInScopeContinue(runDir string, ids []string) (int, error) {
	round, err := LatestRound(runDir)
	if err != nil {
		return 0, err
	}
	if round == 0 {
		return 0, ErrNotEscalated
	}
	cause, _, escalated, err := readEscalation(runDir, round)
	if err != nil {
		return 0, err
	}
	if !escalated {
		return 0, ErrNotEscalated
	}
	if cause != EscalationDecision {
		return 0, fmt.Errorf("%w: round %d's escalation cause is %s", ErrInScopeCause, round, cause)
	}
	raw, err := os.ReadFile(roundReviewPath(runDir, round))
	if err != nil {
		return 0, fmt.Errorf("shedadapters: read round %d's review: %w", round, err)
	}
	_, findings, err := burlerengine.ParseReview(raw)
	if err != nil {
		return 0, fmt.Errorf("shedadapters: parse round %d's review: %w", round, err)
	}
	var decisions, unknown []string
	for _, finding := range findings {
		if finding.Class == burlerengine.ClassDecision {
			decisions = append(decisions, finding.ID)
		}
	}
	for _, id := range ids {
		if !slices.Contains(decisions, id) {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) > 0 {
		return 0, fmt.Errorf("%w: %s, while round %d's decision-class findings are %s", ErrInScopeFinding, strings.Join(unknown, ", "), round, cmp.Or(strings.Join(decisions, ", "), "none"))
	}
	if err := createCirclingDecision(runDir, round, CirclingContinue, cause, false, ids); err != nil {
		return 0, err
	}
	return round, nil
}

// InScopeFindings returns the ids round's circling decision rules in scope, none when the round holds no such ruling.
// A malformed decision file is an error.
func InScopeFindings(runDir string, round int) ([]string, error) {
	path := circlingDecisionPath(runDir, round)
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("shedadapters: read circling decision %s: %w", path, err)
	}
	header, err := splitFrontmatter(raw, "circling decision")
	if err != nil {
		return nil, err
	}
	var parsed circlingDecisionHeader
	if err := yaml.Unmarshal([]byte(header), &parsed); err != nil {
		return nil, fmt.Errorf("shedadapters: circling decision %s frontmatter is not valid YAML: %w", path, err)
	}
	return parsed.InScope, nil
}

// recordAutoAccept records round's circling decision as an automatic, not yet settled accept with cause budget.
// It serves the automatic accepts recorded at the soft cap.
// It returns ErrCirclingDecided when a decision file for the round already exists, leaving it untouched.
func recordAutoAccept(runDir string, round int) error {
	return createCirclingDecision(runDir, round, CirclingAccept, EscalationBudget, true, nil)
}

// recordAutoContinue records round's circling decision as an automatic, not yet settled continue with cause budget.
// It returns ErrCirclingDecided when a decision file for the round already exists, leaving it untouched.
func recordAutoContinue(runDir string, round int) error {
	return createCirclingDecision(runDir, round, CirclingContinue, EscalationBudget, true, nil)
}

// createCirclingDecision writes round's pending decision file, refusing with ErrCirclingDecided to overwrite one.
// inScope lists the findings a continue rules in scope, none for any other decision.
func createCirclingDecision(runDir string, round int, d CirclingDecision, cause EscalationCause, automatic bool, inScope []string) error {
	path := circlingDecisionPath(runDir, round)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return ErrCirclingDecided
		}
		return fmt.Errorf("shedadapters: create circling decision %s: %w", path, err)
	}
	content, err := renderCirclingDecision(round, d, cause, false, automatic, inScope)
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

// PendingCirclingDecision reports whether the run directory's latest round carries an unsettled circling decision file.
// A run directory with no round has no pending decision.
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

// PendingEscalation reports whether the run directory's latest round is escalated and still awaits a circling decision.
// When pending, it returns that round and the path of its escalation brief.
// A run directory with no round is not pending.
// Nor is a round with no escalation record, or one whose decision file exists, settled or not.
// A malformed escalation record is returned as its error, wrapping ErrEscalationMalformed.
func PendingEscalation(runDir string) (round int, briefPath string, pending bool, err error) {
	round, _, briefPath, pending, err = pendingEscalation(runDir)
	return round, briefPath, pending, err
}

// PendingEscalationCause returns the cause and brief path of a pending escalation, and whether one is pending.
// The cause is empty where nothing is pending.
func PendingEscalationCause(runDir string) (cause EscalationCause, briefPath string, pending bool, err error) {
	_, cause, briefPath, pending, err = pendingEscalation(runDir)
	return cause, briefPath, pending, err
}

// pendingEscalation reads the pending escalation of runDir's latest round, returning its round, cause and brief path.
func pendingEscalation(runDir string) (round int, cause EscalationCause, briefPath string, pending bool, err error) {
	round, err = ResolveRound(runDir, func(n int) string { return filepath.Base(roundReviewPath(runDir, n)) })
	if err != nil || round == 0 {
		return 0, "", "", false, err
	}
	cause, _, escalated, err := readEscalation(runDir, round)
	if err != nil || !escalated {
		return 0, "", "", false, err
	}
	_, _, _, decided, err := readCirclingDecision(runDir, round)
	if err != nil {
		if decided {
			return 0, "", "", false, fmt.Errorf("shedadapters: circling decision %s: %w", circlingDecisionPath(runDir, round), err)
		}
		return 0, "", "", false, err
	}
	if decided {
		return 0, "", "", false, nil
	}
	return round, cause, escalationPath(runDir, round), true, nil
}

// readCirclingDecision reads round's decision file.
// It returns the decision, its escalation cause, whether it is settled, and whether a file exists.
// A present file that is malformed is an error:
// the round must match the filename;
// the decision and the cause must each be one of their values, the five causes for the cause;
// settled is legal only on an accept.
func readCirclingDecision(runDir string, round int) (CirclingDecision, EscalationCause, bool, bool, error) {
	decision, cause, settled, _, exists, err := readCirclingFile(runDir, round)
	return decision, cause, settled, exists, err
}

// readCirclingFile reads round's decision file, returning its decision, escalation cause and whether it is settled.
// It also reports whether the decision was recorded automatically, and whether a file exists.
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
	if !cause.valid() {
		return "", "", false, false, true, fmt.Errorf("bouncer: circling decision cause must be %q, %q, %q, %q or %q, got %q", EscalationCircling, EscalationBudget, EscalationHardCap, EscalationDecision, EscalationStalemate, parsed.Cause)
	}
	if parsed.Settled && decision != CirclingAccept {
		return "", "", false, false, true, fmt.Errorf("bouncer: circling decision %q cannot be settled; only an accept settles", parsed.Decision)
	}
	return decision, cause, parsed.Settled, parsed.Automatic, true, nil
}

// settleCirclingAccept rewrites round's pending accept as settled.
// It returns an error when the decision is absent or is not an accept,
// and it keeps the record's automatic value.
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
	content, err := renderCirclingDecision(round, decision, cause, true, automatic, nil)
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
func renderCirclingDecision(round int, d CirclingDecision, cause EscalationCause, settled, automatic bool, inScope []string) ([]byte, error) {
	headerBytes, err := yaml.Marshal(circlingDecisionHeader{Round: round, Decision: string(d), Cause: string(cause), Settled: settled, Automatic: automatic, InScope: inScope})
	if err != nil {
		return nil, fmt.Errorf("bouncer: render circling decision frontmatter: %w", err)
	}
	return append(append([]byte("---\n"), headerBytes...), []byte("---\n")...), nil
}
