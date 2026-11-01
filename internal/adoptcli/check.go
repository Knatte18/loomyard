// check.go is the hidden `lym adopt check <subject>` verb: it runs the hub's adopt gate in-process in the subject's worktree, keeps its progress in the subject's check.json and reports each tier and the end to that worktree's nearest live orch.

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
	// checkRecordName is the progress record of the adopt check, beside suiteLogName.
	checkRecordName = "check.json"
	// checkOutName is the file the spawned check's stdout and stderr go to, so a losing check's envelope and a panic are on disk.
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

// The states a CheckRecord is in.
const (
	checkRunning = "running"
	checkGreen   = "green"
	checkFailed  = "failed"
)

// CheckRecord is the progress of one subject's adopt check, kept in that subject's check.json.
// An unknown key reads, so a newer writer's record never fails an older reader.
type CheckRecord struct {
	// Subject is the beam slug or `root` the check runs for.
	Subject string `json:"subject"`
	// PID and StartTicks name the process that wrote the record; StartTicks is its proc.StartTime, empty where the platform cannot read it.
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
// An absent record reads false without taking a lock or creating a directory, and a record that does not decode reads as absent with one Warn, so a damaged file never blocks the next check from writing a fresh one.
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

// checkDeps holds the seams of one check run: the subject it checks, the scratch directory its files sit in, the clock, the process identity it records, the hub reconcile, the owner read, the gate, the notice to the orch and the mark on the prime's status bar.
// RunGate runs the check's gate key with log as its output and returns its exit code.
// OwnedElsewhere returns the machine that owns the subject when it is not this one, "" otherwise.
// Green runs the green end once the gate exits 0 and the green record is written, and returns the lines to post after the green notice; nil does nothing.
// Failed runs the fix loop once the gate exits non-zero with a failed tier recorded and the failed record is written, and returns the lines to post after the failed notice; nil does nothing.
type checkDeps struct {
	Subject        string
	Dir            string
	Now            func() time.Time
	PID            int
	StartTicks     string
	Reconcile      func() error
	OwnedElsewhere func() (owner string, err error)
	RunGate        func(ctx context.Context, log io.Writer, observer gatecli.TierObserver) int
	Notice         func(line string) error
	Mark           func(mark checkMark) error
	Green          func() []string
	Failed         func(record CheckRecord) []string
}

// subjectDir returns the scratch directory of the adopt check of subject, a beam slug or `root`, in the hub at hubPath.
func subjectDir(hubPath, subject string) string {
	return filepath.Join(fabricengine.AdoptScratchDir(hubPath), subject)
}

// runCheck runs the check under check.lock and returns the final record and whether this call ran it.
// A subject owned by another machine ends the call with an error and a notice before the lock is taken, and writes no record.
// A held lock returns at once with nothing written or posted.
// A failing reconcile ends the check failed before any tier runs, so no check goes green on the template's tiers.
// The record is rewritten, and the prime's status bar marked, as the check starts, as each tier starts and ends, and at its end; a post or mark that fails is logged and changes nothing.
func runCheck(ctx context.Context, deps checkDeps) (CheckRecord, bool, error) {
	opening := "adopt check of " + deps.Subject + ":"
	owner, err := deps.OwnedElsewhere()
	if err != nil {
		return CheckRecord{}, false, fmt.Errorf("%s read the owner of %s: %w; way forward: fix the cause it names, then re-run lym adopt %s", opening, deps.Subject, err, deps.Subject)
	}
	if owner != "" {
		text := fmt.Sprintf("%s %s is owned by %s, so no check ran; way forward: run lym adopt %s on this machine", opening, deps.Subject, owner, deps.Subject)
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
	released := false
	releaseLock := func() {
		if !released {
			released = true
			_ = held.Release()
		}
	}
	defer releaseLock()

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
		record.Error = fmt.Sprintf("the hub reconcile failed (%v); way forward: fix the cause, then run lym fabric reconcile in the prime and re-run lym adopt %s", err, deps.Subject)
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

	switch {
	case code == 0:
		record.State = checkGreen
		numbers := make([]string, len(record.Tiers))
		for i, tier := range record.Tiers {
			numbers[i] = strconv.Itoa(tier.Tier)
		}
		post(fmt.Sprintf("%s green, tiers %s; log %s", opening, strings.Join(numbers, ", "), logPath))
	default:
		record.State = checkFailed
		for _, tier := range record.Tiers {
			if tier.Outcome == checkFailed {
				number := tier.Tier
				record.FailedTier = &number
				break
			}
		}
		if record.FailedTier != nil {
			post(fmt.Sprintf("%s failed, tier%d failed; log %s", opening, *record.FailedTier, logPath))
		} else {
			record.Error = fmt.Sprintf("the gate exited %d before any tier ended", code)
			post(fmt.Sprintf("%s failed, %s; log %s; way forward: fix what stopped the gate, read in the log, then re-run lym adopt %s", opening, record.Error, logPath, deps.Subject))
		}
	}

	final, ran, err := finish()
	if err != nil {
		return final, ran, err
	}
	var end func() []string
	switch {
	case final.State == checkGreen && deps.Green != nil:
		end = deps.Green
	case final.FailedTier != nil && deps.Failed != nil:
		end = func() []string { return deps.Failed(final) }
	default:
		return final, ran, nil
	}
	// The lock is let go first, so a check a concurrent adopt started meanwhile runs instead of exiting on it.
	releaseLock()
	for _, line := range end() {
		post(line)
	}
	return final, ran, nil
}

// checkMark is what the check shows on the prime's status bar: its state, a one-line text and the start of its step in flight, zero for none.
type checkMark struct {
	State string
	Text  string
	Start time.Time
}

// checkMarkOf renders record as its status bar mark, where tierStart is when the tier in flight started.
// Every text names the record's subject, so two subjects' marks tell apart.
// A running check shows its done tiers with their outcomes and wall times, then the tier in flight timed from tierStart, or `starting` timed from the check's start before any tier;
// an ended one shows its result and every tier it ran.
func checkMarkOf(record CheckRecord, tierStart time.Time) checkMark {
	name := "adopt check of " + record.Subject
	parts := make([]string, 0, len(record.Tiers)+1)
	for _, tier := range record.Tiers {
		if record.State == checkGreen {
			parts = append(parts, fmt.Sprintf("tier%d %.0fs", tier.Tier, tier.WallSeconds))
			continue
		}
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
		return checkMark{State: checkGreen, Text: listed(name + " green")}
	case record.State == checkFailed && len(parts) == 0:
		return checkMark{State: checkFailed, Text: name + " failed before any tier ended; lym beam status shows why"}
	case record.State == checkFailed:
		return checkMark{State: checkFailed, Text: listed(name + " failed")}
	case record.Running != nil:
		parts = append(parts, "tier"+strconv.Itoa(*record.Running))
		return checkMark{State: checkRunning, Text: listed(name), Start: tierStart}
	case len(parts) == 0:
		return checkMark{State: checkRunning, Text: name + ": starting", Start: record.Start}
	default:
		return checkMark{State: checkRunning, Text: listed(name)}
	}
}

// primeMark returns the mark the prime's status bar shows for the check of subject, whose own mark is own and whose scratch directories sit under adoptDir.
// Its text is own's, followed by the mark of each other subject whose record reads running, sorted by subject and joined by ` | `;
// its state is running while any joined record runs, else own's state.
// A record that does not read is skipped with one Warn.
func primeMark(adoptDir, subject string, own checkMark) checkMark {
	entries, err := os.ReadDir(adoptDir)
	if err != nil {
		logger.Warn("adoptcli: the adopt scratch directory was not listed for the prime's mark", "path", adoptDir, "error", err)
		return own
	}
	texts := []string{own.Text}
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

// tierNotice renders the notice of one ended tier of the check of subject: its outcome and counts on the first line, and for a failed tier one line per failed test with the first line it printed, at most maxListedFailures, then the log at logPath.
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

// resolveSubjectWorktree returns the location of the worktree the adopt check of subject runs in: the prime for `root`, else the beam's pair.
// A worktree that does not resolve is an error naming `lym beam start` and the re-run of the adopt.
func resolveSubjectWorktree(prime *lymcwd.Location, subject string) (*lymcwd.Location, error) {
	if subject == boardengine.RootSubject {
		return prime, nil
	}
	worktree, err := lymcwd.ResolveWorktree(fabricengine.WorktreePath(prime, subject))
	if err != nil {
		return nil, fmt.Errorf("the worktree of %s does not resolve (%w); way forward: run lym beam start %s, then re-run lym adopt %s", subject, err, subject, subject)
	}
	return worktree, nil
}

// subjectOwnedElsewhere returns a func that reads the board for the owner of subject, the prime owner for `root`, and returns it when another machine than this one owns it, "" otherwise.
func subjectOwnedElsewhere(prime *lymcwd.Location, subject string) func() (string, error) {
	return func() (string, error) {
		owners, err := boardengine.ReadOwners(fabricengine.BoardDir(prime.HubPath))
		if err != nil {
			return "", err
		}
		here, err := machinename.Name()
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
		if !boardengine.OwnedElsewhere(owner, here) {
			return "", nil
		}
		return owner, nil
	}
}

// hubCheckDeps fills checkDeps for the check of subject over the prime and the subject's worktree: the reconcile is the hub's, which reads an absent board-level lym directory as an error since an adopt runs only in a hub, the gate is the hub's adopt key in the subject's anchor, the notice goes to the subject's nearest live orch, and the mark goes on the prime's reed session.
// It returns an error when the subject's worktree does not resolve.
// A reed config or geometry that does not read is each mark's error.
func hubCheckDeps(prime *lymcwd.Location, subject string) (checkDeps, error) {
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
		Green: func() []string {
			green, err := hubGreenDeps(prime, subject)
			if err != nil {
				return []string{fmt.Sprintf("the green end of the adopt check of %s did not run (%v); way forward: fix the cause it names, then re-run lym adopt %s", subject, err, subject)}
			}
			return greenEnd(green)
		},
		Failed: func(record CheckRecord) []string {
			fix, err := hubFixDeps(prime, subject)
			if err != nil {
				return []string{fmt.Sprintf("the fix loop of the adopt check of %s did not run (%v); way forward: fix the cause it names, then re-run lym adopt %s, the holds standing meanwhile", subject, err, subject)}
			}
			return failedEnd(fix, record)
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

// checkCmd is the hidden `check <subject>` verb, which `lym adopt` spawns detached for each adopted subject.
// It resolves the prime from the working directory, so it runs from anywhere in the hub, and prints the record's state, or already_running when another check of the subject holds the lock.
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
			deps, err := hubCheckDeps(prime, args[0])
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			record, ran, err := runCheck(ctx, deps)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			if !ran {
				clihelp.SetExit(ctx, output.Ok(out, map[string]any{"already_running": true}))
				return nil
			}
			clihelp.SetExit(ctx, output.Ok(out, map[string]any{"state": record.State}))
			return nil
		},
	}
}
