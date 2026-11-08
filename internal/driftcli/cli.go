// cli.go builds the drift cobra command and its verbs: capture, check, amend, defer, status and hidden capture-child.
// It also holds the RunCLI and RunCLIIn entry points that run that command over an io.Writer.

package driftcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/selfexec"
)

// The bounds a capture start and a capture child's lock take, in production.
const (
	// captureRecordWait bounds how long a capture start waits for its child's running record.
	captureRecordWait = 30 * time.Second
	// captureRecordPoll is the interval at which a capture start reads that record.
	captureRecordPoll = 50 * time.Millisecond
	// captureLockRetry bounds how long a capture child retries a capture lock a reader's probe holds.
	captureLockRetry = 2 * time.Second
	// compareLockRetry bounds how long a compare retries a compare marker a reader's probe holds.
	compareLockRetry = 2 * time.Second
)

// Command returns the cobra command for `lym drift`: the bare group and its verbs.
func Command() *cobra.Command {
	drift := &cobra.Command{
		Use:   "drift",
		Short: "capture a task's baseline and check its code's changes against the sealed expectation",
		Long: `drift runs regression around a task: Drift-Baseline records the task's scope
from drift.scope, off or full, and a capture takes the suite's profile on the
task's branch point. Drift-Expect has an agent write the changes the task is
expected to make, and seals them before the writer starts. The drift gate and
Drift-Verify then diff a capture of the writer's code against the baseline,
check every changed value against the sealed expectation, and have a judge rule
on each expected one. The verbs run inside the worktree of a tapestry or darn
run, or of a sampler run whose task sets drift.scope full, and judge the run's
status, so each refusal names its way forward. See lym config drift for the
capture and compare commands.`,
		RunE: clihelp.GroupRunE,
	}
	drift.AddCommand(captureCmd(), checkCmd(), amendCmd(), deferCmd(), statusCmd(), captureChildCmd())
	return drift
}

// runVerb resolves the worktree from the command's cwd.
// It prints what run returns under key in the standard envelope, or the refusal in an error envelope.
func runVerb(cmd *cobra.Command, key string, run func(w *worktree) (any, error)) error {
	return runVerbAt(cmd, key, "", run)
}

// runVerbAt resolves the worktree of the board slug in this hub, or the cwd's own worktree when slug is empty.
// It prints what run returns under key in the standard envelope, or the refusal in an error envelope.
func runVerbAt(cmd *cobra.Command, key, slug string, run func(w *worktree) (any, error)) error {
	if clihelp.ShouldAbort(cmd.Context()) {
		return nil
	}
	ctx := cmd.Context()
	out := cmd.OutOrStdout()
	var w *worktree
	var err error
	if slug == "" {
		w, err = openWorktree(cmd)
	} else {
		w, err = openSlugWorktree(cmd, slug)
	}
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

// captureCmd returns the `drift capture` verb.
func captureCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "capture",
		Short:       "capture the baseline in a detached process",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `capture takes the suite's profile on the task's branch point. It runs at
Drift-Baseline, or at a Drift-Await halt, with HEAD on the branch point and a
clean tracked tree, starts the capture in a detached process that outlives this
call, and returns once its record reads running; lym drift status shows it.
Another capture holding the lock is refused. A capture that has already ended
failed when the call returns, such as one that exited 0 with no baseline file,
fails the call with its reason: the directory the command ran in, its last
output and the ways forward. One that fails later records the same reason,
which lym drift status shows and the Drift-Await halt carries.

Example:
  lym drift capture`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, "capture", func(w *worktree) (any, error) { return startMainCapture(w) })
		},
	}
}

// checkCmd returns the `drift check` verb.
func checkCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "check",
		Short:       "run the drift gate's check on the current code and print what it found",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `check runs the check the drift gate runs: it captures the current code, diffs
the capture against the baseline and prints whether the gate would pass, the
cells the sealed expectation does not cover, and the faults the check found.
Run it after a change to see what the gate will say before the gate does.
It decides nothing, since Drift-Verify repeats the check, and writes no
tracked record, only a scratch capture directory. Before Drift-Expect has
passed no expectation is sealed, and it prints that and checks nothing.

Example:
  lym drift check`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, "check", func(w *worktree) (any, error) { return runCheck(cmd.Context(), w) })
		},
	}
}

// runCheck runs the drift check over w's worktree and returns what it printed.
func runCheck(ctx context.Context, w *worktree) (any, error) {
	deps, err := w.checkDeps()
	if err != nil {
		return nil, err
	}
	report, err := driftengine.Check(ctx, deps)
	if err != nil {
		return nil, err
	}
	return checkResult(report), nil
}

// checkResult renders a check report for the envelope.
// It reports whether the drift gate would pass, the skip, the faults by name, the unexpected cells and the findings.
// The gate fails on a capture fault or an unexpected cell, unless a skip or baseline fault ended the check first.
// No writer session clears a skip or baseline fault. A records or seal fault does not stop the gate failing.
func checkResult(report driftengine.CheckReport) map[string]any {
	unexpected := report.Unexpected()
	cells := make([]map[string]any, 0, len(unexpected))
	for _, classified := range unexpected {
		cell := classified.Cell
		cells = append(cells, map[string]any{
			"kind": string(cell.Kind), "case": cell.Case,
			"key": cell.Identity.Key, "n": cell.Identity.N, "line": cell.Identity.Line,
			"old": cell.Old, "new": cell.New,
		})
	}
	faults := map[string]string{}
	for name, text := range map[string]string{"baseline": report.BaselineFault, "records": report.RecordsFault, "seal": report.SealFault, "capture": report.CaptureFault} {
		if text != "" {
			faults[name] = text
		}
	}
	clearable := report.Skip == "" && report.BaselineFault == ""
	failing := clearable && (report.CaptureFault != "" || len(unexpected) > 0)
	return map[string]any{"would_pass": !failing, "skipped": report.Skip, "faults": faults, "unexpected": cells, "findings": report.Findings()}
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
	executable, err := selfexec.LymBinary()
	if err != nil {
		return nil, fmt.Errorf("drift: resolve this binary: %w", err)
	}
	pid, err := driftengine.StartCapture(driftengine.SpawnRequest{
		Executable:     executable,
		AnchorDir:      w.anchor(),
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
	rec, _, err := driftengine.ReadCapture(w.paths)
	if err != nil {
		return nil, err
	}
	if rec.Pid == pid && rec.State == driftengine.CaptureFailed {
		return nil, driftengine.FailedCaptureError(rec)
	}
	return map[string]any{"pid": pid, "state": rec.State}, nil
}

// captureChildCmd returns the hidden `drift capture-child` verb, the detached process `drift capture` spawns.
func captureChildCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "capture-child",
		Short:       "run the main capture of the detached child",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `capture-child is the body of the process "lym drift capture" spawns detached.
It holds the capture's named job, runs the main capture under the capture lock,
re-checking the precondition under the lock, and releases the job as its last
act. A capture that ends failed is written to its log as an error envelope
with the capture's reason, under the lock, and the process exits non-zero.
It writes nothing to its log once the capture has returned.`,
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
// Its only log writes come before the capture returns: a refusal of the precondition is written under the capture lock.
// The Drift-Await row may commit the log once the lock is free.
func runCaptureChild(cmd *cobra.Command) {
	ctx, out := cmd.Context(), cmd.OutOrStdout()
	fail := func(err error) { clihelp.SetExit(ctx, output.Err(out, err.Error())) }

	w, err := openWorktree(cmd)
	if err != nil {
		// Only a refusal fixed by the directory is lasting; a read error on the anchor marker may pass on a fresh start.
		if errors.Is(err, lymcwd.ErrNotAGitRepo) || errors.Is(err, lymcwd.ErrCwdOutsideAnchor) {
			clihelp.SetExit(ctx, output.ErrFields(out, err.Error(), map[string]any{driftengine.LastingFaultKey: true}))
			return
		}
		fail(err)
		return
	}
	cfg, err := w.config()
	if err != nil {
		fail(err)
		return
	}
	step, err := cfg.StepWithHubEnv("capture", w.configDirs())
	if err != nil {
		fail(err)
		return
	}
	release, err := proc.HoldNamedJob(driftengine.CaptureJobName(w.root()))
	if err != nil {
		fail(err)
		return
	}

	request := w.captureRequest(step)
	request.Check = func() (string, error) {
		head, err := checkMainCapture(w)
		if err != nil {
			fail(err)
			return "", err
		}
		return head, nil
	}
	request.LockRetry = captureLockRetry
	request.OnEnd = func(rec driftengine.CaptureRecord) {
		if rec.State != driftengine.CapturePassed {
			fail(driftengine.FailedCaptureError(rec))
		}
	}
	_, runErr := driftengine.RunCapture(ctx, request)
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
		Use:         "defer [<slug>]",
		Short:       "settle the baseline wait with no baseline taken",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `defer sets the baseline as awaited with none taken, so the run goes on to
code changes; run lym loom resume after it. It settles a Drift-Await halt, a
Drift-Baseline capture halt, and a halt of Drift-Expect, the writer,
Drift-Verify or Drift-Recheck whose reason names it or whose baseline check
fails. It is refused while a capture or a lym drift check holds the capture
lock. After the writer it skips the regression check for the rest of the run,
and the verdict states it, so only the run's parent or the operator's own
shell runs it.

With a slug it acts on that task's worktree, so the parent need not stand in
the run's worktree.

Example:
  lym drift defer
  lym drift defer my-task`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := ""
			if len(args) == 1 {
				slug = args[0]
			}
			return runVerbAt(cmd, "defer", slug, func(w *worktree) (any, error) {
				if _, err := w.callerBy("defer"); err != nil {
					return nil, err
				}
				st, err := w.verbStatus()
				if err != nil {
					return nil, err
				}
				deps, err := w.checkDeps()
				if err != nil {
					return nil, err
				}
				if err := driftengine.Defer(deps, st); err != nil {
					return nil, err
				}
				return map[string]any{"deferred": true}, nil
			})
		},
	}
}

// statusCmd returns the `drift status` verb.
func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "status",
		Short:       "print the scope and capture records, the locks and the diff mode",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `status prints the scope record, the capture record, whether the capture lock
and the compare marker are held, and the diff mode, the one the scope record
holds or drift.yaml's while none is recorded. Under lines it also prints
compare_ignored: true, since the compare command never runs under that mode.
It changes nothing.

Example:
  lym drift status`,
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
	mode := scope.Diff
	if mode == "" {
		cfg, err := w.config()
		if err != nil {
			return nil, err
		}
		mode = cfg.Diff
	}
	result := map[string]any{"capture_held": captureHeld, "compare_held": compareHeld, "diff": mode}
	if mode == driftengine.DiffLines {
		result["compare_ignored"] = true
	}
	if scopeFound {
		result["scope"] = scope
	}
	if captureFound {
		result["capture"] = capture
	}
	return result, nil
}

// RunCLI runs the drift command over args, writes its output to out and returns the exit code.
// It is the public entry point for the drift module CLI.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn runs the drift command over args in the directory cwd, writes its output to out and returns the exit code.
// An empty cwd reads the process's working directory;
// any other value is seeded into the execution context, since seeding an empty directory panics.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
