// check.go implements the hidden `lym adopt check <subject>` verb and the one-subject check it and the adopt run.
// runCheck runs the hub's adopt gate in-process in the subject's worktree.
// It keeps the progress in the subject's check.json and posts each tier's notice to its nearest live orch;
// the check's end is left to runHoldCheck.

package adoptcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gatecli"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/state"
)

const (
	// checkRecordName is the name of the file that holds the adopt check's progress record.
	checkRecordName = "check.json"
	// checkOutName is the file the spawned check's stdout and stderr go to,
	// so the envelope of a check that loses the lock and a panic stay on disk.
	checkOutName = "check.out"
	// checkLockName is the lock one running check holds.
	checkLockName = "check.lock"
	// checkGateKey is the gate.yaml key the check runs.
	checkGateKey = "adopt"
	// checkProgram signs the check's notices to the orch.
	checkProgram = "ADOPT"
	// maxListedFailures caps the failed tests one tier's notice lists; suite.log and check.json hold the rest.
	maxListedFailures = 10
)

// The states of an adopt check's progress record.
const (
	checkRunning = "running"
	checkGreen   = "green"
	checkFailed  = "failed"
)

// CheckRecord is the progress of one subject's adopt check, kept in that subject's check.json.
// Unknown keys are ignored on read, so a record from a newer writer never fails an older reader.
type CheckRecord struct {
	// Subject is the beam slug or `root` the check runs for.
	Subject string `json:"subject"`
	// PID and StartTicks name the writing process; StartTicks is its start time, empty where the platform cannot read it.
	PID        int       `json:"pid"`
	StartTicks string    `json:"start_ticks,omitempty"`
	Start      time.Time `json:"start"`
	// Gate is the gate.yaml key the check runs.
	Gate string `json:"gate"`
	// Tiers holds each tier as it ended, in order.
	Tiers []CheckTier `json:"tiers,omitempty"`
	// Running is the tier in flight, nil between tiers.
	Running *int `json:"running,omitempty"`
	// State is running, green or failed.
	State   string    `json:"state"`
	EndedAt time.Time `json:"ended_at,omitzero"`
	// FailedTier is the first tier that failed, nil when none did.
	FailedTier *int   `json:"failed_tier,omitempty"`
	Error      string `json:"error,omitempty"`
}

// CheckTier is one tier of the check as it ended.
type CheckTier struct {
	Tier int `json:"tier"`
	// Outcome is green or failed.
	Outcome     string  `json:"outcome"`
	Passed      int     `json:"passed"`
	Failed      int     `json:"failed"`
	Skipped     int     `json:"skipped"`
	WallSeconds float64 `json:"wall_seconds"`
	// Failures lists the tier's failed tests as the gate's counter reported them.
	Failures []gateslot.TestFailure `json:"failures,omitempty"`
}

// ReadCheck reads the check record at recordPath and reports whether one exists.
// An absent record reads false without taking a lock or creating a directory;
// a record that does not decode reads as absent with one Warn,
// so a damaged file never blocks the next check from writing a fresh one.
func ReadCheck(recordPath string) (CheckRecord, bool, error) {
	if _, err := os.Stat(recordPath); errors.Is(err, os.ErrNotExist) {
		return CheckRecord{}, false, nil
	}
	record, found, err := state.ReadJSON[CheckRecord](recordPath, recordPath+".lock")
	if errors.Is(err, state.ErrDecode) {
		logger.Warn("adoptcli: the adopt check record does not decode and reads as absent", "path", recordPath, "error", err)
		return CheckRecord{}, false, nil
	}
	return record, found, err
}

// checkDeps holds the seams of one check run: the subject and its scratch directory, the clock and process identity;
// the hub reconcile, owner read, gate, notice to the orch and mark on the prime's status bar.
// RunGate runs the check's gate key with log as its output and returns its exit code.
// OwnedElsewhere returns the machine that owns the subject when it is not this one, "" otherwise.
// Rerun is the command that checks the hold's subjects again, which the check's way forward lines name.
type checkDeps struct {
	Subject        string
	Dir            string
	Rerun          string
	Now            func() time.Time
	PID            int
	StartTicks     string
	Reconcile      func() error
	OwnedElsewhere func() (owner string, err error)
	RunGate        func(ctx context.Context, log io.Writer, observer gatecli.TierObserver) int
	Notice         func(line string) error
	Mark           func(mark checkMark) error
}

// subjectDir returns the scratch directory of the adopt check of subject, a beam slug or `root`, in the hub at hubPath.
func subjectDir(hubPath, subject string) string {
	return filepath.Join(fabriclayout.AdoptScratchDir(hubPath), subject)
}

// runCheck runs the check under check.lock and returns the final record and whether this call ran it.
// A subject owned by another machine ends the call with an error and a notice before the lock is taken;
// it writes no record.
// A held lock returns at once with no record written and nothing posted.
// A failing reconcile ends the check failed before any tier runs,
// so no check goes green on the template's tiers.
// The record is rewritten, and the prime's status bar marked, at each step of the check.
// The steps are its start, each tier's start and end, and its end;
// a post or mark that fails is logged and changes nothing.
// It posts each tier's notice and the notice of a check that ended before any tier did, and no end line.
func runCheck(ctx context.Context, deps checkDeps) (CheckRecord, bool, error) {
	opening := "adopt check of " + deps.Subject + ":"
	owner, err := deps.OwnedElsewhere()
	if err != nil {
		return CheckRecord{}, false, fmt.Errorf("%s read the owner of %s: %w; way forward: fix the cause it names, then re-run %s", opening, deps.Subject, err, deps.Rerun)
	}
	if owner != "" {
		text := fmt.Sprintf("%s %s is owned by %s, so no check ran; way forward: run %s on this machine", opening, deps.Subject, owner, deps.Rerun)
		if err := deps.Notice(text); err != nil {
			logger.Warn("adoptcli: the adopt check notice was not posted", "line", text, "error", err)
		}
		return CheckRecord{}, false, errors.New(text)
	}

	if err := os.MkdirAll(deps.Dir, 0o755); err != nil {
		return CheckRecord{}, false, fmt.Errorf("create the adopt scratch directory: %w", err)
	}
	held, acquired, err := lock.TryAcquireWriteLock(filepath.Join(deps.Dir, checkLockName))
	if err != nil {
		return CheckRecord{}, false, err
	}
	if !acquired {
		return CheckRecord{}, false, nil
	}
	defer func() { _ = held.Release() }()

	recordPath := filepath.Join(deps.Dir, checkRecordName)
	logPath := filepath.Join(deps.Dir, suiteLogName)
	logFile, err := os.Create(logPath)
	if err != nil {
		return CheckRecord{}, false, fmt.Errorf("open the suite log: %w", err)
	}
	defer logFile.Close()

	record := CheckRecord{Subject: deps.Subject, PID: deps.PID, StartTicks: deps.StartTicks, Start: deps.Now(), Gate: checkGateKey, State: checkRunning}
	if err := writeCheck(recordPath, record); err != nil {
		return CheckRecord{}, false, err
	}
	// tierStart is when the tier in flight started, which the record does not keep.
	var tierStart time.Time
	mark := func() {
		if err := deps.Mark(checkMarkOf(record, tierStart)); err != nil {
			logger.Warn("adoptcli: the adopt check mark was not set", "error", err)
		}
	}
	mark()
	post := func(line string) {
		if err := deps.Notice(line); err != nil {
			logger.Warn("adoptcli: the adopt check notice was not posted", "line", line, "error", err)
		}
	}
	// save rewrites the record mid-run, where a failed write costs a progress line and ends nothing.
	save := func() {
		if err := writeCheck(recordPath, record); err != nil {
			logger.Warn("adoptcli: the adopt check record was not written", "path", recordPath, "error", err)
		}
		mark()
	}
	finish := func() (CheckRecord, bool, error) {
		record.Running = nil
		record.EndedAt = deps.Now()
		err := writeCheck(recordPath, record)
		mark()
		return record, true, err
	}

	if err := deps.Reconcile(); err != nil {
		record.State = checkFailed
		record.Error = fmt.Sprintf("the hub reconcile failed (%v); way forward: fix the cause, then run lym fabric reconcile in the prime and re-run %s", err, deps.Rerun)
		post(opening + " " + record.Error)
		return finish()
	}

	observer := gatecli.TierObserver{
		Started: func(tier int) {
			record.Running = &tier
			tierStart = deps.Now()
			save()
		},
		Ended: func(ended gateslot.RunRecord) {
			tier := CheckTier{Tier: ended.Tier, Outcome: checkGreen, Passed: ended.Passed, Failed: ended.Failed, Skipped: ended.Skipped, WallSeconds: ended.WallSec, Failures: ended.Failures}
			if ended.Outcome != "passed" {
				tier.Outcome = checkFailed
			}
			record.Tiers = append(record.Tiers, tier)
			record.Running = nil
			save()
			post(tierNotice(deps.Subject, tier, logPath))
		},
	}
	code := deps.RunGate(ctx, logFile, observer)

	if code == 0 {
		record.State = checkGreen
		return finish()
	}
	record.State = checkFailed
	for _, tier := range record.Tiers {
		if tier.Outcome == checkFailed {
			number := tier.Tier
			record.FailedTier = &number
			break
		}
	}
	if record.FailedTier == nil {
		record.Error = fmt.Sprintf("the gate exited %d before any tier ended", code)
		post(fmt.Sprintf("%s failed, %s; log %s; way forward: fix what stopped the gate, read in the log, then re-run %s", opening, record.Error, logPath, deps.Rerun))
	}
	return finish()
}

// checkMark is what the check shows on the prime's status bar:
// its state, a one-line text and the start of its step in flight, zero for none.
type checkMark struct {
	State string
	Text  string
	Start time.Time
}

// checkMarkOf renders record as its status bar mark, where tierStart is when the tier in flight started.
// Every text names the record's subject, so two subjects' marks tell apart.
// A running check shows its done tiers with their outcomes and wall times;
// then the tier in flight timed from tierStart, or `starting` timed from the check's start before any tier.
// A green check renders the zero mark, which clears it:
// the check record, the orch's notice and `lym beam status` keep the result.
// A failed check stays shown as one short item, its first failed tier, naming `lym beam status` for the rest.
func checkMarkOf(record CheckRecord, tierStart time.Time) checkMark {
	name := "adopt check of " + record.Subject
	parts := make([]string, 0, len(record.Tiers)+1)
	for _, tier := range record.Tiers {
		parts = append(parts, fmt.Sprintf("tier%d %s %.0fs", tier.Tier, tier.Outcome, tier.WallSeconds))
	}
	listed := func(head string) string {
		if len(parts) == 0 {
			return head
		}
		return head + ": " + strings.Join(parts, " · ")
	}

	switch {
	case record.State == checkGreen:
		return checkMark{}
	case record.State == checkFailed && len(parts) == 0:
		return checkMark{State: checkFailed, Text: name + " failed before any tier ended; lym beam status shows why"}
	case record.State == checkFailed && record.FailedTier != nil:
		return checkMark{State: checkFailed, Text: fmt.Sprintf("%s failed at tier%d; lym beam status shows why", name, *record.FailedTier)}
	case record.State == checkFailed:
		return checkMark{State: checkFailed, Text: name + " failed; lym beam status shows why"}
	case record.Running != nil:
		parts = append(parts, "tier"+strconv.Itoa(*record.Running))
		return checkMark{State: checkRunning, Text: listed(name), Start: tierStart}
	case len(parts) == 0:
		return checkMark{State: checkRunning, Text: name + ": starting", Start: record.Start}
	default:
		return checkMark{State: checkRunning, Text: listed(name)}
	}
}

// primeMark returns the mark the prime's status bar shows for the check of subject.
// own is that subject's mark, and adoptDir holds the scratch directories of every subject's check.
// Its text is own's, then the marks of the other running checks, joined by ` | ` in subject order;
// its state is running while any joined record runs, else own's state.
// A zero own mark, a green check's, is left out of the join, so with no other check running the mark clears.
// A record that does not read is skipped with one Warn.
func primeMark(adoptDir, subject string, own checkMark) checkMark {
	entries, err := os.ReadDir(adoptDir)
	if err != nil {
		logger.Warn("adoptcli: the adopt scratch directory was not listed for the prime's mark", "path", adoptDir, "error", err)
		return own
	}
	var texts []string
	if own.Text != "" {
		texts = append(texts, own.Text)
	}
	state := own.State
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == subject {
			continue
		}
		record, found, err := ReadCheck(filepath.Join(adoptDir, entry.Name(), checkRecordName))
		if err != nil {
			logger.Warn("adoptcli: an adopt check record was not read for the prime's mark", "subject", entry.Name(), "error", err)
			continue
		}
		if !found || record.State != checkRunning {
			continue
		}
		texts = append(texts, checkMarkOf(record, time.Time{}).Text)
		state = checkRunning
	}
	return checkMark{State: state, Text: strings.Join(texts, " | "), Start: own.Start}
}

// tierNotice renders the notice of one ended tier of the check of subject.
// Its first line gives the outcome and counts; a failed tier adds one line per failed test.
// Each such line carries the first line the test printed, up to a fixed cap followed by a line counting the rest.
// The notice of a failed tier ends with the log at logPath.
func tierNotice(subject string, tier CheckTier, logPath string) string {
	lines := []string{fmt.Sprintf("adopt check of %s: tier%d %s, passed %d failed %d skipped %d, %.1fs", subject, tier.Tier, tier.Outcome, tier.Passed, tier.Failed, tier.Skipped, tier.WallSeconds)}
	if tier.Outcome != checkFailed {
		return lines[0]
	}
	for i, failure := range tier.Failures {
		if i == maxListedFailures {
			lines = append(lines, fmt.Sprintf("and %d more", len(tier.Failures)-maxListedFailures))
			break
		}
		line := failure.Package + " " + failure.Test
		if failure.FirstLine != "" {
			line += ": " + failure.FirstLine
		}
		lines = append(lines, line)
	}
	lines = append(lines, "log "+logPath)
	return strings.Join(lines, "\n")
}

// writeCheck writes record to the check record at recordPath under its state lock.
func writeCheck(recordPath string, record CheckRecord) error {
	return state.WriteJSON(recordPath, recordPath+".lock", record)
}

// resolveSubjectWorktree returns the location of the worktree the adopt check of subject runs in:
// the prime for `root`, else the beam's pair.
// A worktree that does not resolve is an error naming `lym beam start`; its caller adds the re-run.
func resolveSubjectWorktree(prime *lymcwd.Location, subject string) (*lymcwd.Location, error) {
	if subject == boardengine.RootSubject {
		return prime, nil
	}
	worktree, err := lymcwd.ResolveWorktree(fabricengine.WorktreePath(prime, subject))
	if err != nil {
		return nil, fmt.Errorf("the worktree of %s does not resolve (%w); way forward: run lym beam start %s", subject, err, subject)
	}
	return worktree, nil
}

// subjectOwnedElsewhere returns a func that reads the board for the owner of subject:
// the prime owner for `root`.
// The func returns that owner when another machine than this one owns it, and "" otherwise.
func subjectOwnedElsewhere(prime *lymcwd.Location, subject string) func() (string, error) {
	return func() (string, error) {
		owners, err := boardengine.ReadOwners(fabriclayout.BoardDir(prime.HubPath))
		if err != nil {
			return "", err
		}
		here, err := machinename.ReadIdentity()
		if err != nil {
			return "", err
		}
		owner := owners.Prime
		if subject != boardengine.RootSubject {
			owner, err = owners.Effective(subject, func(slug string) (string, error) { return hubgeom.ParentOf(prime, slug) })
			if err != nil {
				return "", err
			}
		}
		if !boardengine.OwnedElsewhere(owner, owners.Registry, here) {
			return "", nil
		}
		return owner, nil
	}
}

// hubCheckDeps fills checkDeps for the check of subject over the prime and the subject's worktree.
// The reconcile is the hub's: an absent board-level lym directory is an error, since an adopt runs only in a hub;
// the gate runs the hub's adopt key in the subject's anchor.
// The notice goes to the subject's nearest live orch, and the mark goes on the prime's reed session.
// A non-nil terminal also receives the gate's output, beside the suite log.
// It leaves Rerun unset, which its caller sets.
// It returns an error when the subject's worktree does not resolve.
// A reed config or geometry that does not read is each mark's error.
func hubCheckDeps(prime *lymcwd.Location, subject string, terminal io.Writer) (checkDeps, error) {
	worktree, err := resolveSubjectWorktree(prime, subject)
	if err != nil {
		return checkDeps{}, err
	}
	pid := os.Getpid()
	ticks, _ := proc.StartTime(pid)
	reed, reedErr := primeReed(prime)
	dir := subjectDir(prime.HubPath, subject)
	return checkDeps{
		Subject:    subject,
		Dir:        dir,
		Now:        time.Now,
		PID:        pid,
		StartTicks: ticks,
		Reconcile: func() error {
			geometry, ok := hubgeom.ReconcileGeometry(prime)
			if !ok {
				return errors.New("the hub has no board-level lym directory; an adopt runs only in a hub")
			}
			return hubreconcile.Ensure(geometry, hubreconcile.Options{})
		},
		OwnedElsewhere: subjectOwnedElsewhere(prime, subject),
		RunGate: func(ctx context.Context, log io.Writer, observer gatecli.TierObserver) int {
			if terminal != nil {
				log = io.MultiWriter(log, terminal)
			}
			return gatecli.RunGate(ctx, worktree.AnchorPath(), checkGateKey, log, log, observer)
		},
		Notice: func(line string) error {
			posted, err := orchcli.NotifyFrom(worktree, checkProgram, line)
			if err == nil && !posted {
				return errors.New("no live orch strand to post to")
			}
			return err
		},
		Mark: func(mark checkMark) error {
			if reedErr != nil {
				return reedErr
			}
			shown := primeMark(filepath.Dir(dir), subject, mark)
			return reed.SetCheckMark(shown.State, shown.Text, shown.Start)
		},
	}, nil
}

// primeReed returns the reed engine of the prime's session.
func primeReed(prime *lymcwd.Location) (*reedengine.Engine, error) {
	cfg, err := reedengine.LoadConfig(fabricengine.ConfigDirs(prime), "reed")
	if err != nil {
		return nil, err
	}
	geom, err := hubgeom.ReedGeometry(prime)
	if err != nil {
		return nil, err
	}
	return reedengine.New(cfg, geom, shuttleengines.EnvScrub()), nil
}

// checkCmd is the hidden `check <subject>` verb that a fix task's landing starts detached.
// It resolves the prime from the working directory, so it runs from anywhere in the hub.
// It runs the hold check over the subject and prints the check's state, the fix task filed or standing, and its lines.
// A check that did not run because another holds the lock also prints already_running.
// It exits 0 once the check returned a result, a failed check that filed its next task and one with no hold included.
func checkCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "check <subject>",
		Short:       "run the hub's adopt gate in a subject's worktree and report each tier to its orch",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()
			cwd, err := lymcwd.CwdFrom(ctx)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			location, err := lymcwd.Resolve(cwd)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			prime, err := battencli.ResolveBattenPrime(location)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			deps, err := hubHoldCheckDeps(prime, nil, false)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			result, err := runHoldCheck(ctx, deps, args[0])
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			fields := map[string]any{"state": result.State, "fix_task": result.FixTask, "lines": result.Lines}
			if !result.Ran {
				fields["already_running"] = true
			}
			clihelp.SetExit(ctx, output.Ok(out, fields))
			return nil
		},
	}
}
