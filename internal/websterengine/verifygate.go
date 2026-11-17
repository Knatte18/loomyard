// verifygate.go implements NewVerifyGate, the must-pass gate on Master's strand.
// It holds a webster run's done until the hub's `webster` gate passes on the committed tree.
// A failure returns to Master as findings; Master fixes it through a fixer fork, or the cold fixer when escalated.

package websterengine

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// verifyGateName names the gate entry Run adds to Master's spec; a `deps.Gate` entry may not carry this name.
const verifyGateName = "verify"

// verifyGateSite is the site label the gate's verify call writes to the running marker.
const verifyGateSite = "webster gate"

// verifyGateCrashFindingsPrefix opens the findings of a verify whose test runner crashed.
// verifyGateStuckReason reads that prefix to end the stuck reason in reentry alone.
const verifyGateCrashFindingsPrefix = "The verify command's test runner crashed"

// VerifyGateNotes collects what a gate's evaluations observed that is not a failure:
// the identities that failed once and passed on rerun, and the commits made above the pre-fix head.
// It is applied after the gate's wait, so a flaky pass reaches the warnings, summary.md and a friction note,
// and the fixer's commits reach summary.md.
// It also holds the fixer the latest failure went to, which the re-prompt line names.
type VerifyGateNotes struct {
	frictionDir string

	mu      sync.Mutex
	flaky   []string
	commits []CommitLogEntry
	fixer   string
}

// setFixer records fixer as the one the latest failure went to.
func (n *VerifyGateNotes) setFixer(fixer string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.fixer = fixer
}

// Fixer returns the fixer the latest failure went to, FixerFork before any failure.
func (n *VerifyGateNotes) Fixer() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.fixer == "" {
		return FixerFork
	}
	return n.fixer
}

// addCommits records entries, keeping each commit once by SHA in first-seen order.
func (n *VerifyGateNotes) addCommits(entries []CommitLogEntry) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, entry := range entries {
		found := false
		for _, have := range n.commits {
			found = found || have.SHA == entry.SHA
		}
		if !found {
			n.commits = append(n.commits, entry)
		}
	}
}

// FixCommits returns the recorded commits whose subject starts with `fix:` or that carry a departure, in first-seen order.
func (n *VerifyGateNotes) FixCommits() []CommitLogEntry {
	n.mu.Lock()
	defer n.mu.Unlock()
	var kept []CommitLogEntry
	for _, entry := range n.commits {
		if strings.HasPrefix(entry.Subject, "fix:") || len(entry.Departures) > 0 {
			kept = append(kept, entry)
		}
	}
	return kept
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

// Apply writes the flaky identities into the summary.md under websterDir and a friction note.
// It returns the warnings that name them.
// It writes the flaky identities only when some were flaky, and returns no warnings otherwise.
// It then appends the fixer's commits to summary.md.
// The gate's wait precedes this call, so the section lands after Master's last rewrite of summary.md.
// The friction note is best-effort: a failed note is logged and never fails the run.
func (n *VerifyGateNotes) Apply(websterDir string) ([]string, error) {
	var warnings []string
	if flaky := n.Flaky(); len(flaky) > 0 {
		if err := AppendIntegrationTriage(websterDir, flaky); err != nil {
			return nil, err
		}
		if err := writeTriageFrictionNote(n.frictionDir, flaky); err != nil {
			logger.Warn("websterengine: verify gate friction note not written", "cause", err)
		}
		warnings = triageWarnings(flaky)
	}
	if err := AppendVerifyGateFixCommits(websterDir, n.FixCommits()); err != nil {
		return nil, err
	}
	return warnings, nil
}

// AppendVerifyGateFixCommits appends a "Verify gate fix commits" section to the summary.md under websterDir:
// one bullet per `fix:` commit naming its SHA and subject;
// then one bullet per departure line of every commit in commits, a non-`fix:` commit's included.
// It appends nothing when no commit's subject starts with `fix:`.
func AppendVerifyGateFixCommits(websterDir string, commits []CommitLogEntry) error {
	var b strings.Builder
	var departures []string
	fixes := 0
	for _, commit := range commits {
		if strings.HasPrefix(commit.Subject, "fix:") {
			fixes++
			fmt.Fprintf(&b, "- `%s` %s\n", commit.SHA, commit.Subject)
		}
		departures = append(departures, commit.DepartureLines()...)
	}
	if fixes == 0 {
		return nil
	}
	for _, line := range departures {
		fmt.Fprintf(&b, "- %s\n", line)
	}
	section := "\n\n## Verify gate fix commits\n\nThe verify-gate fixers' commits above the pre-fix head, and the PATTERN departures their messages declare.\n\n" + b.String()
	return appendToSummary(websterDir, "verify gate fix commits", section)
}

// verifyGateSeams are the reads and runs the gate closure makes, injected so its logic is testable offline.
type verifyGateSeams struct {
	// outcome returns the outcome outcome.yaml names.
	outcome func() (string, error)
	// verifyCommand returns the hub's `webster` gate call over the whole tree, or the gate config's refusal.
	verifyCommand func() (string, error)
	// dirty returns the worktree's dirty paths.
	dirty func() ([]string, error)
	// head returns the worktree's HEAD commit.
	head func() (string, error)
	// fixRejection returns the first commit from base to HEAD that is not an acceptable fix commit, and why.
	fixRejection func(base string) (commit, reason string, err error)
	// commitsSince returns the commits from base to HEAD, oldest first.
	commitsSince func(base string) ([]string, error)
	// commitLog returns the non-merge first-parent commits from base to HEAD, oldest first, with their departures.
	commitLog func(base string) ([]CommitLogEntry, error)
	// verify runs the verify command through verifytree.
	verify func(site verifytree.Site, command string) (verifytree.Result, error)
	// readLog returns the content of the verify log at path.
	readLog func(path string) (string, error)
	// trail returns the card commit trail and its parallel NN-slug labels.
	trail func() (shas, labels []string, err error)
	// changedPaths returns the paths one commit changed.
	changedPaths func(sha string) ([]string, error)
	// modulePath returns go.mod's module path, empty when the worktree has none.
	modulePath func() string
	// recordPreFix persists head as state.json's PreFixHead.
	recordPreFix func(head string) error
	// clearPreFix clears state.json's PreFixHead.
	clearPreFix func() error
}

// NewVerifyGate returns the must-pass shuttleengine.Gate for Master's spec, and the notes to apply after the wait.
// geom is the told Geometry.
// attempts is the cap the entry's budget carries, shown in the findings.
// batches is the run's execution order, which orders the card hint.
// parentBranch lets a clean parent merge made while fixing pass the commit check.
// It is nil in standalone mode, where no merge is accepted.
// frictionDir is where the flaky note goes, empty when friction is off.
// fixPromptPath is the absolute path of the verify-fix prompt, which the findings name in their way forward.
//
// At each arrival the closure does, in order:
// it passes without verifying when outcome.yaml names an outcome other than done;
// it fails with the dirty paths when the tree is not clean;
// once a pre-fix head is recorded, it fails `Terminal` on a merge above it other than a clean parent merge;
// otherwise it records those commits in the notes;
// it renders the hub's `webster` gate call over the whole tree from the told gate config;
// it returns the config's refusal as its error, which names `lym config gate`;
// otherwise it runs that call through verifytree.Verify, told geom.VerifyHold on the first run and the rerun alike;
// on a failure it reruns once, and a failure whose rerun passes is flaky and passes the gate.
// A run that timed out or hung, as classifyHang reads its log and exit code, is not re-run: it fails at once.
// A run whose every failed tier run crashed its test runner with no failing test judged no code:
// it fails `Terminal` at once, on the first run or the rerun, naming the crashed runs,
// and no fixer gets it, no attempt is spent and no report is written.
// A failure that survives returns findings, and the first failure of any kind records HEAD as the pre-fix head.
// Each failure goes to a fixer, which the report, the findings and the re-prompt line name.
// A failure that reaches attempts goes to the cold fixer.
// So does one that follows a fixer which added no commit above the pre-fix head.
// Once a failure goes cold, every later failure does.
// Every other failure goes to the fixer fork.
// Every failed evaluation writes the verify-gate report.
// Attempt and failure counts, the routing and the pre-fix head live in the closure for one shuttle run.
// The pre-fix head is also persisted as state.json's PreFixHead when recorded, overwriting an earlier run's value.
// A passing evaluation clears it.
func NewVerifyGate(geom Geometry, attempts int, batches []batcher.Batch, parentBranch ParentBranchFunc, frictionDir, fixPromptPath string) (shuttleengine.Gate, *VerifyGateNotes) {
	paths := verifytree.NewPaths(geom.PaneCwd, geom.VerifyDir)
	seams := verifyGateSeams{
		outcome: func() (string, error) {
			o, err := parseOutcome(OutcomePath(geom.WebsterDir))
			if err != nil {
				return "", err
			}
			return o.Outcome, nil
		},
		verifyCommand: func() (string, error) {
			cfg, err := geom.GateConfig()
			if err != nil {
				return "", err
			}
			return cfg.GateCall("webster")
		},
		dirty: func() ([]string, error) { return geom.git().DirtyPaths(geom.WorktreeRoot) },
		head:  func() (string, error) { return geom.git().HeadSHA(geom.WorktreeRoot) },
		fixRejection: func(base string) (string, string, error) {
			return fixCommitRejection(geom.WorktreeRoot, base, parentBranch)
		},
		commitsSince: func(base string) ([]string, error) { return commitsSince(geom.WorktreeRoot, base) },
		commitLog: func(base string) ([]CommitLogEntry, error) {
			head, err := geom.git().HeadSHA(geom.WorktreeRoot)
			if err != nil {
				return nil, err
			}
			return geom.git().CommitLog(geom.WorktreeRoot, base, head)
		},
		verify: func(site verifytree.Site, command string) (verifytree.Result, error) {
			site.Hold = geom.VerifyHold
			return verifytree.Verify(context.Background(), paths, site, command)
		},
		readLog: func(path string) (string, error) {
			data, err := os.ReadFile(path)
			return string(data), err
		},
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
		recordPreFix: func(head string) error { return setPreFixHead(geom, head) },
		clearPreFix:  func() error { return setPreFixHead(geom, "") },
	}
	notes := &VerifyGateNotes{frictionDir: frictionDir}
	return newVerifyGate(geom.ReportsDir, attempts, notes, fixPromptPath, seams), notes
}

// setPreFixHead loads state.json under the state-mutation lease, sets PreFixHead and saves.
// Master is idle at a gate evaluation, so no bracket verb holds the lease then.
// A run with no state.json has nothing to record into and is left alone.
func setPreFixHead(geom Geometry, head string) error {
	lease, err := AcquireStateMutation(geom.ScratchDir)
	if err != nil {
		return err
	}
	defer func() { _ = lease.Release() }()
	st, err := LoadState(geom.WebsterDir, geom.ScratchDir)
	if err != nil {
		return err
	}
	if st == nil || st.PreFixHead == head {
		return nil
	}
	st.PreFixHead = head
	return SaveState(geom.WebsterDir, geom.ScratchDir, st)
}

// newVerifyGate builds the gate closure over injected seams, which make its logic testable offline.
func newVerifyGate(reportsDir string, attempts int, notes *VerifyGateNotes, fixPromptPath string, s verifyGateSeams) shuttleengine.Gate {
	attempt := 0
	preFix := ""
	reportPath := VerifyGateReportPath(reportsDir)
	failures := 0
	cold := false
	lastFixCommits := 0
	// command is the current arrival's verify command, empty until the arrival renders it.
	command := ""

	// fail records the pre-fix head on the first failure.
	// It routes the failure to a fixer, writes the report naming the arrival's verify command, and returns its findings.
	fail := func(report VerifyGateReport) (shuttleengine.GateResult, error) {
		failures++
		report.Command = command
		if preFix == "" {
			head, err := s.head()
			if err != nil {
				return shuttleengine.GateResult{}, err
			}
			if err := s.recordPreFix(head); err != nil {
				return shuttleengine.GateResult{}, err
			}
			preFix = head
		}
		fixCommits, err := s.commitsSince(preFix)
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		// A failure that reaches attempts goes to the cold fixer.
		// So does one after a fixer that added no commit above the pre-fix head.
		// Once a failure goes cold, every later one does.
		cold = cold || failures >= attempts || (failures > 1 && len(fixCommits) == lastFixCommits)
		lastFixCommits = len(fixCommits)
		report.Fixer = FixerFork
		if cold {
			report.Fixer = FixerCold
		}
		notes.setFixer(report.Fixer)
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
		return shuttleengine.GateResult{Passed: false, Findings: renderVerifyGateFindings(report, fixPromptPath)}, nil
	}

	// pass clears the persisted pre-fix head and returns a passing result.
	pass := func() (shuttleengine.GateResult, error) {
		if err := s.clearPreFix(); err != nil {
			return shuttleengine.GateResult{}, err
		}
		return shuttleengine.GateResult{Passed: true}, nil
	}

	// readOutput returns the verify log of res.
	readOutput := func(res verifytree.Result) (string, error) {
		output, err := s.readLog(res.Log)
		if err != nil {
			return "", fmt.Errorf("websterengine: read verify log %s: %w", res.Log, err)
		}
		return output, nil
	}

	// failTimedOut fails the gate for res, a verify that outlived its timeout.
	// A hang is not flakiness, so it is never rerun.
	failTimedOut := func(res verifytree.Result) (shuttleengine.GateResult, error) {
		output, err := readOutput(res)
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		return fail(VerifyGateReport{TimedOut: res.Detail, LogPath: res.Log, LogTail: logTail(output)})
	}

	// failHung fails the gate for res, a failed verify whose output classifyHang read as a hang.
	// A hang is not flakiness, so it is never rerun.
	failHung := func(res verifytree.Result, output string, hang verifyHang) (shuttleengine.GateResult, error) {
		return fail(VerifyGateReport{
			Failures:       parseVerifyFailures(output, false),
			LogPath:        res.Log,
			LogTail:        logTail(output),
			Hang:           hang.Matched,
			HangStacks:     hang.Stacks,
			WatchdogWindow: hang.Window,
		})
	}

	// failRunnerCrash fails the gate terminally for res, a verify whose failed tier runs all crashed the test runner:
	// No failing test judged the code, and no change to the code mends it, so no fixer gets it and it is never rerun.
	failRunnerCrash := func(res verifytree.Result) shuttleengine.GateResult {
		logger.Warn("websterengine: verify gate's test runner crashed, so no fixer runs", "attempt", attempt, "detail", res.Detail, "log", res.Log)
		findings := fmt.Sprintf("%s in its run and its one rerun, so no test judged the code: %s. No change to the code mends it, so no fixer runs and the run stops here; output: %s.", verifyGateCrashFindingsPrefix, res.Detail, res.Log)
		return shuttleengine.GateResult{Passed: false, Terminal: true, Findings: findings}
	}

	return func() (shuttleengine.GateResult, error) {
		attempt++
		command = ""

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
				findings := fmt.Sprintf("Commit %s, made above the pre-fix head %s, is neither a non-merge commit nor a clean merge of the parent branch: %s.", commit, preFix, reason)
				return shuttleengine.GateResult{Passed: false, Terminal: true, Findings: findings}, nil
			}
			entries, err := s.commitLog(preFix)
			if err != nil {
				return shuttleengine.GateResult{}, err
			}
			notes.addCommits(entries)
		}

		command, err = s.verifyCommand()
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("websterengine: verify gate: %w", err)
		}

		site := verifytree.Site{Label: verifyGateSite, Attempt: attempt}
		res, err := s.verify(site, command)
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("websterengine: verify gate: %w", err)
		}
		switch res.Status {
		case verifytree.StatusPassed, verifytree.StatusSkipped:
			return pass()
		case verifytree.StatusDirty:
			return fail(VerifyGateReport{Dirty: res.Dirty})
		}
		if res.TimedOut {
			return failTimedOut(res)
		}
		if res.RunnerCrash {
			return failRunnerCrash(res), nil
		}

		firstOutput, err := readOutput(res)
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if hang, hung := classifyHang(res, firstOutput); hung {
			return failHung(res, firstOutput, hang)
		}
		first := parseVerifyFailures(firstOutput, false)
		res, err = s.verify(site, command)
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("websterengine: verify gate: %w", err)
		}
		switch res.Status {
		case verifytree.StatusPassed, verifytree.StatusSkipped:
			notes.addFlaky(failureIDs(first))
			logger.Warn("websterengine: verify gate failures passed on rerun", "flaky", strings.Join(failureIDs(first), ", "))
			return pass()
		case verifytree.StatusDirty:
			return fail(VerifyGateReport{Dirty: res.Dirty})
		}
		if res.TimedOut {
			return failTimedOut(res)
		}
		if res.RunnerCrash {
			return failRunnerCrash(res), nil
		}
		rerunOutput, err := readOutput(res)
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if hang, hung := classifyHang(res, rerunOutput); hung {
			return failHung(res, rerunOutput, hang)
		}
		report := VerifyGateReport{Failures: parseVerifyFailures(rerunOutput, false), LogPath: res.Log, ExitCode: res.ExitCode}
		if len(report.Failures) == 1 && report.Failures[0].Kind == FailureKindOpaque {
			// A failure that names no test, such as a build error or the gate tool's own error, is shown by its output alone.
			report.Failures = nil
			report.LogTail = logTail(rerunOutput)
		}
		return fail(report)
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

// verifyGateStuckReason is the one-line stuck reason of a done run whose verify gate did not pass.
// A Terminal failure, the rejection of a fixer commit, names the gate's own reason.
// It ends in the reset to the pre-fix head, then reentry.
// A Terminal runner crash names the gate's own reason and ends in reentry alone, since no fixer commit exists to reset.
// Otherwise it opens with the re-prompts the failing entry sent against its limit.
// When the run ended with re-prompts left, it says why.
// It then names the verify command and what failed, and ends in a way forward.
func verifyGateStuckReason(reportsDir string, gate *shuttleengine.GateOutcome, reentry string) string {
	if strings.HasPrefix(gate.Reason, verifyGateCrashFindingsPrefix) {
		return "verify gate failed: " + oneLine(gate.Reason) + "; " + wayForwardSteps(reentry)
	}
	if gate.Reason != "" {
		return "verify gate failed: " + oneLine(gate.Reason) + "; " + wayForwardSteps("lym webster reset --to pre-fix", reentry)
	}
	report, err := readVerifyGateReport(VerifyGateReportPath(reportsDir))
	if err != nil {
		return fmt.Sprintf("verify gate did not pass after %d re-prompt(s); its report could not be read: %v; %s", failedEntryReprompts(gate), err, wayForwardSteps(reentry))
	}
	return verifyGateStuckLead(gate, report.Cap) + ": " + verifyGateStuckFailure(report, reentry)
}

// verifyGateStuckLead opens the stuck reason of a verify gate that did not pass with no terminal reason.
// It counts the failing entry's re-prompts against limit, the entry's re-prompt budget.
// When the run ended with re-prompts left, it says why.
func verifyGateStuckLead(gate *shuttleengine.GateOutcome, limit int) string {
	sent := failedEntryReprompts(gate)
	switch {
	case gate.EndedEarly == "":
		return fmt.Sprintf("verify gate did not pass after %d of %d re-prompt(s)", sent, limit)
	case sent == 0:
		return fmt.Sprintf("verify gate failed its first check and the session was never re-prompted, since %s", gate.EndedEarly)
	default:
		return fmt.Sprintf("verify gate did not pass after %d of %d re-prompt(s) and the run ended with re-prompts left, since %s", sent, limit, gate.EndedEarly)
	}
}

// failedEntryReprompts returns the re-prompts the failed gate entry sent.
// It returns every entry's re-prompts when none is marked failed.
func failedEntryReprompts(gate *shuttleengine.GateOutcome) int {
	for _, entry := range gate.Entries {
		if entry.State == shuttleengine.GateEntryFailed {
			return entry.Attempts
		}
	}
	return gate.Attempts
}

// verifyGateStuckTextBytes bounds each piece of gate output a stuck reason quotes, so the one-line reason stays readable.
const verifyGateStuckTextBytes = 400

// verifyGateStuckFailure names what the report's failure was and ends in its way forward.
// A failed verify names its command, then the hang, the failing identities, the dirty paths or the timeout.
// A failure that named no identity says so.
// It quotes the tail of the command's output, or says the output was empty, since the gate itself may have erred.
// A run-level entry's failure names the entry and opens its findings.
// After a CPU watchdog quit, the hang's own way forward replaces the general one.
func verifyGateStuckFailure(report VerifyGateReport, reentry string) string {
	fixStep := fmt.Sprintf("read %s and commit a fix on the task branch", report.LogPath)
	var what string
	switch {
	case report.Entry != "":
		what = fmt.Sprintf("the run-level %q entry failed: %s", report.Entry, oneLine(cutHead(report.Findings, verifyGateStuckTextBytes)))
		fixStep = fmt.Sprintf("fix what the %q entry's findings name and commit it on the task branch", report.Entry)
	case len(report.Dirty) > 0:
		what = "the worktree held uncommitted paths " + strings.Join(report.Dirty, ", ")
		fixStep = "commit or remove those paths on the task branch"
	case report.Hang != "":
		what = hangStuckReason(report, reentry)
		if report.WatchdogWindow != "" {
			return withCommand(report.Command, what)
		}
	case report.TimedOut != "":
		what = fmt.Sprintf("the verify command timed out (%s) and was not re-run, see %s", report.TimedOut, report.LogPath)
	case len(report.Failures) > 0:
		what = fmt.Sprintf("failing %s, see %s", strings.Join(failureIDs(report.Failures), ", "), report.LogPath)
	default:
		output := "its output was empty"
		if report.LogTail != "" {
			output = "output tail: " + oneLine(cutTail(report.LogTail, verifyGateStuckTextBytes))
		}
		what = fmt.Sprintf("the verify command exited with code %d and its output named no failing test, so the gate itself may have erred; %s; see %s",
			report.ExitCode, output, report.LogPath)
	}
	return withCommand(report.Command, what) + "; " + wayForwardSteps(fixStep, reentry)
}

// withCommand opens what with the verify command it failed under, and returns what alone when command is empty.
func withCommand(command, what string) string {
	if command == "" {
		return what
	}
	return fmt.Sprintf("`%s`: %s", command, what)
}

// cutHead returns s cut to its first limit bytes on a rune boundary.
func cutHead(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	for limit > 0 && !utf8.RuneStart(s[limit]) {
		limit--
	}
	return s[:limit]
}

// cutTail returns s cut to its last limit bytes on a rune boundary.
func cutTail(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	start := len(s) - limit
	for start < len(s) && !utf8.RuneStart(s[start]) {
		start++
	}
	return s[start:]
}

// hangStuckReason describes a hang report for the stuck reason.
// It names the hang and that the command was not re-run.
// After a CPU watchdog quit it also names the window, the way forward and reentry.
func hangStuckReason(report VerifyGateReport, reentry string) string {
	what := fmt.Sprintf("the verify command was killed or exited with %s and was not re-run, see %s", report.Hang, report.LogPath)
	if report.HangStacks {
		what = fmt.Sprintf("the verify command hung (%s) and was not re-run; its goroutine stacks are in %s", report.Hang, report.LogPath)
	}
	if report.WatchdogWindow != "" {
		what += fmt.Sprintf("; the gate test's CPU watchdog quit a test binary busy for %s: if that package is CPU-bound but healthy, %s, then %s", report.WatchdogWindow, gateslot.BusyQuitWayForward, reentry)
	}
	return what
}

// oneLine collapses the line breaks of s into spaces, so a multi-line reason fits a one-line outcome.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// readModulePath returns the module path that go.mod in worktree declares.
// It returns empty when there is no go.mod or it declares no module.
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
