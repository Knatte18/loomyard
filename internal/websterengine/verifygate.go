// verifygate.go implements NewVerifyGate, the must-pass gate on Merriam's strand that holds a webster run's done until the plan's verify command passes on the committed tree.
// A failure goes back to Merriam's live session as findings, and Merriam fixes it through one fixer fork.

package websterengine

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// verifyGateName is the name of the gate entry Run adds to Merriam's spec, and the name a `deps.Gate` entry may not carry.
const verifyGateName = "verify"

// verifyGateSite is the site label the gate's verify call writes to the running marker.
const verifyGateSite = "webster gate"

// VerifyGateNotes collects what a gate's evaluations observed that is not a failure: the identities that failed once and passed on rerun.
// Run applies it after the wait, so a flaky pass reaches the warnings, summary.md and a friction note.
type VerifyGateNotes struct {
	frictionDir string

	mu    sync.Mutex
	flaky []string
}

// addFlaky records ids as flaky, skipping any already recorded.
func (n *VerifyGateNotes) addFlaky(ids []string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, id := range ids {
		found := false
		for _, have := range n.flaky {
			found = found || have == id
		}
		if !found {
			n.flaky = append(n.flaky, id)
		}
	}
}

// Flaky returns the identities recorded as flaky, in first-seen order.
func (n *VerifyGateNotes) Flaky() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.flaky...)
}

// Apply writes the flaky identities into the summary.md under websterDir and a friction note, and returns the warnings that name them.
// It does nothing, and returns no warnings, when no identity was flaky.
// The friction note is best-effort, like the rest of the friction plumbing: a failed note is logged and never fails the run.
func (n *VerifyGateNotes) Apply(websterDir string) ([]string, error) {
	flaky := n.Flaky()
	if len(flaky) == 0 {
		return nil, nil
	}
	if err := AppendIntegrationTriage(websterDir, flaky); err != nil {
		return nil, err
	}
	if err := writeTriageFrictionNote(n.frictionDir, flaky); err != nil {
		logger.Warn("websterengine: verify gate friction note not written", "cause", err)
	}
	return triageWarnings(flaky), nil
}

// verifyGateSeams are the reads and runs the gate closure makes, injectable so the closure's own logic is testable offline.
type verifyGateSeams struct {
	// outcome returns the outcome outcome.yaml names.
	outcome func() (string, error)
	// verifyCommand returns the plan's `## verify:` command, empty when the plan has none.
	verifyCommand func() (string, error)
	// dirty returns the worktree's dirty paths.
	dirty func() ([]string, error)
	// head returns the worktree's HEAD commit.
	head func() (string, error)
	// fixRejection returns the first commit from base to HEAD that is not an acceptable fix commit, and why.
	fixRejection func(base string) (commit, reason string, err error)
	// commitsSince returns the commits from base to HEAD, oldest first.
	commitsSince func(base string) ([]string, error)
	// verify runs the verify command through verifytree.
	verify func(site verifytree.Site, command string) (verifytree.Result, error)
	// readLog returns the verify log of the latest verify call.
	readLog func() (string, error)
	// logPath is the verify log's path.
	logPath string
	// trail returns the card commit trail and its parallel NN-slug labels.
	trail func() (shas, labels []string, err error)
	// changedPaths returns the paths one commit changed.
	changedPaths func(sha string) ([]string, error)
	// modulePath returns go.mod's module path, empty when the worktree has none.
	modulePath func() string
}

// NewVerifyGate returns the must-pass shuttleengine.Gate Run adds to Merriam's spec, with the notes Run applies after the wait.
// geom is the told Geometry; attempts is the cap the entry's budget carries, shown in the findings; batches is the run's execution order, which orders the card hint.
// parentBranch lets a clean parent merge made while fixing pass the commit check, and is nil in standalone mode, where no merge is accepted.
// frictionDir is where the flaky note goes, empty when friction is off.
//
// At each arrival the closure does, in order:
// it passes without verifying when outcome.yaml names an outcome other than done;
// it fails with the dirty paths when the tree is not clean;
// once a pre-fix head is recorded, it fails `Terminal` when a commit above it is neither a non-merge commit nor a clean parent merge;
// it passes with a warning when the plan has no `## verify:` section;
// otherwise it runs verifytree.Verify, reruns once on a failure, and passes a flaky failure whose rerun passed.
// A failure that survives returns findings, and the first failure of any kind records HEAD as the pre-fix head.
// Every failed evaluation writes the verify-gate report.
// Attempt counts and the pre-fix head live in the closure for one shuttle run.
func NewVerifyGate(geom Geometry, attempts int, batches []batcher.Batch, parentBranch ParentBranchFunc, frictionDir string) (shuttleengine.Gate, *VerifyGateNotes) {
	paths := verifytree.NewPaths(geom.WorktreeRoot, geom.VerifyDir)
	seams := verifyGateSeams{
		outcome: func() (string, error) {
			o, err := parseOutcome(OutcomePath(geom.WebsterDir))
			if err != nil {
				return "", err
			}
			return o.Outcome, nil
		},
		verifyCommand: func() (string, error) {
			plan, err := planparser.ParsePlan(geom.PlanDir)
			if err != nil {
				return "", err
			}
			return plan.Verify, nil
		},
		dirty: func() ([]string, error) { return verifytree.DirtyPaths(geom.WorktreeRoot) },
		head:  func() (string, error) { return headSHA(geom.WorktreeRoot) },
		fixRejection: func(base string) (string, string, error) {
			return fixCommitRejection(geom.WorktreeRoot, base, parentBranch)
		},
		commitsSince: func(base string) ([]string, error) { return commitsSince(geom.WorktreeRoot, base) },
		verify: func(site verifytree.Site, command string) (verifytree.Result, error) {
			return verifytree.Verify(context.Background(), paths, site, command)
		},
		readLog: func() (string, error) {
			data, err := os.ReadFile(paths.Log)
			return string(data), err
		},
		logPath: paths.Log,
		trail: func() ([]string, []string, error) {
			st, err := LoadState(geom.WebsterDir, geom.ScratchDir)
			if err != nil || st == nil {
				return nil, nil, err
			}
			shas, labels := accumulatedCardSHAs(batches, st)
			return shas, labels, nil
		},
		changedPaths: func(sha string) ([]string, error) { return commitChangedPaths(geom.WorktreeRoot, sha) },
		modulePath:   func() string { return readModulePath(geom.WorktreeRoot) },
	}
	notes := &VerifyGateNotes{frictionDir: frictionDir}
	return newVerifyGate(geom.ReportsDir, attempts, notes, seams), notes
}

// newVerifyGate is NewVerifyGate over injected seams.
func newVerifyGate(reportsDir string, attempts int, notes *VerifyGateNotes, s verifyGateSeams) shuttleengine.Gate {
	attempt := 0
	preFix := ""
	reportPath := VerifyGateReportPath(reportsDir)

	// fail records the pre-fix head on the first failure, then writes the report and returns its findings.
	fail := func(report VerifyGateReport) (shuttleengine.GateResult, error) {
		if preFix == "" {
			head, err := s.head()
			if err != nil {
				return shuttleengine.GateResult{}, err
			}
			preFix = head
		}
		fixCommits, err := s.commitsSince(preFix)
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		report.Attempt = attempt
		report.Cap = attempts
		report.FixCommits = fixCommits
		if len(report.Failures) > 0 {
			shas, labels, err := s.trail()
			if err != nil {
				return shuttleengine.GateResult{}, err
			}
			report.Hint, err = cardHint(s.modulePath(), report.Failures, shas, labels, s.changedPaths)
			if err != nil {
				return shuttleengine.GateResult{}, err
			}
		}
		if err := WriteVerifyGateReport(reportPath, report); err != nil {
			return shuttleengine.GateResult{}, err
		}
		logger.Warn("websterengine: verify gate failed", "attempt", attempt, "cap", attempts, "dirty", len(report.Dirty), "failures", len(report.Failures))
		return shuttleengine.GateResult{Passed: false, Findings: renderVerifyGateFindings(report)}, nil
	}

	// failures parses the latest verify log into failing identities.
	failures := func() ([]VerifyFailure, error) {
		output, err := s.readLog()
		if err != nil {
			return nil, fmt.Errorf("websterengine: read verify log %s: %w", s.logPath, err)
		}
		return parseVerifyFailures(output, false), nil
	}

	return func() (shuttleengine.GateResult, error) {
		attempt++

		outcome, err := s.outcome()
		if err != nil {
			// An unreadable outcome file is mapMasterDone's to report, not a verify failure to spend an attempt on.
			logger.Warn("websterengine: verify gate could not read outcome.yaml, passing without verifying", "cause", err)
			return shuttleengine.GateResult{Passed: true}, nil
		}
		if outcome != outcomeDone {
			return shuttleengine.GateResult{Passed: true}, nil
		}

		dirty, err := s.dirty()
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if len(dirty) > 0 {
			return fail(VerifyGateReport{Dirty: dirty})
		}

		if preFix != "" {
			commit, reason, err := s.fixRejection(preFix)
			if err != nil {
				return shuttleengine.GateResult{}, err
			}
			if commit != "" {
				findings := fmt.Sprintf("Commit %s, made above the pre-fix head %s, is neither a non-merge commit nor a clean merge of the parent branch: %s.\nMove HEAD back to %s and fix with plain commits.", commit, preFix, reason, preFix)
				return shuttleengine.GateResult{Passed: false, Terminal: true, Findings: findings}, nil
			}
		}

		command, err := s.verifyCommand()
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("websterengine: verify gate: read the plan: %w", err)
		}
		if command == "" {
			logger.Warn("websterengine: verify gate found no verify section in the plan")
			return shuttleengine.GateResult{Passed: true}, nil
		}

		site := verifytree.Site{Label: verifyGateSite, Attempt: attempt}
		res, err := s.verify(site, command)
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("websterengine: verify gate: %w", err)
		}
		switch res.Status {
		case verifytree.StatusPassed, verifytree.StatusSkipped:
			return shuttleengine.GateResult{Passed: true}, nil
		case verifytree.StatusDirty:
			return fail(VerifyGateReport{Dirty: res.Dirty})
		}

		first, err := failures()
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		res, err = s.verify(site, command)
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("websterengine: verify gate: %w", err)
		}
		switch res.Status {
		case verifytree.StatusPassed, verifytree.StatusSkipped:
			notes.addFlaky(failureIDs(first))
			logger.Warn("websterengine: verify gate failures passed on rerun", "flaky", strings.Join(failureIDs(first), ", "))
			return shuttleengine.GateResult{Passed: true}, nil
		case verifytree.StatusDirty:
			return fail(VerifyGateReport{Dirty: res.Dirty})
		}
		surviving, err := failures()
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		return fail(VerifyGateReport{Failures: surviving, LogPath: s.logPath})
	}
}

// readVerifyGateReport reads the verify-gate report at path.
func readVerifyGateReport(path string) (VerifyGateReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return VerifyGateReport{}, fmt.Errorf("websterengine: read verify-gate report %s: %w", path, err)
	}
	var r VerifyGateReport
	if err := yaml.Unmarshal(data, &r); err != nil {
		return VerifyGateReport{}, fmt.Errorf("websterengine: verify-gate report %s: %w", path, err)
	}
	return r, nil
}

// verifyGateStuckReason is the stuck reason of a done run whose verify gate did not pass.
// A Terminal failure names the gate's own reason.
// Otherwise it names the failing identities, or the dirty paths, the latest failed evaluation's report holds, and the attempts spent.
func verifyGateStuckReason(reportsDir string, gate *shuttleengine.GateOutcome) string {
	if gate.Reason != "" {
		return "verify gate failed: " + oneLine(gate.Reason)
	}
	report, err := readVerifyGateReport(VerifyGateReportPath(reportsDir))
	if err != nil {
		return fmt.Sprintf("verify gate failed after %d re-prompt(s); its report could not be read: %v", gate.Attempts, err)
	}
	what := "none named"
	switch {
	case len(report.Failures) > 0:
		what = strings.Join(failureIDs(report.Failures), ", ")
	case len(report.Dirty) > 0:
		what = "uncommitted paths " + strings.Join(report.Dirty, ", ")
	}
	return fmt.Sprintf("verify gate failed after %d attempt(s) of %d: %s", report.Attempt, report.Cap, what)
}

// oneLine collapses s's line breaks into spaces, for a reason that rides a one-line outcome.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// readModulePath returns the module path go.mod in worktree declares, or empty when there is no go.mod or it declares none.
func readModulePath(worktree string) string {
	f, err := os.Open(filepath.Join(worktree, "go.mod"))
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(scanner.Text()), "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`)
		}
	}
	return ""
}
