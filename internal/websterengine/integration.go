// integration.go implements the plan-level integration-suite stage: the
// skip-check (ShouldRunIntegration), the single dedicated integration
// fork's own await/report plumbing (AwaitIntegration/RunIntegration,
// reusing AwaitBatch's own bounded long-poll idiom over a fixed, non-batch
// report path, and webster's own ParseReport for the fork's OK/FAILED), and
// the in-process SHA-bisect + escalation path a FAILED report triggers
// (bisect, RecordIntegrationFailure, BisectAndEscalate). The integration
// fork itself is spawned the same way a batch's own implementer is —
// Master's own in-session Agent-tool fork call, per master-template.md's
// own integration-fork bracket instruction — so this file never spawns
// anything; it only confirms the fork's report has landed, interprets it,
// and — on failure — localizes and records the offending card entirely
// in-process (no fork per bisect candidate), per the
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

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// IntegrationReportFileName is the integration fork's own fixed report file
// name inside a webster reports dir — distinct from ReportFileName's
// per-batch "NN-<slug>.yaml" naming, since the integration stage is not
// itself a plan card or execution batch: there is exactly one integration
// report per run.
const IntegrationReportFileName = "integration.yaml"

// IntegrationReportPath returns the path to the integration fork's report
// file inside reportsDir. Per the Hub Geometry Invariant, the caller
// resolves reportsDir (hubgeometry.WebsterReportsDir(...)); this function
// never constructs a `_lyx` path itself.
func IntegrationReportPath(reportsDir string) string {
	return filepath.Join(reportsDir, IntegrationReportFileName)
}

// ShouldRunIntegration reports whether plan carries a plan-level
// "## verify:" section at all — the skip-check for the WHOLE integration
// stage. A plan with no such section (plan.Verify == "") never drives the
// integration fork and proceeds straight to the summary/finish path, per
// plan-format-v3.md's "verify model" (the plan-level integration verify is
// optional): no error, no empty fork.
func ShouldRunIntegration(plan *planparser.Plan) bool {
	return plan.Verify != ""
}

// IntegrationAwaitResult is what one AwaitIntegration call hands back:
// ReportPresent reports whether the integration fork's report file existed
// by the time the call returned, and ElapsedS is how many seconds this call
// actually blocked — mirroring AwaitResult's own shape (awaitbatch.go) for
// the per-batch case.
type IntegrationAwaitResult struct {
	ReportPresent bool
	ElapsedS      int
}

// AwaitIntegration blocks until the integration fork's report file exists
// in reportsDir or wait elapses, re-checking on a fixed one-second tick
// (awaitTick, awaitbatch.go's own constant) via clk — the integration-stage
// analog of AwaitBatch, over the ONE fixed IntegrationReportPath rather than
// a per-batch report path, since the integration stage is not itself a plan
// card or execution batch. It reads and mutates NOTHING but the report
// path's existence, mirroring AwaitBatch's own read-only posture.
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

// RunIntegration is the integration stage's own run-once trigger: it blocks
// (via AwaitIntegration) until the integration fork's report lands or wait
// elapses, then parses it via ParseReport. It never runs the bisect itself
// — see this file's own bisect and BisectAndEscalate, which a caller
// invokes separately once RunIntegration reports a FAILED status — this
// function is the trigger/skip/await plumbing only.
func RunIntegration(reportsDir string, wait time.Duration, clk Clock) (*Report, error) {
	result, err := AwaitIntegration(reportsDir, wait, clk)
	if err != nil {
		return nil, err
	}
	if !result.ReportPresent {
		return nil, fmt.Errorf("webster: integration report did not land within %s", wait)
	}
	return ParseReport(IntegrationReportPath(reportsDir))
}

// integrationBatchKey is the reserved State.Batches key the integration
// stage's own terminal escalation record lives under: -1, which can never
// collide with a real plan card's own number (plan-format v3 card numbers
// are always positive, 1..N, per docs/reference/plan-format-v3.md's
// "Numbering and commit subject"), so RecordIntegrationFailure's own entry
// can never be mistaken for a real batch's record, and RenderProgress's
// walk over plan.Cards (which only ever looks up positive card numbers) can
// never surface it by accident.
const integrationBatchKey = -1

// bisect performs an in-process binary search over shas — the ordered
// per-card commit SHA trail accumulated across every batch's own
// BatchState.CardSHAs — to localize the first SHA at which verifyCmd
// fails: it captures repo's current branch (CurrentBranch), checks out each
// candidate SHA detached (CheckoutDetached), runs verifyCmd IN-PROCESS via
// os/exec at worktree (never a fork per candidate, preserving the
// no-concurrent-forks guarantee the integration stage as a whole depends
// on), and ALWAYS restores the captured branch (RestoreBranch) before
// returning — including on error, via defer — so HEAD is never left
// detached. shas is assumed ordered oldest-to-newest with verifyCmd passing
// on early entries and failing from the offending entry onward (the
// monotonic property a binary search requires); offendingIndex is the
// first index at which verifyCmd fails. An empty shas is a hard error —
// there is nothing to search over and nothing safe to report. A
// single-element shas degrades gracefully without spawning a checkout or a
// verify run at all: by construction it is the only candidate, so
// offendingIndex is 0 unconditionally (the sole/HEAD card).
func bisect(repo *gitrepo.Repo, shas []string, verifyCmd string, worktree string) (offendingIndex int, err error) {
	if len(shas) == 0 {
		return 0, fmt.Errorf("webster: bisect: no card SHAs recorded to search")
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

	// Standard binary search for the first failing index: a passing
	// candidate means the offending SHA is later; a failing one means it is
	// this candidate or earlier.
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

// checkoutAndVerify checks out sha detached in repo, then runs verifyCmd
// in-process at worktree, reporting whether it passed.
func checkoutAndVerify(repo *gitrepo.Repo, sha, verifyCmd, worktree string) (bool, error) {
	if err := repo.CheckoutDetached(sha); err != nil {
		return false, fmt.Errorf("webster: bisect: checkout %s: %w", sha, err)
	}
	return runVerifyCommand(verifyCmd, worktree)
}

// runVerifyCommand runs verifyCmd in-process via os/exec, in a shell ("sh
// -c" on non-Windows, "cmd /C" on Windows — the same GOOS split
// hubgeometry.go's own menuLauncherName applies for a platform-varying
// choice), from worktree. A non-zero exit is a failed verify — reported as
// (false, nil), never an error, since a verify command failing is the
// expected shape of a bisect step, not a bisect malfunction; a genuine
// spawn failure (the shell itself could not run) is a real error.
func runVerifyCommand(verifyCmd, worktree string) (bool, error) {
	shell, flag := "sh", "-c"
	if runtime.GOOS == "windows" {
		shell, flag = "cmd", "/C"
	}

	cmd := exec.Command(shell, flag, verifyCmd)
	cmd.Dir = worktree
	if err := cmd.Run(); err != nil {
		// An *exec.ExitError means the command ran and exited non-zero — a
		// failed verify, not a bisect malfunction. Any other error (the shell
		// itself could not be spawned) propagates as a real failure.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return false, nil
		}
		return false, fmt.Errorf("webster: bisect: run verify command %q: %w", verifyCmd, err)
	}
	return true, nil
}

// RecordIntegrationFailure marks a terminal, non-successful record for the
// integration stage into st under integrationBatchKey — the existing
// State.Batches terminal-record mechanism, reused rather than inventing a
// new operator signal or a new State field, per the
// integration-suite-fork-with-bisect decision's own "reuse the existing
// terminal path" escalation mechanism. The caller persists st via SaveState
// under the state-mutation lease, exactly as every other terminal batch
// mutation in this package does.
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

// BisectAndEscalate runs bisect over shas against a FAILED integration
// report, resolves the localized index to its own card label from labels
// (parallel to shas, same order/length — the caller's own ordered
// accumulation of every terminal batch's "NN-slug" identity alongside its
// own BatchState.CardSHAs), records the terminal escalation into st
// (RecordIntegrationFailure), and extends websterDir's summary.md
// (AppendIntegrationFailure, summary.go) naming the localized card. The
// caller persists st via SaveState under the state-mutation lease.
func BisectAndEscalate(repo *gitrepo.Repo, shas, labels []string, verifyCmd, worktree, websterDir string, st *State) error {
	idx, err := bisect(repo, shas, verifyCmd, worktree)
	if err != nil {
		return err
	}

	offendingSHA := shas[idx]
	offendingCard := "unknown"
	if idx < len(labels) {
		offendingCard = labels[idx]
	}

	RecordIntegrationFailure(st, offendingCard, offendingSHA)
	return AppendIntegrationFailure(websterDir, offendingCard, offendingSHA)
}
