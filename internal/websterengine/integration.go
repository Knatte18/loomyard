// integration.go implements the plan-level integration-suite stage: the skip-check
// (ShouldRunIntegration), the single dedicated integration fork's own await/report plumbing
// (AwaitIntegration/RunIntegration, reusing AwaitBatch's own bounded long-poll idiom over a fixed,
// non-batch report path, and webster's own ParseReport for the fork's OK/FAILED), and the
// in-process SHA-bisect + escalation path a FAILED report triggers (bisect,
// RecordIntegrationFailure, BisectAndEscalate).
// The integration fork itself is spawned the same way a batch's own implementer is — Master's own
// in-session Agent-tool fork call, per webster-template-master.md's own integration-fork bracket
// instruction — so this file never spawns anything;
// it only confirms the fork's report has landed, interprets it, and — on failure — localizes and
// records the offending card entirely in-process (no fork per bisect candidate), per the
// integration-suite-fork-with-bisect decision.

package websterengine

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// FabricBisector is the git surface in-process bisect drives: capture branch, checkout SHA detached,
// restore branch.
// Satisfied by *gitrepo.Repo and *fabricengine.Fabric.
type FabricBisector interface {
	CurrentBranch() (string, error)
	CheckoutDetached(sha string) error
	RestoreBranch(ref string) error
}

// IntegrationReportFileName is the integration fork's own fixed report file name inside a webster
// reports dir — distinct from ReportFileName's per-batch "NN-<slug>.yaml" naming, since the
// integration stage is not itself a plan card or execution batch: there is exactly one integration
// report per run.
const IntegrationReportFileName = "integration.yaml"

// IntegrationReportPath returns the path to the integration fork's report file inside reportsDir.
func IntegrationReportPath(reportsDir string) string {
	return filepath.Join(reportsDir, IntegrationReportFileName)
}

// integrationPromptFileName is the integration fork's prompt file name inside a webster prompts dir.
const integrationPromptFileName = "integration.md"

// ShouldRunIntegration reports whether plan carries a plan-level "## verify:" section.
func ShouldRunIntegration(plan *planparser.Plan) bool {
	return plan.Verify != ""
}

// IntegrationAwaitResult is what one AwaitIntegration call hands back: ReportPresent (report file
// existed) and ElapsedS (seconds blocked).
type IntegrationAwaitResult struct {
	ReportPresent bool
	ElapsedS      int
}

// AwaitIntegration blocks until the integration fork's report file exists or wait elapses.
// It re-checks on a fixed one-second tick via clk (analogous to AwaitBatch).
func AwaitIntegration(reportsDir string, wait time.Duration, clk Clock) (*IntegrationAwaitResult, error) {
	reportPath := IntegrationReportPath(reportsDir)

	start := clk.Now()
	for {
		if _, statErr := os.Stat(reportPath); statErr == nil {
			return &IntegrationAwaitResult{
				ReportPresent: true,
				ElapsedS:      int(clk.Now().Sub(start).Seconds()),
			}, nil
		} else if !os.IsNotExist(statErr) {
			return nil, fmt.Errorf("webster: stat integration report %s: %w", reportPath, statErr)
		}

		elapsed := clk.Now().Sub(start)
		if elapsed >= wait {
			return &IntegrationAwaitResult{
				ReportPresent: false,
				ElapsedS:      int(elapsed.Seconds()),
			}, nil
		}
		clk.Sleep(awaitTick)
	}
}

// integrationBatchKey is the reserved State.Batches key (-1) for integration escalation.
// It never collides with real plan card numbers (1..N), so RenderProgress never surfaces it.
const integrationBatchKey = -1

// bisect performs an in-process binary search over shas to localize the
// first SHA at which verifyCmd fails. It restores HEAD to its original branch
// even on error (via defer). Edge cases: empty shas returns -1 (no search);
// single-element shas returns 0 (sole candidate).
func bisect(repo FabricBisector, shas []string, verifyCmd string, worktree string) (offendingIndex int, err error) {
	if len(shas) == 0 {
		return -1, nil
	}
	if len(shas) == 1 {
		return 0, nil
	}

	branch, err := repo.CurrentBranch()
	if err != nil {
		return 0, fmt.Errorf("webster: bisect: capture current branch: %w", err)
	}
	defer func() {
		_ = repo.RestoreBranch(branch)
	}()

	// Binary search for the first failing index.
	lo, hi := 0, len(shas)-1
	for lo < hi {
		mid := (lo + hi) / 2
		passed, verErr := checkoutAndVerify(repo, shas[mid], verifyCmd, worktree)
		if verErr != nil {
			return 0, verErr
		}
		if passed {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, nil
}

// checkoutAndVerify checks out sha detached, then runs verifyCmd in-process, reporting pass/fail.
func checkoutAndVerify(repo FabricBisector, sha, verifyCmd, worktree string) (bool, error) {
	if err := repo.CheckoutDetached(sha); err != nil {
		return false, fmt.Errorf("webster: bisect: checkout %s: %w", sha, err)
	}
	return runVerifyCommand(verifyCmd, worktree)
}

// runVerifyCommand runs verifyCmd in-process via os/exec. A non-zero exit is
// a failed verify (false, nil); a spawn failure propagates as a real error.
func runVerifyCommand(verifyCmd, worktree string) (bool, error) {
	shell, flag := "sh", "-c"
	if runtime.GOOS == "windows" {
		shell, flag = "cmd", "/C"
	}

	cmd := exec.Command(shell, flag, verifyCmd)
	cmd.Dir = worktree
	logger.Info("websterengine: spawning verify command", "shell", shell, "verifyCmd", verifyCmd, "worktree", worktree)
	if err := cmd.Run(); err != nil {
		// *exec.ExitError is a failed verify (expected); other errors propagate.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			logger.Info("websterengine: verify command exited", "verifyCmd", verifyCmd, "worktree", worktree, "exitCode", exitErr.ExitCode())
			return false, nil
		}
		logger.Warn("websterengine: verify command failed to spawn", "verifyCmd", verifyCmd, "worktree", worktree, "cause", err)
		return false, fmt.Errorf("webster: bisect: run verify command %q: %w", verifyCmd, err)
	}
	logger.Info("websterengine: verify command exited", "verifyCmd", verifyCmd, "worktree", worktree, "exitCode", 0)
	return true, nil
}

// RecordIntegrationFailure marks a terminal, non-successful record for the integration stage into
// st under integrationBatchKey.
// Caller persists via SaveState.
func RecordIntegrationFailure(st *State, offendingCard, offendingSHA string) {
	if st.Batches == nil {
		st.Batches = map[int]*BatchState{}
	}
	st.Batches[integrationBatchKey] = &BatchState{
		Slug:     offendingCard,
		Terminal: true,
		Status:   DigestStatusStuck,
		Digest: &Digest{
			Batch:   offendingCard,
			Status:  DigestStatusStuck,
			HeadSHA: offendingSHA,
		},
		CardSHAs: []string{offendingSHA},
	}
}

// BisectAndEscalate runs bisect over shas, records the terminal escalation into st, and extends
// summary.md naming the localized card.
// When shas is empty, falls back to "unknown" for both SHA and card.
// Caller persists via SaveState.
func BisectAndEscalate(repo FabricBisector, shas, labels []string, verifyCmd, worktree, websterDir string, st *State) error {
	idx, err := bisect(repo, shas, verifyCmd, worktree)
	if err != nil {
		return err
	}

	offendingSHA := "unknown"
	offendingCard := "unknown"
	if idx >= 0 {
		offendingSHA = shas[idx]
		if idx < len(labels) {
			offendingCard = labels[idx]
		}
	}

	RecordIntegrationFailure(st, offendingCard, offendingSHA)
	return AppendIntegrationFailure(websterDir, offendingCard, offendingSHA)
}
