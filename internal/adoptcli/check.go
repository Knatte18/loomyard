// check.go is the hidden `lym adopt check` verb: it runs the hub's adopt gate in-process in the prime, keeps its progress in check.json and reports each tier and the end to the nearest live orch.

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
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gatecli"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/proc"
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
)

// The states a CheckRecord is in.
const (
	checkRunning = "running"
	checkGreen   = "green"
	checkFailed  = "failed"
)

// CheckRecord is the progress of the adopt check, kept in check.json.
// An unknown key reads, so a newer writer's record never fails an older reader.
type CheckRecord struct {
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

// checkDeps holds the seams of one check run: the scratch directory its files sit in, the clock, the process identity it records, the hub reconcile, the gate and the notice to the orch.
// RunGate runs the check's gate key with log as its output and returns its exit code.
type checkDeps struct {
	Dir        string
	Now        func() time.Time
	PID        int
	StartTicks string
	Reconcile  func() error
	RunGate    func(ctx context.Context, log io.Writer, observer gatecli.TierObserver) int
	Notice     func(line string) error
}

// runCheck runs the check under check.lock and returns the final record and whether this call ran it.
// A held lock returns at once with nothing written or posted.
// A failing reconcile ends the check failed before any tier runs, so no check goes green on the template's tiers.
// The record is rewritten as each tier starts and ends; a post that fails is logged and changes nothing.
func runCheck(ctx context.Context, deps checkDeps) (CheckRecord, bool, error) {
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
	defer held.Release()

	recordPath := filepath.Join(deps.Dir, checkRecordName)
	logPath := filepath.Join(deps.Dir, suiteLogName)
	logFile, err := os.Create(logPath)
	if err != nil {
		return CheckRecord{}, false, fmt.Errorf("open the suite log: %w", err)
	}
	defer logFile.Close()

	record := CheckRecord{PID: deps.PID, StartTicks: deps.StartTicks, Start: deps.Now(), Gate: checkGateKey, State: checkRunning}
	if err := writeCheck(recordPath, record); err != nil {
		return CheckRecord{}, false, err
	}
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
	}
	finish := func() (CheckRecord, bool, error) {
		record.Running = nil
		record.EndedAt = deps.Now()
		return record, true, writeCheck(recordPath, record)
	}

	rerun := "lym gate test --gate " + checkGateKey
	if err := deps.Reconcile(); err != nil {
		record.State = checkFailed
		record.Error = fmt.Sprintf("the hub reconcile failed (%v); way forward: fix the cause, then run lym fabric reconcile in the prime and %s", err, rerun)
		post("adopt check: " + record.Error)
		return finish()
	}

	observer := gatecli.TierObserver{
		Started: func(tier int) {
			record.Running = &tier
			save()
		},
		Ended: func(ended gateslot.RunRecord) {
			tier := CheckTier{Tier: ended.Tier, Outcome: checkGreen, Passed: ended.Passed, Failed: ended.Failed, Skipped: ended.Skipped, WallSeconds: ended.WallSec}
			if ended.Outcome != "passed" {
				tier.Outcome = checkFailed
			}
			record.Tiers = append(record.Tiers, tier)
			record.Running = nil
			save()
			post(fmt.Sprintf("adopt check: tier%d %s, passed %d failed %d skipped %d, %.1fs", tier.Tier, tier.Outcome, tier.Passed, tier.Failed, tier.Skipped, tier.WallSeconds))
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
		post(fmt.Sprintf("adopt check: green, tiers %s; log %s", strings.Join(numbers, ", "), logPath))
	default:
		record.State = checkFailed
		var failure string
		for _, tier := range record.Tiers {
			if tier.Outcome == checkFailed {
				number := tier.Tier
				record.FailedTier = &number
				failure = fmt.Sprintf("tier%d failed", number)
				break
			}
		}
		if record.FailedTier == nil {
			record.Error = fmt.Sprintf("the gate exited %d before any tier ended", code)
			failure = record.Error
		}
		post(fmt.Sprintf("adopt check: failed, %s; log %s; way forward: fix it, then run %s in the prime", failure, logPath, rerun))
	}
	return finish()
}

// writeCheck writes record to the check record at recordPath under its state lock.
func writeCheck(recordPath string, record CheckRecord) error {
	return state.WriteJSON(recordPath, recordPath+".lock", record)
}

// hubCheckDeps fills checkDeps over the prime: the reconcile is the hub's, which reads an absent board-level lym directory as an error since an adopt runs only in a hub, the gate is the hub's adopt key in the prime's anchor, and the notice goes to the nearest live orch.
func hubCheckDeps(prime *lymcwd.Location) checkDeps {
	pid := os.Getpid()
	ticks, _ := proc.StartTime(pid)
	return checkDeps{
		Dir:        fabricengine.AdoptScratchDir(prime.HubPath),
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
		RunGate: func(ctx context.Context, log io.Writer, observer gatecli.TierObserver) int {
			return gatecli.RunGate(ctx, prime.AnchorPath(), checkGateKey, log, log, observer)
		},
		Notice: func(line string) error {
			posted, err := orchcli.NotifyFrom(prime, checkProgram, line)
			if err == nil && !posted {
				return errors.New("no live orch strand to post to")
			}
			return err
		},
	}
}

// checkCmd is the hidden `check` verb, which `lym adopt` spawns detached in the prime.
// It resolves the prime from the working directory, so it runs from anywhere in the hub, and prints the record's state, or already_running when another check holds the lock.
func checkCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "check",
		Short:       "run the hub's adopt gate in the prime and report each tier to the orch",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Args:        cobra.NoArgs,
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
			record, ran, err := runCheck(ctx, hubCheckDeps(prime))
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
