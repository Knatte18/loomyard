// integrationfix.go is the home of the integration-fix attempt: the one-shot strand that tries to repair an integration regression.
// checkFixCommits decides whether Go accepts the commits that strand left behind.
//
// The path rule binds commits only.
// When the plan directory or `_lyx` lies outside the task repository, as in the hub geometry where `_lyx` is a directory link to the fabric's other side,
// no commit can reach a file there, and only a commit adding or retargeting the `_lyx` entry itself is visible.
// An on-disk plan write is caught by the plan-fingerprint compare around the strand;
// any other on-disk write under `_lyx` outside the repository is bounded by the strand's prompt alone.

package websterengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// IntegrationFixReportFileName is the integration-fix strand's report file name inside a webster reports dir.
const IntegrationFixReportFileName = "integration-fix.yaml"

// IntegrationFixReportPath returns the integration-fix strand's report path inside reportsDir.
func IntegrationFixReportPath(reportsDir string) string {
	return filepath.Join(reportsDir, IntegrationFixReportFileName)
}

// FixStarter is the seam the integration stage spawns its one fix strand through.
// Like MasterStarter it is two-phase, so the strand's GUID is persisted before Go blocks on the handle;
// production code passes an adapter over *shuttleengine.Runner, tests pass a fake.
type FixStarter interface {
	StartFix(shuttleengine.Spec) (MasterHandle, error)
}

// fixInputs is what the integration stage hands attemptIntegrationFix: the regression it found and the context to judge a fix against.
type fixInputs struct {
	plan *planparser.Plan
	// regressing are the pre-fix regressing failures with their tails.
	regressing []IntegrationFailure
	// offendingCard is bisect's "NN-slug" label, or "unknown".
	offendingCard string
	// startSHAs are the batches' recorded start commits, the post-fix triage's baseline candidates.
	startSHAs []string
	// bisector is the post-fix triage's repository handle, nil when the mode has no fabric.
	bisector FabricBisector
}

// fixAttempt is the outcome of the one integration-fix attempt.
type fixAttempt struct {
	Record IntegrationFixRecord
	// Post is the post-fix triage of a verify that ran red, nil when the verify passed or never ran.
	Post *triageOutcome
	// Warnings are the non-fatal observations the attempt made.
	Warnings []string
}

// fixed reports whether the attempt cleared the regression.
func (a *fixAttempt) fixed() bool {
	return a != nil && a.Record.Result == FixResultFixed
}

// attemptIntegrationFix runs the one fix attempt for a regression and returns its record.
// A returned error is an infrastructure failure; every way the strand itself can fail is a record with a non-fixed Result.
// It never holds the state-mutation lease across the strand's wait or a verify run.
func attemptIntegrationFix(deps RunDeps, in fixInputs) (*fixAttempt, error) {
	// A wiring fault must not spend the attempt, so the guard runs before anything is recorded.
	if deps.FixStarter == nil {
		return nil, fmt.Errorf("webster: integration stage: no FixStarter wired for the integration-fix attempt; this is a wiring fault, not a plan fault")
	}
	resolved, ok := deps.Roles[RoleRecovery]
	if !ok {
		return nil, fmt.Errorf("webster: no resolved model-spec for role %q", RoleRecovery)
	}
	worktree := deps.Geom.WorktreeRoot

	head, err := headSHA(worktree)
	if err != nil {
		return nil, err
	}

	// Leased: the attempt is recorded before the spawn, so a resumed run never spends a second one.
	spent, err := recordFixStart(deps, head)
	if err != nil {
		return nil, err
	}
	if spent != nil {
		commits, err := fixCommitsSince(worktree, spent.PreFixHead)
		if err != nil {
			return nil, err
		}
		return &fixAttempt{Record: IntegrationFixRecord{
			Result:     FixResultSpent,
			Detail:     fmt.Sprintf("the run's one fix attempt was already spent (result %q)", spent.Result),
			PreFixHead: spent.PreFixHead,
			Commits:    commits,
			Remaining:  failureIDs(in.regressing),
		}}, nil
	}

	planBefore, err := fingerprint(deps.Geom.PlanDir)
	if err != nil {
		return nil, err
	}

	fail := func(result, detail string) *fixAttempt {
		return &fixAttempt{Record: IntegrationFixRecord{Result: result, Detail: detail, PreFixHead: head, Remaining: failureIDs(in.regressing)}}
	}

	// Unleased: render and start the strand. A render or start error is a failed attempt, recorded like any other.
	reportPath, err := filepath.Abs(IntegrationFixReportPath(deps.Geom.ReportsDir))
	if err != nil {
		return nil, fmt.Errorf("webster: resolve integration fix report path: %w", err)
	}
	if err := os.MkdirAll(deps.Geom.ReportsDir, 0o755); err != nil {
		return nil, fmt.Errorf("webster: create reports dir %s: %w", deps.Geom.ReportsDir, err)
	}
	if err := os.Remove(reportPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("webster: remove stale integration fix report %s: %w", reportPath, err)
	}

	cardHint := ""
	if in.offendingCard != "unknown" {
		cardHint = filepath.ToSlash(filepath.Join(masterPlanDirDisplay(worktree, deps.Geom.PlanDir), in.offendingCard+".md"))
	}
	notePath := friction.NotePath(deps.FrictionDir, "webster-integration-fix")
	prompt, err := RenderIntegrationFixPrompt(in.regressing, in.plan.Verify, cardHint, reportPath, worktree, deps.Geom.PlanDir, deps.Geom.StencilsDir, notePath)
	if err != nil {
		return fail(FixResultFailed, fmt.Sprintf("render the fix prompt: %v", err)), nil
	}

	handle, err := deps.FixStarter.StartFix(shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{reportPath},
		Model:       resolved.Model,
		Effort:      resolved.Params["effort"],
		Version:     resolved.Params["version"],
		Role:        string(RoleRecovery),
		Round:       "integration-fix",
		Timeout:     time.Duration(deps.Config.RecoveryTimeoutMin) * time.Minute,
	})
	if err != nil {
		return fail(FixResultFailed, fmt.Sprintf("start the fix strand: %v", err)), nil
	}

	// Leased: the strand's GUID is durable before Go blocks on it, which is what run entry's reclaim reads.
	if err := recordFixStrand(deps, handle.StrandGUID()); err != nil {
		return nil, err
	}

	// Unleased: wait, then stop the strand and drop its run dir, as recover-batch does for a terminal recovery.
	result, waitErr := handle.Wait()
	var warnings []string
	if err := removeStrandIfLive(deps.Reed, handle.StrandGUID()); err != nil {
		warnings = append(warnings, fmt.Sprintf("integration fix: remove strand %s: %v", handle.StrandGUID(), err))
	}
	if result.RunDir != "" {
		if err := os.RemoveAll(result.RunDir); err != nil {
			warnings = append(warnings, fmt.Sprintf("integration fix: remove run dir %s: %v", result.RunDir, err))
		}
	}

	attempt := fixStrandVerdict(deps, head, reportPath, planBefore, result, waitErr, fail)
	attempt.Warnings = append(warnings, attempt.Warnings...)
	if attempt.Record.Result != "" {
		return attempt, nil
	}

	// Unleased: the post-fix verify decides, never the strand's report.
	return finishFixAttempt(deps, in, head, attempt)
}

// fixStrandVerdict judges the strand's end: its outcome, its report, the plan fingerprint and the commit check.
// It returns a record with a non-empty Result for every failed path and an empty Result when the strand's work is acceptable and still has to pass the verify.
func fixStrandVerdict(deps RunDeps, head, reportPath, planBefore string, result shuttleengine.Result, waitErr error, fail func(result, detail string) *fixAttempt) *fixAttempt {
	switch {
	case waitErr != nil:
		return withCommits(deps, head, fail(FixResultFailed, fmt.Sprintf("wait for the fix strand: %v", waitErr)))
	case result.Outcome == shuttleengine.OutcomeTimeout:
		return withCommits(deps, head, fail(FixResultTimeout, "the fix strand timed out"))
	case result.Outcome != shuttleengine.OutcomeDone:
		return withCommits(deps, head, fail(FixResultFailed, fmt.Sprintf("the fix strand ended %q without finishing", result.Outcome)))
	}

	report, err := ParseReport(reportPath)
	if err != nil {
		return withCommits(deps, head, fail(FixResultFailed, fmt.Sprintf("the fix strand's report is missing or malformed: %v", err)))
	}
	if report.Status != ReportStatusOK {
		return withCommits(deps, head, fail(FixResultFailed, "the fix strand reported FAILED"))
	}

	// The fingerprint compare catches an on-disk plan write no commit can show.
	planAfter, err := fingerprint(deps.Geom.PlanDir)
	if err != nil {
		return withCommits(deps, head, fail(FixResultRefused, fmt.Sprintf("fingerprint the plan after the fix strand: %v", err)))
	}
	if planAfter != planBefore {
		return withCommits(deps, head, fail(FixResultRefused, "the fix strand changed the plan on disk"))
	}
	warning, err := checkFixCommits(deps.Geom.WorktreeRoot, head, report.HeadSHA, deps.Geom.PlanDir, deps.ParentBranch)
	if err != nil {
		return withCommits(deps, head, fail(FixResultRefused, err.Error()))
	}
	attempt := &fixAttempt{Record: IntegrationFixRecord{PreFixHead: head}}
	if warning != "" {
		attempt.Warnings = append(attempt.Warnings, warning)
	}
	return attempt
}

// withCommits fills a's record with the commits now on the branch after head, so a failed attempt's way forward names them.
// A git failure leaves the list empty and is carried as a warning.
func withCommits(deps RunDeps, head string, a *fixAttempt) *fixAttempt {
	commits, err := fixCommitsSince(deps.Geom.WorktreeRoot, head)
	if err != nil {
		a.Warnings = append(a.Warnings, fmt.Sprintf("integration fix: list the fix commits: %v", err))
		return a
	}
	a.Record.Commits = commits
	return a
}

// finishFixAttempt runs the verify once at HEAD, triages a red run against the batches' start commits, and returns the attempt's final record.
// No regression in the result is success.
func finishFixAttempt(deps RunDeps, in fixInputs, head string, attempt *fixAttempt) (*fixAttempt, error) {
	worktree := deps.Geom.WorktreeRoot
	commits, err := fixCommitsSince(worktree, head)
	if err != nil {
		return nil, err
	}
	attempt.Record.Commits = commits
	regressingIDs := failureIDs(in.regressing)

	logPath := fixVerifyLogPath(deps.Geom.ScratchDir)
	run, err := runVerifyCapture(in.plan.Verify, worktree, logPath)
	if err != nil {
		return nil, err
	}
	if run.Passed {
		attempt.Record.Result = FixResultFixed
		attempt.Record.Cleared = regressingIDs
		return attempt, nil
	}

	post, err := triageIntegrationFailure(runVerifyCapture, in.bisector, in.startSHAs, in.plan.Verify, worktree, deps.Geom.ScratchDir, logPath)
	if err != nil {
		return nil, err
	}
	attempt.Post = &post
	if post.Triage.Verdict != TriageVerdictRegression {
		attempt.Record.Result = FixResultFixed
		attempt.Record.Cleared = regressingIDs
		return attempt, nil
	}

	attempt.Record.Result = FixResultFailed
	attempt.Record.Detail = "the post-fix verify still reports a regression"
	attempt.Record.Remaining = post.Triage.Regressions
	for _, id := range regressingIDs {
		if !slices.Contains(post.Triage.Regressions, id) {
			attempt.Record.Cleared = append(attempt.Record.Cleared, id)
		}
	}
	return attempt, nil
}

// fixVerifyLogPath returns the post-fix verify run's log path inside scratchDir.
func fixVerifyLogPath(scratchDir string) string {
	return filepath.Join(scratchDir, verifyLogDirName, "fix.log")
}

// fixCommitsSince returns the commits on HEAD after base, oldest first.
func fixCommitsSince(worktree, base string) ([]string, error) {
	head, err := headSHA(worktree)
	if err != nil {
		return nil, err
	}
	commits, err := commitsBetween(worktree, base, head)
	if err != nil {
		return nil, err
	}
	slices.Reverse(commits)
	return commits, nil
}

// recordFixStart records the attempt under the state-mutation lease and returns nil, or returns the already-recorded attempt, leaving state untouched.
func recordFixStart(deps RunDeps, head string) (*IntegrationFixState, error) {
	lease, err := AcquireStateMutation(deps.Geom.ScratchDir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = lease.Release() }()

	st, err := LoadState(deps.Geom.WebsterDir, deps.Geom.ScratchDir)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, fmt.Errorf("webster: integration stage: no state.json to record the fix attempt in")
	}
	if st.IntegrationFix != nil {
		spent := *st.IntegrationFix
		return &spent, nil
	}
	st.IntegrationFix = &IntegrationFixState{PreFixHead: head, SpawnedAt: time.Now().UTC().Format(time.RFC3339)}
	return nil, SaveState(deps.Geom.WebsterDir, deps.Geom.ScratchDir, st)
}

// recordFixStrand records the started strand's GUID under the state-mutation lease.
func recordFixStrand(deps RunDeps, guid string) error {
	lease, err := AcquireStateMutation(deps.Geom.ScratchDir)
	if err != nil {
		return err
	}
	defer func() { _ = lease.Release() }()

	st, err := LoadState(deps.Geom.WebsterDir, deps.Geom.ScratchDir)
	if err != nil {
		return err
	}
	if st == nil || st.IntegrationFix == nil {
		return fmt.Errorf("webster: integration stage: the fix attempt's record vanished before its strand was recorded")
	}
	st.IntegrationFix.StrandGUID = guid
	return SaveState(deps.Geom.WebsterDir, deps.Geom.ScratchDir, st)
}

// fixStuckReason extends base with the fix attempt's result and the way forward the refusal row names:
// the pre-fix head and the fix commits, keep or drop them, fix the regression, verify and commit, then re-run.
func fixStuckReason(base string, fix IntegrationFixRecord) string {
	commits := "none"
	if len(fix.Commits) > 0 {
		commits = strings.Join(fix.Commits, ", ")
	}
	reason := fmt.Sprintf("%s; integration fix attempt %s", base, fix.Result)
	if fix.Detail != "" {
		reason += " (" + fix.Detail + ")"
	}
	return fmt.Sprintf("%s; pre-fix head %s, fix commits %s: keep them or drop them with `git reset --hard %s`, fix the regression on the task branch, run the plan's `## verify:` and commit, then re-run `lyx webster run` (re-step the Webster row)", reason, fix.PreFixHead, commits, fix.PreFixHead)
}

// checkFixCommits returns an error, making the fix attempt count as failed, unless the strand's commits are acceptable:
// reportHead is preFixHead or descends from it, every commit in preFixHead..reportHead has exactly one parent,
// none changes a path under planDir, the `_lyx` entry itself or a path under it, HEAD reconciles against reportHead under the merge-only rule,
// and the worktree carries no uncommitted or untracked change, since the post-fix verify runs over the working tree and must judge only the commits.
// planDir may be absolute or relative to worktree; the plan rule is skipped when it lies outside worktree.
// The warning is reconcileHead's moved-HEAD notice, passed through.
func checkFixCommits(worktree, preFixHead, reportHead, planDir string, parentBranch ParentBranchFunc) (warning string, err error) {
	if reportHead != preFixHead {
		descends, err := isAncestor(worktree, preFixHead, reportHead)
		if err != nil {
			return "", err
		}
		if !descends {
			return "", fmt.Errorf("webster: integration fix: reported head %q does not descend from the pre-fix head %q", reportHead, preFixHead)
		}
	}

	planRel, err := planDirRel(worktree, planDir)
	if err != nil {
		return "", err
	}
	commits, err := commitsBetween(worktree, preFixHead, reportHead)
	if err != nil {
		return "", err
	}
	for _, commit := range commits {
		parents, err := commitParentCount(worktree, commit)
		if err != nil {
			return "", err
		}
		if parents != 1 {
			return "", fmt.Errorf("webster: integration fix: commit %s has %d parents; only non-merge commits are accepted", commit, parents)
		}
		paths, err := commitChangedPaths(worktree, commit)
		if err != nil {
			return "", err
		}
		for _, path := range paths {
			if fixForbiddenPath(path, planRel) {
				return "", fmt.Errorf("webster: integration fix: commit %s changes %s, which is under the plan or %s", commit, path, lyxdirs.LyxDirName)
			}
		}
	}

	warning, err = reconcileHead(worktree, reportHead, "integration fix", parentBranch, integrationFixHeadRefusal)
	if err != nil {
		return "", err
	}

	isDirty, err := dirty(worktree)
	if err != nil {
		return "", err
	}
	if isDirty {
		return "", fmt.Errorf("webster: integration fix: worktree %s has uncommitted or untracked changes; the post-fix verify must judge only commits", worktree)
	}
	return warning, nil
}

// planDirRel returns planDir as a slash-separated path relative to worktree, or "" when it lies outside worktree or is worktree itself.
func planDirRel(worktree, planDir string) (string, error) {
	if !filepath.IsAbs(planDir) {
		return filepath.ToSlash(filepath.Clean(planDir)), nil
	}
	root, err := canonicalPath(worktree)
	if err != nil {
		return "", err
	}
	dir, err := canonicalPath(planDir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", nil
	}
	return filepath.ToSlash(rel), nil
}

// fixForbiddenPath reports whether a fix commit may not change path: the `_lyx` entry itself, anything under it, or anything under planRel.
func fixForbiddenPath(path, planRel string) bool {
	if path == lyxdirs.LyxDirName || strings.HasPrefix(path, lyxdirs.LyxDirName+"/") {
		return true
	}
	return planRel != "" && (path == planRel || strings.HasPrefix(path, planRel+"/"))
}
