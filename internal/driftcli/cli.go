// cli.go builds the drift cobra command with its scope, accept, withdraw, compare, capture, defer, waive, status and hidden capture-child verbs, and the RunCLI/RunCLIIn seams that wire them into the standard io.Writer-based call contract.

package driftcli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/proc"
)

// The bounds a capture start and a capture child's lock take, in production.
const (
	// captureRecordWait bounds how long a capture start waits for its child's running record.
	captureRecordWait = 30 * time.Second
	// captureRecordPoll is the interval a capture start reads that record at.
	captureRecordPoll = 50 * time.Millisecond
	// captureLockRetry bounds how long a capture child retries a capture lock a reader's probe holds.
	captureLockRetry = 2 * time.Second
	// compareLockRetry bounds how long a compare retries a compare marker a reader's probe holds.
	compareLockRetry = 2 * time.Second
)

// Command returns the cobra command for `lyx drift`: the bare group and its verbs.
func Command() *cobra.Command {
	drift := &cobra.Command{
		Use:   "drift",
		Short: "record a task's regression scope, capture its baseline and compare its code against it",
		Long: `drift runs regression around a task: the Drift-Baseline agent records the
task's scope, a capture takes the suite's profile on the task's branch point,
and Drift-Verify compares the task's code against it. The verbs run inside a
loom or darn run's worktree and judge the run's status, so each refusal names
its way forward. See lyx config drift for the capture and compare commands.`,
		RunE: clihelp.GroupRunE,
	}
	drift.AddCommand(scopeCmd(), acceptCmd(), withdrawCmd(), compareCmd(), captureCmd(), deferCmd(), waiveCmd(), statusCmd(), captureChildCmd())
	return drift
}

// runVerb resolves the worktree from the command's cwd and prints what run returns under key in the standard envelope, or a refusal through output.Err.
func runVerb(cmd *cobra.Command, key string, run func(w *worktree) (any, error)) error {
	if clihelp.ShouldAbort(cmd.Context()) {
		return nil
	}
	ctx := cmd.Context()
	out := cmd.OutOrStdout()
	w, err := openWorktree(cmd)
	if err == nil {
		var result any
		if result, err = run(w); err == nil {
			clihelp.SetExit(ctx, output.Ok(out, map[string]any{key: result}))
			return nil
		}
	}
	clihelp.SetExit(ctx, output.Err(out, err.Error()))
	return nil
}

// scopeCmd returns the `drift scope` verb.
func scopeCmd() *cobra.Command {
	var (
		scope string
		skip  []string
	)
	cmd := &cobra.Command{
		Use:         "scope --scope <scope>",
		Short:       "record the task's regression scope",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `scope records how much of the regression suite covers the task: off checks
nothing, full checks everything, and partial checks all but the tests --skip
names, which it requires. It runs once, at Drift-Baseline; a second call is
refused, and the agent then writes baseline.md alone and ends its turn.

Example:
  lyx drift scope --scope partial --skip slow_test --skip flaky_test`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, "scope", func(w *worktree) (any, error) {
				st, err := w.verbStatus()
				if err != nil {
					return nil, err
				}
				if err := driftengine.RecordScope(w.paths, st, scope, skip); err != nil {
					return nil, err
				}
				return map[string]any{"scope": scope, "skip": skip}, nil
			})
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "", "the scope: off, partial or full")
	cmd.Flags().StringArrayVar(&skip, "skip", nil, "a test the partial scope leaves out; repeatable")
	_ = cmd.MarkFlagRequired("scope")
	return cmd
}

// acceptCmd returns the `drift accept` verb.
func acceptCmd() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:         "accept <test> --reason <text>",
		Short:       "accept a drifted test as intended",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `accept records that a test's drift is intended, with the reason, so the
compare skips the test from then on. The Drift-Verify agent accepts while its
session runs; under scope full its acceptances stay unruled until the run's
parent rules at a decision halt. The parent uses accept at a halt, where it is
the only way to re-accept a test it withdrew. A halt that is no decision halt
refuses the parent's accept of a withdrawn test or under scope full.

Example:
  lyx drift accept parser_roundtrip --reason "the format changed on purpose"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, "accept", func(w *worktree) (any, error) {
				st, err := w.verbStatus()
				if err != nil {
					return nil, err
				}
				if err := driftengine.Accept(w.paths, st, args[0], reason); err != nil {
					return nil, err
				}
				return map[string]any{"test": args[0]}, nil
			})
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "why the drift is intended")
	_ = cmd.MarkFlagRequired("reason")
	return cmd
}

// withdrawCmd returns the `drift withdraw` verb.
func withdrawCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "withdraw <test>",
		Short:       "withdraw an accepted test so the compare runs it again",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `withdraw moves an accepted test back to the compare, so its drift is judged
again. The run's parent uses it at a Drift-Verify halt to overrule an
acceptance; the agent then fixes the drift or reports it in verify-report.md,
and only the parent re-accepts the test at a decision halt. A test that is not
accepted is refused; lyx drift status lists the accepted tests.

Example:
  lyx drift withdraw parser_roundtrip`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, "withdraw", func(w *worktree) (any, error) {
				st, err := w.verbStatus()
				if err != nil {
					return nil, err
				}
				if err := driftengine.Withdraw(w.paths, st, args[0]); err != nil {
					return nil, err
				}
				return map[string]any{"test": args[0]}, nil
			})
		},
	}
}

// spentRefusal words a refused run for the caller: the agent session ends its turn, and a caller at a halt resumes the run.
func spentRefusal(st driftengine.VerbStatus, err error) error {
	if st.AtAgentSession() {
		return fmt.Errorf("%w; way forward: write what you found to verify-report.md and end your turn, which stops the row", err)
	}
	return fmt.Errorf("%w; way forward: lyx loom resume spawns a fresh agent with a fresh budget, since a halt's run spends the counter of no live session", err)
}

// takeRun checks the Drift-Verify row, reads the declared step, and takes one run of the session's budget.
func takeRun(w *worktree, verb, stepName string) (driftengine.CommandStep, error) {
	st, err := w.verbStatus()
	if err != nil {
		return driftengine.CommandStep{}, err
	}
	if err := driftengine.CheckVerifyRow(st, verb); err != nil {
		return driftengine.CommandStep{}, err
	}
	cfg, err := w.config()
	if err != nil {
		return driftengine.CommandStep{}, err
	}
	step, err := cfg.Step(stepName)
	if err != nil {
		return driftengine.CommandStep{}, err
	}
	if _, err := driftengine.TakeRun(w.paths, cfg.VerifyMaxRuns); err != nil {
		if errors.Is(err, driftengine.ErrRunsSpent) {
			err = spentRefusal(st, err)
		}
		return driftengine.CommandStep{}, err
	}
	return step, nil
}

// compareCmd returns the `drift compare` verb.
func compareCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "compare",
		Short:       "run the compare against the baseline and print its result",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `compare runs the declared compare command against the baseline, skipping the
scope's skipped and accepted tests, and prints its exit code and log. It runs
at Drift-Verify only and spends one run of the agent session's budget; once
the budget is spent the agent writes what it found to verify-report.md and
ends its turn, and a caller at a halt resumes the run for a fresh budget.

Example:
  lyx drift compare`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, "compare", func(w *worktree) (any, error) {
				step, err := takeRun(w, "compare", "compare")
				if err != nil {
					return nil, err
				}
				scope, _, err := driftengine.ReadScope(w.paths)
				if err != nil {
					return nil, err
				}
				pool, waitDir := w.slots()
				result, err := driftengine.RunCompare(cmd.Context(), driftengine.CompareRequest{
					Paths:        w.paths,
					WorktreeRoot: w.root(),
					Step:         step,
					Pool:         pool,
					WaitDir:      waitDir,
					Skip:         driftengine.SkipList(scope),
					Timeout:      stepTimeout(step),
					Now:          time.Now,
					LockRetry:    compareLockRetry,
				})
				if err != nil {
					return nil, err
				}
				return map[string]any{"exit_code": result.ExitCode, "log": result.Log, "passed": result.Passed, "reason": result.Reason}, nil
			})
		},
	}
}

// captureCmd returns the `drift capture` verb.
func captureCmd() *cobra.Command {
	var side bool
	cmd := &cobra.Command{
		Use:         "capture",
		Short:       "capture the baseline in a detached process, or a side capture with --side",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `capture takes the suite's profile on the task's branch point. It runs at
Drift-Baseline, or at a Drift-Await halt, with HEAD on the branch point and a
clean tracked tree, starts the capture in a detached process that outlives this
call, and returns once its record reads running; lyx drift status shows it.
Another capture holding the lock is refused.

--side is the Drift-Verify agent's form: it captures a spare profile into a
numbered directory under the side directory, spends one run of the session's
budget, and needs no clean tree.

Example:
  lyx drift capture`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if side {
				return runVerb(cmd, "side_capture", func(w *worktree) (any, error) { return runSideCapture(cmd, w) })
			}
			return runVerb(cmd, "capture", func(w *worktree) (any, error) { return startMainCapture(w) })
		},
	}
	cmd.Flags().BoolVar(&side, "side", false, "take a spare side capture at Drift-Verify instead of the baseline")
	return cmd
}

// runSideCapture runs the agent's side capture and returns its result.
func runSideCapture(cmd *cobra.Command, w *worktree) (any, error) {
	step, err := takeRun(w, "capture --side", "capture")
	if err != nil {
		return nil, err
	}
	scope, _, err := driftengine.ReadScope(w.paths)
	if err != nil {
		return nil, err
	}
	pool, waitDir := w.slots()
	result, err := driftengine.RunSideCapture(cmd.Context(), driftengine.CaptureRequest{
		Paths:        w.paths,
		WorktreeRoot: w.root(),
		Step:         step,
		Pool:         pool,
		WaitDir:      waitDir,
		Skip:         driftengine.SkipList(scope),
		Timeout:      stepTimeout(step),
		Now:          time.Now,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"dir": result.Dir, "log": result.Log, "exit_code": result.ExitCode, "timed_out": result.TimedOut}, nil
}

// startMainCapture checks the capture's precondition and starts the detached child.
func startMainCapture(w *worktree) (any, error) {
	st, err := w.verbStatus()
	if err != nil {
		return nil, err
	}
	head, branchPoint, dirty, err := w.gitFacts()
	if err != nil {
		return nil, err
	}
	if err := driftengine.CheckMainCapture(st.Status, head, branchPoint, dirty); err != nil {
		return nil, err
	}
	cfg, err := w.config()
	if err != nil {
		return nil, err
	}
	if _, err := cfg.Step("capture"); err != nil {
		return nil, err
	}
	if testing.Testing() {
		return nil, errors.New("drift: the capture child is not spawned under go test")
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("drift: resolve this binary: %w", err)
	}
	pid, err := driftengine.StartCapture(driftengine.SpawnRequest{
		Executable:     executable,
		WorktreeRoot:   w.root(),
		Paths:          w.paths,
		RunProcDir:     fabricengine.RunProcDir(w.location.HubPath),
		Owner:          w.location.WorktreeName,
		FallbackDetach: true,
		RecordWait:     captureRecordWait,
		Poll:           captureRecordPoll,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"pid": pid}, nil
}

// captureChildCmd returns the hidden `drift capture-child` verb, the detached process `drift capture` spawns.
func captureChildCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "capture-child",
		Short:       "run the main capture of the detached child",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `capture-child is the body of the process "lyx drift capture" spawns detached.
It holds the capture's named job, runs the main capture under the capture lock,
re-checking the precondition under the lock, and releases the job as its last
act. It writes nothing to its log once the capture has returned.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			runCaptureChild(cmd)
			return nil
		},
	}
}

// runCaptureChild runs the child's capture and sets the exit code.
// Its only log writes come before the capture returns: a refusal of the precondition is written under the capture lock, because the Drift-Await row may commit the log once the lock is free.
func runCaptureChild(cmd *cobra.Command) {
	ctx, out := cmd.Context(), cmd.OutOrStdout()
	fail := func(err error) { clihelp.SetExit(ctx, output.Err(out, err.Error())) }

	w, err := openWorktree(cmd)
	if err != nil {
		fail(err)
		return
	}
	cfg, err := w.config()
	if err != nil {
		fail(err)
		return
	}
	step, err := cfg.Step("capture")
	if err != nil {
		fail(err)
		return
	}
	scope, _, err := driftengine.ReadScope(w.paths)
	if err != nil {
		fail(err)
		return
	}
	release, err := proc.HoldNamedJob(driftengine.CaptureJobName(w.root()))
	if err != nil {
		fail(err)
		return
	}

	pool, waitDir := w.slots()
	_, runErr := driftengine.RunCapture(ctx, driftengine.CaptureRequest{
		Paths:        w.paths,
		WorktreeRoot: w.root(),
		Step:         step,
		Pool:         pool,
		WaitDir:      waitDir,
		Skip:         scope.Skip,
		Check: func() (string, error) {
			head, err := checkMainCapture(w)
			if err != nil {
				fail(err)
				return "", err
			}
			return head, nil
		},
		Timeout:   stepTimeout(step),
		Now:       time.Now,
		LockRetry: captureLockRetry,
	})
	if runErr != nil {
		clihelp.SetExit(ctx, 1)
		logger.Warn("drift: the capture child ended with an error", "error", runErr.Error())
	}
	if releaseErr := release(); releaseErr != nil {
		logger.Warn("drift: releasing the capture job failed", "error", releaseErr.Error())
	}
}

// checkMainCapture re-reads the status, HEAD and the dirty paths, checks the main capture's precondition and returns the HEAD it checked.
func checkMainCapture(w *worktree) (string, error) {
	st, err := w.verbStatus()
	if err != nil {
		return "", err
	}
	head, branchPoint, dirty, err := w.gitFacts()
	if err != nil {
		return "", err
	}
	if err := driftengine.CheckMainCapture(st.Status, head, branchPoint, dirty); err != nil {
		return "", err
	}
	return head, nil
}

// deferCmd returns the `drift defer` verb.
func deferCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "defer",
		Short:       "settle the baseline wait with no baseline taken",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `defer sets the baseline as awaited with none taken, so the run goes on to
code changes; run lyx loom resume after it. It settles a Drift-Await halt or a
Drift-Baseline capture halt, and is refused while a capture runs. Drift-Verify
then halts on no counted capture, unless a passed capture exists, and only the
run's parent waives there.

Example:
  lyx drift defer`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, "defer", func(w *worktree) (any, error) {
				st, err := w.verbStatus()
				if err != nil {
					return nil, err
				}
				if err := driftengine.Defer(w.paths, st); err != nil {
					return nil, err
				}
				return map[string]any{"deferred": true}, nil
			})
		},
	}
}

// waiveCmd returns the `drift waive` verb.
func waiveCmd() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:         "waive --reason <text>",
		Short:       "waive the compare when no counted capture exists",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `waive records that the run goes on with no regression compare, with the
reason. It is admitted only at the blocked Drift-Verify halt that finds no
counted capture, so only the run's parent waives; run lyx loom resume after it.
At any other Drift-Verify halt it is refused, naming the halt's own ways
forward.

Example:
  lyx drift waive --reason "the suite cannot run on this machine"`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, "waive", func(w *worktree) (any, error) {
				st, err := w.verbStatus()
				if err != nil {
					return nil, err
				}
				if err := driftengine.Waive(w.paths, st, reason); err != nil {
					return nil, err
				}
				return map[string]any{"waived": true}, nil
			})
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "why the compare is waived")
	_ = cmd.MarkFlagRequired("reason")
	return cmd
}

// statusCmd returns the `drift status` verb.
func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "status",
		Short:       "print the scope and capture records, the locks and the run count",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `status prints the scope record, the capture record, whether the capture lock
and the compare marker are held, and how many runs the Drift-Verify agent
session has taken. It changes nothing.

Example:
  lyx drift status`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, "status", func(w *worktree) (any, error) { return readStatus(w) })
		},
	}
}

// readStatus gathers what `drift status` prints.
func readStatus(w *worktree) (any, error) {
	scope, scopeFound, err := driftengine.ReadScope(w.paths)
	if err != nil {
		return nil, err
	}
	capture, captureFound, err := driftengine.ReadCapture(w.paths)
	if err != nil {
		return nil, err
	}
	captureHeld, err := driftengine.ProbeLock(w.paths.CaptureLock)
	if err != nil {
		return nil, err
	}
	compareHeld, err := driftengine.ProbeLock(w.paths.CompareMarker)
	if err != nil {
		return nil, err
	}
	runs, err := driftengine.RunCount(w.paths)
	if err != nil {
		return nil, err
	}
	result := map[string]any{"capture_held": captureHeld, "compare_held": compareHeld, "runs": runs}
	if scopeFound {
		result["scope"] = scope
	}
	if captureFound {
		result["capture"] = capture
	}
	return result, nil
}

// RunCLI is the public seam for the drift module CLI, delegating to clihelp.Execute for output capture.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn is RunCLI's cwd-carrying sibling: an empty cwd reads the process cwd, any other value seeds cwd into the execution context, because lyxcwd.WithCwd panics on an empty directory.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
