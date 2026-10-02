// wiring.go implements wire, extracted from the pre-run (cli.go's resolvePersistentPreRun) so a test
// can drive it against a hand-built *lyxcwd.Location and stay tier 1.
// wire resolves no cwd and spawns no process -- every path it touches is either caller-supplied
// (location, cwd) or a plain config read anchored at location.AnchorPath(), so a test can drive it
// directly without breaching the Test Tier Purity Invariant.

package loomcli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/parentreview"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// commitStatusDeps carries the three fabric calls newCommitStatusSeam drives, injected as plain
// function values rather than reached directly, so the seam's branching -- commit hard-errors, push
// warns, mid-merge skips -- is drivable in a Tier 1 test from stub closures, with no hub fixture and
// no git spawn.
type commitStatusDeps struct {
	// MergeActive reports whether the fabric sibling worktree that carries the status file is
	// mid-merge at the git level.
	MergeActive func() (bool, error)
	// Commit commits loom's own status file with msg.
	// When the reviews directory holds a file, the same commit also carries the review round record.
	// While a rejection is pending the reviews root and the loom durable directory are left out until PR-Rework's round commit has landed and cleared it, so a status commit never lands half of a rework round; the next status commit sweeps them.
	Commit func(msg string) error
	// Push pushes the fabric sibling worktree's unpushed commits.
	Push func() error
}

// statusCommitPathspec returns the fabric-sibling pathspec one status commit stages: shedrun.StatusRel(location, runID), plus each of loomengine.LoomReviewsDirRel(), loomengine.LoomDurableDirRel() and shedrun.DriveReportsRel(location, runID) when its directory holds at least one non-directory entry anywhere beneath it.
// The loom durable directory is staged whole rather than its friction directory alone:
// staging the parent carries the friction directory, every timestamped archive sibling, and a reflection rename's removal of the old path in one entry,
// where a friction-only entry would miss the removal once the directory is renamed away.
// Existence alone is not enough: shedrecipe's Bouncer and BurlerRound entries os.MkdirAll every review row's run directory at recipe build time,
// so from a run's first transition the reviews root exists holding only empty segment directories.
// `git add -- <dir>` accepts an empty directory,
// but StageAndCommit's `git commit -- <pathspec>` then fails with "pathspec did not match any file(s) known to git", which the seam would turn into a hard error;
// a directory with no file is nothing to commit, never an error, so every absent, unreadable, non-directory or file-less outcome omits the entry.
// The directory pathspec is whole rather than per-file because StageAndCommit runs `git add -- <pathspec>`:
// an archive rename commits both the new timestamped sibling and the old path's removal,
// and a transition whose commit was skipped mid-merge is caught up by the next one.
// While a rejection is pending the reviews root and the loom durable directory are held back, so a status commit never lands half of a PR-Rework round:
// `git add -- <dir>` stages deletions too, and a status commit between the archive step and the round commit would land `round-<N>/` and the review run directories' deletions without the plan's.
// A classed `record.json` that reached HEAD this way would also make the next step count the round as committed and clear the rejection with no round commit.
// The pending rejection is the marker because `lyx loom reject` writes it before the archive step and PR-Rework removes it only after its round commit has landed.
// A stat of it that fails for any reason other than not-exist also holds back, since holding back defers the sweep and sweeping could split the round.
// The round commit's own pathspec covers both held directories, and the next status commit after the rejection clears sweeps what remained, so nothing is lost, only deferred.
func statusCommitPathspec(location *lyxcwd.Location, runID string) []string {
	paths := []string{shedrun.StatusRel(location, runID)}
	if !rejectionPending(location) {
		if holdsFile(loomengine.LoomReviewsDir(location)) {
			paths = append(paths, loomengine.LoomReviewsDirRel())
		}
		if holdsFile(loomengine.LoomDurableDir(location)) {
			paths = append(paths, loomengine.LoomDurableDirRel())
		}
	}
	if holdsFile(shedrun.DriveReportsDir(location, runID)) {
		paths = append(paths, shedrun.DriveReportsRel(location, runID))
	}
	return paths
}

// rejectionPending reports whether loomengine.LoomRejectionPath exists, or cannot be ruled out because its stat failed for a reason other than not-exist.
func rejectionPending(location *lyxcwd.Location) bool {
	_, err := os.Stat(loomengine.LoomRejectionPath(location))
	return err == nil || !errors.Is(err, fs.ErrNotExist)
}

// reworkCommitPathspec returns the pathspec PR-Rework's round commit stages: the plan directory, the rework directory, the reviews root and webster's durable directory,
// so the moved-from deletions and the archive land in one commit.
// The reviews root and webster's directory are left out when they match nothing in the index or the working tree, since git refuses such a pathspec (see statusCommitPathspec):
// that is when neither holds a file now and the round archived no file out of it, so nothing under it was ever tracked for this round to delete.
// The plan and rework directories always hold files by the time the round commits.
func reworkCommitPathspec(location *lyxcwd.Location) []string {
	reworkDir := loomengine.LoomReworkDir(location)
	paths := []string{planparser.PlanDirRel(), loomengine.LoomReworkDirRel()}
	if holdsFile(loomengine.LoomReviewsDir(location)) || holdsFileAt(loomshed.LatestArchivedReviewsDir(reworkDir)) {
		paths = append(paths, loomengine.LoomReviewsDirRel())
	}
	if holdsFile(websterengine.Dir(location.AnchorPath())) || holdsFileAt(loomshed.LatestArchivedWebsterDir(reworkDir)) {
		paths = append(paths, websterengine.DirRel())
	}
	return paths
}

// holdsFileAt is holdsFile with an empty dir meaning no directory, hence no file.
func holdsFileAt(dir string) bool {
	return dir != "" && holdsFile(dir)
}

// holdsFile reports whether dir is a directory holding at least one non-directory entry anywhere beneath it, stopping the walk at the first such entry.
func holdsFile(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	found := false
	// A walk error can only end the walk before a file is found, so found alone is the answer.
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}

// loomCommitStatusDeps builds a commitStatusDeps over location and runID, filling each field from fabric: MergeActive from fabricengine.MergeStateActive, Commit from fabricengine.CommitAnchoredPaths scoped to statusCommitPathspec (the status file, plus the review round record, the loom durable directory and the drive reports when each holds a file), and Push from fabricengine.PushAnchored.
func loomCommitStatusDeps(location *lyxcwd.Location, runID string) commitStatusDeps {
	return commitStatusDeps{
		MergeActive: func() (bool, error) {
			return fabricengine.MergeStateActive(location)
		},
		// Commit discards the (sha, committed) pair in favour of the error alone, exactly as
		// landingdeps.go's own CommitStatus closure does -- which is what makes a second call over
		// an already-clean tracked path a no-op rather than a failure.
		Commit: func(msg string) error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, statusCommitPathspec(location, runID), msg, fabricengine.EnvSyncOptions())
			return err
		},
		Push: func() error {
			_, err := fabricengine.PushAnchored(location, fabricengine.EnvSyncOptions())
			return err
		},
	}
}

// commitStatusMessage renders the commit message for a per-transition status commit. It is a
// function rather than a bare constant because the seam fires once per transition rather than once
// per landing, and an unreadable stream of identical messages is the log a resuming operator has to
// read.
func commitStatusMessage(producer, state string) string {
	return fmt.Sprintf("loom: %s -> %s", producer, state)
}

// commitStatusFailureDisposition decides what a failed status Commit means, and it exists because
// the mid-merge probe is unlocked by construction: nothing fabric can hold serialises against an
// operator running plain git inside the fabric sibling worktree, which the Fabric Git Invariant's
// own carve-out permits. So a merge can become live in the window between MergeActive answering
// false and the commit running, and when it does the commit fails on git's own "cannot do a partial
// commit during a merge" -- a path-scoped commit is a partial commit by definition.
//
// Without this re-probe that lost race took the commit-hard-errors disposition and killed the whole
// run, in precisely the situation skip-while-mid-merge exists to absorb gracefully. Re-probing turns
// it back into the skip it was always meant to be: if a merge is live NOW, the commit failure is
// explained and the run continues, and the next transition retries once the operator is done.
// A probe that errors on the re-read is treated as active for the same reason the first probe is --
// an unreadable probe is the same untrustworthy-git-state category, and probe I/O failures cluster
// exactly when foreign merge machinery is touching the repo.
//
// Every other commit failure keeps the hard-error disposition unchanged: a git fault on the run's
// own bookkeeping with no merge to explain it is real infrastructure breakage.
//
// One residual is NOT closed here and must not be read as closed: gitrepo.StageAndCommit runs
// `git add` before `git commit`, so a commit that loses this race has already staged the status file
// into the foreign merge's index, and the operator's own conclude will carry it. Closing that would
// take a lock the operator does not take, so it is stated rather than defended against.
func commitStatusFailureDisposition(deps commitStatusDeps, producer, state string, commitErr error) error {
	active, probeErr := deps.MergeActive()
	if probeErr != nil {
		logger.Warn("loomcli: status commit failed and the merge-state re-probe failed too; continuing",
			"producer", producer, "state", state, "commit_error", commitErr, "probe_error", probeErr)
		return nil
	}
	if active {
		logger.Warn("loomcli: status commit failed because the fabric sibling went mid-merge after the probe; skipping this transition",
			"producer", producer, "state", state, "error", commitErr)
		return nil
	}
	return commitErr
}

// newCommitStatusSeam builds the shedengine.Shed.CommitStatus closure from deps, implementing three
// dispositions, evaluated in the order they appear in the closure body -- skip first, then commit,
// then push:
//
//  1. skip-while-mid-merge: MergeActive reporting true skips both Commit and Push, logged at warn. A
//     non-nil error from MergeActive is treated exactly like true -- an unreadable probe is the same
//     "git state cannot be trusted right now" category the skip exists for, and probe I/O failures
//     cluster precisely when foreign merge machinery is touching the repo.
//     The probe is unlocked, so this disposition also has a second half at the other end of the
//     window: see commitStatusFailureDisposition.
//  2. commit-hard-errors: a Commit failure returns an error from the seam and therefore halts the
//     run -- a git fault on the run's own bookkeeping is infrastructure breakage, the same disposition
//     landingshed already gives a failing CommitStatus. The one exception is a failure the re-probe
//     explains as a merge that went live after the first probe, which takes the skip disposition
//     instead.
//  3. push-warns: a Push failure logs a warning and returns nil -- an offline laptop must not kill an
//     autonomous run, and the next transition's push catches the branch up.
//     EVERY push error warns here, gitrepo.ErrPushRejected and otherwise alike; the sentinel is not
//     discriminated. A rejection means another machine advanced the branch, which is a human decision
//     rather than something a background persist may rewrite history over -- and an unreachable
//     remote is the offline case the disposition exists for -- so the two land in the same place.
func newCommitStatusSeam(deps commitStatusDeps) func(producer, state string) error {
	return func(producer, state string) error {
		active, err := deps.MergeActive()
		if err != nil {
			logger.Warn("loomcli: skip status commit, merge-state probe failed", "producer", producer, "state", state, "error", err)
			return nil
		}
		if active {
			logger.Warn("loomcli: skip status commit, fabric sibling is mid-merge", "producer", producer, "state", state)
			return nil
		}

		if err := deps.Commit(commitStatusMessage(producer, state)); err != nil {
			return commitStatusFailureDisposition(deps, producer, state, err)
		}

		if err := deps.Push(); err != nil {
			logger.Warn("loomcli: status push failed, next transition will catch up", "producer", producer, "state", state, "error", err)
			return nil
		}
		return nil
	}
}

// wireLightweight builds the minimum the verbUsesLightweightWiring set needs onto c: location, cwd,
// the two status-file paths, and (for the validate verbs) the c.env path fields they read plus the
// committed-file seam validate-plan --rework reads. It loads
// no module config, constructs no engine, and can fail only if loomengine's own path accessors do,
// which they cannot.
//
// It exists because wire() below loads eight module configs, a model-spec registry and the active
// batchifier before any verb body runs, so a fault in ANY of them refused "lyx loom status" and
// "lyx loom pause" outright -- including a fault an agent loom itself spawned, which is how this was
// found: a Discussion-Write agent rewrote loom.yaml mid-run and from that moment the operator had
// neither the read-out nor the emergency brake for a run that was still going. pause is the
// documented graceful-stop mechanism; losing it to an unrelated config problem inverts the cost.
//
// Crucible round sonnet5-xhigh-r8's F2 extended this path to validate-discussion/validate-plan for
// the identical reason: both writer agents' own stencils instruct them to run these two verbs as a
// pre-handoff self-check, mid-run, while a sibling process may be actively rewriting any of the eight
// configs wire() loads -- and validateDiscussionCmd/validatePlanCmd (validate.go) read only
// DecisionRecordPath/SupportLogPath/AnchorPath/WorktreeRoot, none of which need any config load.
// validate-description joined for the same reason, reading DescriptionPath alone.
//
// approve is on this path because it builds no producer: it reads the status file, opens the fabric
// on demand and queries GitHub, then writes the approval record, none of which needs a module config.
//
// The verbs that actually build producers -- start and run -- deliberately keep the full wire(), and
// keep failing early on a bad config, because for them an unloadable config is a real refusal rather
// than an unrelated one. That is the same reasoning wire()'s own landingCfg comment already gives
// for loading landing.yaml eagerly.
func (c *loomCLI) wireLightweight(location *lyxcwd.Location, cwd string) {
	c.location = location
	c.cwd = cwd
	// CommitStatus is filled here too, even though no verb on this path writes the status file and so
	// none invokes it: filling both literals keeps them structurally identical, so a future verb promoted
	// from this path to wire()'s cannot silently lose the hook. loomCommitStatusDeps builds three
	// closures and performs no I/O at build time, so it neither loads config nor opens a fabric --
	// this doc comment's own claim that wireLightweight "loads no module config, constructs no
	// engine, and can fail only if loomengine's own path accessors do" stays true with this fill in
	// place.
	c.shedPaths = shedbuild.ShedPaths{
		StatusPath:     shedrun.StatusFile(location, c.runID),
		LockPath:       shedrun.RunLock(location, c.runID),
		StatusLockPath: shedrun.StatusLock(location, c.runID),
		CommitStatus:   newCommitStatusSeam(loomCommitStatusDeps(location, c.runID)),

		RunID:                   c.runID,
		MissingStatusWayForward: loomMissingStatusWayForward,
	}
	// c.env is otherwise left at its zero value deliberately: status/pause/approve read nothing from
	// it, and filling only the fields the validate verbs actually read (rather than the whole of
	// wire()'s c.env assembly) is what keeps this path from re-acquiring the eight-config load it
	// exists to avoid.
	c.env.AnchorPath = location.AnchorPath()
	c.env.WorktreeRoot = location.WorktreePath()
	c.env.DecisionRecordPath = loomengine.DiscussionDecisionRecord(location)
	c.env.SupportLogPath = loomengine.DiscussionSupportLog(location)
	c.env.DescriptionPath = summaryparser.Path(loomengine.LandingDir(location))
	c.env.Rework.ReadCommitted = committedAnchoredReader(location)
}

// committedAnchoredReader returns the seam that reads an anchor-relative file as committed at HEAD for location, with found false when HEAD has no such file.
// Building it opens nothing: the fabric is read on each call.
func committedAnchoredReader(location *lyxcwd.Location) func(anchorRel string) ([]byte, bool, error) {
	return func(anchorRel string) ([]byte, bool, error) {
		return fabricengine.CommittedAnchoredFile(location, anchorRel)
	}
}

// wire builds the whole engine stack onto c from location and cwd: every module config anchored at
// location.AnchorPath(), the reed engine and shuttle runner, the assembled websterengine.RunDeps, and
// the assembled shedrecipe.Env/shedbuild.ShedPaths pair wrapping it.
// discussionCommitPathspec is the pathspec CommitDiscussion stages: the whole discussion directory, plus the parent-review round directories so the review records land in the same commit.
// The parent-review directory is left out while it holds no file, since git refuses such a pathspec (see statusCommitPathspec):
// that is a run with no reviewer, whose fresh spawn leaves an empty round directory, or one whose parent-review entry is off.
// Its round files are never deleted, so a directory holding no file was never tracked either.
func discussionCommitPathspec(location *lyxcwd.Location) []string {
	paths := []string{loomengine.DiscussionDirRel()}
	if holdsFile(loomengine.LoomParentReviewDir(location)) {
		paths = append(paths, loomengine.LoomParentReviewDirRel())
	}
	return paths
}

// newParentReviewConfig builds the Discussion-Write parent-review gate's told input.
// The reviewer is the seed's Parent, empty when the seed has none or no seed exists;
// a seed read error is a wiring error.
func newParentReviewConfig(location *lyxcwd.Location, runID string, cfg loomengine.Config, stencilsDir string) (parentreview.GateConfig, error) {
	seed, _, err := shedrun.ReadSeed(location, runID)
	if err != nil {
		return parentreview.GateConfig{}, fmt.Errorf("loom: read seed for the parent review reviewer: %w", err)
	}
	slug := seedSlug(location.WorktreeName)
	decisionRecord := loomengine.DiscussionDecisionRecord(location)
	supportLog := loomengine.DiscussionSupportLog(location)
	return parentreview.GateConfig{
		Store:          parentreview.Store{Root: loomengine.LoomParentReviewDir(location), LockDir: loomengine.LoomParentReviewLockDir(location)},
		Slug:           slug,
		Reviewer:       seed.Parent,
		DecisionRecord: decisionRecord,
		SupportLog:     supportLog,
		WaitBound:      time.Duration(cfg.ParentReviewWaitMin) * time.Minute,
		RenderDelivery: func(requestPath string) (string, error) {
			return loomengine.ParentReviewDeliveryPrompt(stencilsDir, slug, requestPath, seed.Parent)
		},
		RenderBrief: func() (string, error) {
			return loomengine.ParentReviewBrief(stencilsDir, slug, decisionRecord, supportLog)
		},
	}, nil
}

func (c *loomCLI) wire(location *lyxcwd.Location, cwd string) error {
	anchorPath := location.AnchorPath()

	loomCfg, err := loomengine.LoadConfig(anchorPath, "loom")
	if err != nil {
		return err
	}
	reedCfg, err := reedengine.LoadConfig(anchorPath, "reed")
	if err != nil {
		return err
	}
	shuttleCfg, err := shuttleengine.LoadConfig(anchorPath, "shuttle")
	if err != nil {
		return err
	}
	websterCfg, err := websterengine.LoadConfig(anchorPath, "webster")
	if err != nil {
		return err
	}
	landingCfg, err := landingshed.LoadConfig(anchorPath, "landing")
	if err != nil {
		return err
	}
	// burlerengine.LoadConfig takes one argument, not the (baseDir, module) shape every other
	// loader above takes: it is an optional-file loader, so an absent burler.yaml yields a zero
	// Config and a nil error rather than a load failure.
	burlerCfg, err := burlerengine.LoadConfig(anchorPath)
	if err != nil {
		return err
	}
	registry, err := modelspec.LoadRegistry(anchorPath)
	if err != nil {
		return err
	}
	roles, err := websterengine.ResolveRoles(websterCfg, registry)
	if err != nil {
		return err
	}
	activeBatcher, err := batcher.Active(anchorPath)
	if err != nil {
		return err
	}
	reviewSettings, err := loomengine.ResolveReview(loomCfg, registry)
	if err != nil {
		return err
	}

	reedGeom, err := hubgeom.ReedGeometry(location)
	if err != nil {
		return err
	}
	reedEngine := reedengine.New(reedCfg, reedGeom)
	claudeEngine := claudeengine.New()
	runner := shuttleengine.NewRunner(reedEngine, claudeEngine, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)

	websterGeom := hubgeom.WebsterGeometry(location)

	// frictionDir is the single resolved value every told-friction consumer below reads: non-empty
	// only when loom.yaml's friction key is set, per the "Tier 2 off is an empty path string, never a
	// boolean" Shared Decision. Resolving it once here, rather than letting each consumer re-check
	// loomCfg.Friction itself, is what keeps burlerengine and websterengine's RunDeps from disagreeing
	// about whether Tier 2 is on.
	frictionDir := ""
	if loomCfg.Friction != "" {
		frictionDir = loomengine.LoomFrictionDir(location)
	}

	// BurlerGeometry, not WebsterGeometry: this is burler's own geometry, carrying burler's
	// AnchorPath semantics and the field set burlerengine.Geometry declares, rather than webster's
	// (see hubgeom.go's BurlerGeometry doc comment). The two geometry builders are distinct types
	// with distinct field sets, not interchangeable constructors of the same shape.
	burlerEngine := burlerengine.New(runner, hubgeom.BurlerGeometry(location), burlerCfg, websterGeom.StencilsDir, frictionDir)

	runDeps := websterengine.RunDeps{
		Starter:    runnerMasterStarter{runner: runner},
		Reed:       reedEngine,
		Engine:     claudeEngine,
		ShuttleCfg: shuttleCfg,
		Roles:      roles,
		Config:     websterCfg,
		Batcher:    activeBatcher,
		Geom:       websterGeom,
		// FrictionDir covers the Master and integration prompts only: the per-batch fork and recovery
		// prompts are composed in a separate `lyx webster` process (begin-batch/recover-batch), which
		// resolves the same value itself in internal/webstercli, per the
		// webstercli-resolves-the-friction-directory-in-hub-mode Shared Decision.
		FrictionDir: frictionDir,
		// The reference matcher is pinned to a real fabricengine.NewRefScanner(location), built
		// eagerly because that constructor only compiles a regexp and cannot fail. It must never be
		// the never-matching stand-in: that stand-in is permitted only in standalone, where there is
		// no wired fabric for the guard to protect, and loom is hub-only.
		RefMatcher: fabricengine.NewRefScanner(location),
		// The bisector opener stays a lazy closure over fabricengine.Open(location) and must not be
		// opened here: opening stat-checks the paired sibling, and this pre-run must not fail
		// "status"/"pause" against a healthy-but-unwired location.
		OpenBisector: func() (websterengine.FabricBisector, error) {
			return fabricengine.Open(location)
		},
	}

	statusPath := shedrun.StatusFile(location, c.runID)
	statusLockPath := shedrun.StatusLock(location, c.runID)

	// reworkDeps opens nothing at wire time: every closure reads or writes on demand, since wire() also runs for status/pause.
	// PR-Rework and the Plan-Review skip seam share it.
	reworkDeps := loomshed.PRReworkDeps{
		PlanDir:      planparser.PlanDir(anchorPath),
		ReworkDir:    loomengine.LoomReworkDir(location),
		ReworkDirRel: loomengine.LoomReworkDirRel(),
		ReviewsDir:   loomengine.LoomReviewsDir(location),
		// The run subdirectories are the recipe's own run_subdir values for the Plan-Review and Webster-Review segments, the two reviews a generation owns.
		ReviewRunSubdirs: []string{"plan", "webster"},
		ReadCommitted:    committedAnchoredReader(location),
		ArchiveWebster: func(dest string) error {
			return websterengine.ArchiveRunRecord(websterGeom, dest)
		},
		ReadRejection: func() (loomshed.PendingRejection, bool, error) {
			r, found, err := landingshed.ReadRejection(loomengine.LoomRejectionPath(location))
			if err != nil || !found {
				return loomshed.PendingRejection{}, found, err
			}
			return loomshed.PendingRejection{PRNumber: r.PRNumber, HeadSHA: r.HeadSHA, RejectedAt: r.RejectedAt, Findings: r.Findings}, true, nil
		},
		ClearRejection: func() error {
			return landingshed.RemoveRecord(loomengine.LoomRejectionPath(location))
		},
		Commit: func() error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, reworkCommitPathspec(location), fmt.Sprintf("loom: rework round for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())
			return err
		},
	}

	parentReviewCfg, err := newParentReviewConfig(location, c.runID, loomCfg, websterGeom.StencilsDir)
	if err != nil {
		return err
	}

	c.env = shedrecipe.Env{
		ParentReview:       parentReviewCfg,
		Cwd:                cwd,
		AnchorPath:         anchorPath,
		WorktreeRoot:       location.WorktreePath(),
		StatusPath:         statusPath,
		StatusLockPath:     statusLockPath,
		DecisionRecordPath: loomengine.DiscussionDecisionRecord(location),
		SupportLogPath:     loomengine.DiscussionSupportLog(location),
		WebsterDeps:        runDeps,
		// ReflectFriction is a method value over the receiver, so frictionDir and armedVerb are read
		// when the row runs, not when wire runs.
		ReflectFriction: c.reflectFrictionRow,
		// WebsterRun is set explicitly to websterengine.Run, per the
		// env-webster-run-is-filled-explicitly Shared Decision: websterEntry errors on a nil
		// WebsterRun, unlike loomshed.Deps.WebsterRun, which shedadapters.NewWebsterProducer
		// defaulted when left nil.
		WebsterRun: websterengine.Run,
		// CommitWebster mirrors CommitPlan below: the pathspec is webster's whole durable
		// directory, so Master's outcome.yaml and summary.md and the integration report land in
		// git rather than as untracked dirt that refuses the task worktree's removal.
		// The plan directory rides along because webster rewrites card files during the run
		// (handle binding, handle canonicalization);
		// PR-Rework archives the plan at its working-tree state, so a rewrite left uncommitted would be archived without ever having been committed in place.
		CommitWebster: func() error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{websterengine.DirRel(), planparser.PlanDirRel()}, fmt.Sprintf("loom: webster run record for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())
			return err
		},
		// Shuttle is runner, already built above: *shuttleengine.Runner already satisfies
		// shedadapters.Shuttle, and row 3 (Discussion-Write) reads it now.
		Shuttle: runner,
		// DiscussionSpec is evaluated per Call, not resolved here, so the stencil is read at call
		// time -- what the Stencil Ownership Invariant requires. autonomous is now
		// !loomCfg.DiscussionInteractive, read fresh on every wire() call. Nothing compares it
		// against the mode a live run was started with: the resume decision is made purely on
		// live-agent evidence, so flipping the key between a crash and a resume is permitted and
		// benign -- it means only that the next spawn is interviewed differently.
		DiscussionSpec: func() (shuttleengine.Spec, error) {
			return loomengine.DiscussionSpec(location, websterGeom.StencilsDir, loomCfg, registry, seedSlug(location.WorktreeName), !loomCfg.DiscussionInteractive)
		},
		// CommitDiscussion mirrors the seed commit start.go already performs, including its
		// NewMutations("") record and its EnvSyncOptions(). The pathspec is the whole discussion
		// directory deliberately, so archiveStaleOutputs' timestamped siblings are committed rather
		// than left as untracked dirt. A second Done over already-committed artifacts is a
		// no-op rather than an error: CommitAnchoredPaths reports committed == false for an
		// already-clean, already-tracked path, and this closure discards that result alongside the
		// sha, returning only the error -- and this idempotence now covers two callers rather than
		// one, since the Discussion-Bouncer row's approved settle reaches this same closure through
		// the row's commit_seam: discussion config key.
		CommitDiscussion: func() error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, discussionCommitPathspec(location),fmt.Sprintf("loom: discussion artifacts for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())
			return err
		},
		// DescriptionPath is the change description Describe writes and its gate and the landing
		// rows read.
		DescriptionPath: summaryparser.Path(loomengine.LandingDir(location)),
		// DescribeSpec is evaluated per Call, so the stencil is read at call time. It opens the
		// fabric lazily: wire() also runs for status/pause, where opening the fabric must not
		// happen (the OpenBisector hazard above).
		DescribeSpec: func() (shuttleengine.Spec, error) {
			handle, err := fabricengine.Open(location)
			if err != nil {
				return shuttleengine.Spec{}, err
			}
			taskBranch, parentBranch, err := landingBranches(handle, location)
			if err != nil {
				return shuttleengine.Spec{}, err
			}
			priorDirs, err := loomshed.ArchivedWebsterDirs(loomengine.LoomReworkDir(location))
			if err != nil {
				return shuttleengine.Spec{}, err
			}
			var priorRecords []string
			for _, dir := range priorDirs {
				record := summaryparser.Path(dir)
				_, err := os.Stat(record)
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				if err != nil {
					return shuttleengine.Spec{}, fmt.Errorf("loom: describe: stat prior run record %s: %w", record, err)
				}
				priorRecords = append(priorRecords, record)
			}
			return landingshed.DescribeSpec(landingshed.DescribeInputs{
				StencilsDir:         websterGeom.StencilsDir,
				DecisionRecordPath:  loomengine.DiscussionDecisionRecord(location),
				RunRecordPath:       summaryparser.Path(websterGeom.WebsterDir),
				PriorRunRecordPaths: priorRecords,
				DescriptionPath:     summaryparser.Path(loomengine.LandingDir(location)),
				TaskBranch:          taskBranch,
				ParentBranch:        parentBranch,
				Slug:                seedSlug(location.WorktreeName),
			}, landingCfg, registry)
		},
		// CommitDescription mirrors CommitDiscussion, including its discard of (sha, committed),
		// which makes a repeat commit over an unchanged directory a no-op.
		CommitDescription: func() error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{loomengine.LandingDirRel()}, fmt.Sprintf("loom: change description for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())
			return err
		},
		// PlanSpec is evaluated per Call, not resolved here, so the stencil is read at call time --
		// what the Stencil Ownership Invariant requires. Unlike DiscussionSpec beside it, PlanSpec
		// takes no autonomous argument: the Plan producer is autonomous by design and hard-codes
		// Interactive: false internally.
		PlanSpec: func() (shuttleengine.Spec, error) {
			return loomengine.PlanSpec(location, websterGeom.StencilsDir, websterGeom.SpecsDir, loomCfg, registry)
		},
		// CommitPlan mirrors CommitDiscussion above: it keeps the working tree clean for the rows
		// that follow, makes the artifact durable across a crash or a resume, and sweeps the
		// decorator's archive subdirectory into git rather than leaving it as untracked dirt. A
		// second Done over already-committed artifacts is a no-op rather than an error:
		// CommitAnchoredPaths reports committed == false for an already-clean, already-tracked path,
		// and this closure discards that result alongside the sha, returning only the error -- and
		// this idempotence now covers two callers rather than one, since the Plan-Bouncer row's
		// approved settle reaches this same closure through the row's commit_seam: plan config key.
		// The commit message is deliberately shared between the two callers: it names the artifact
		// set rather than the producer that last touched it, so a Plan-Write commit and a
		// Plan-Bouncer commit read identically. Both callers fire this closure only after a gate has
		// already judged the plan: Plan-Write's own gate runs inside its Call, ahead of its own
		// post-Done commit, and Plan-Burler's own gate runs the same way, ahead of the Plan-Bouncer
		// approved settle that commits its overlay fix -- and that is intentional and matches the
		// discussion precedent: the commit keeps the artifact durable, it does not certify it. The
		// pathspec is the whole plan directory via
		// planparser.PlanDirRel(), never a hand-built filepath.Join naming the _lyx literal, which
		// the Lyxdirs Single-Declarer Invariant forbids in production path-construction context.
		CommitPlan: func() error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{planparser.PlanDirRel()}, fmt.Sprintf("loom: plan artifacts for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())
			return err
		},
		// ApprovePlan is what flips approved: true on the Plan-Bouncer row's approved settle. It runs
		// before that row's commit seam (CommitPlan above), so the flag lands inside the commit rather
		// than as working-tree dirt afterwards. It is idempotent -- a second run over an
		// already-approved plan is a successful no-op -- which is what makes the failed-settle resume
		// path converge.
		ApprovePlan: func() error {
			return planparser.SetApproved(planparser.PlanDir(location.AnchorPath()))
		},
		// SkipPlanReview lets Plan-Bouncer approve an exempt rework generation without a judge; it reads committed state on demand and opens nothing at wire time.
		SkipPlanReview: func() (bool, error) { return loomshed.PlanReviewSkippable(reworkDeps) },
		// ReworkSpec is evaluated per Call like PlanSpec above, so the stencil is read at call time.
		// The values the session is told arrive from the PR-Rework producer, which decides them.
		ReworkSpec: func(told loomshed.ReworkTold) (shuttleengine.Spec, error) {
			return loomengine.ReworkSpec(location, websterGeom.StencilsDir, websterGeom.SpecsDir, loomCfg, registry, told.FirstCard, told.PriorPlanDir)
		},
		// Rework opens nothing at wire time: every closure reads or writes on demand, since wire() also runs for status/pause.
		Rework: reworkDeps,
		// StencilsDir, SpecsDir, RunRoot, Burler, and Now are filled for both review segments --
		// Discussion-Bouncer/Discussion-Burler and Plan-Bouncer/Plan-Burler alike. StencilsDir and
		// SpecsDir are websterGeom.StencilsDir and websterGeom.SpecsDir -- the same values the
		// DiscussionSpec and PlanSpec closures above already capture directly -- so each is one
		// value read from one place, not a second copy that could drift from theirs. Now is filled
		// explicitly with time.Now rather than left nil, even though nil defaults to time.Now
		// inside the underlying constructors, because the Bouncer's archive-filename collision
		// suffix is the one place a test wants to inject a clock.
		StencilsDir: websterGeom.StencilsDir,
		SpecsDir:    websterGeom.SpecsDir,
		// RunRoot is durable: the status seam commits it with every transition.
		RunRoot: loomengine.LoomReviewsDir(location),
		Burler:  burlerEngine,
		Now:     time.Now,

		ReviewModel:   reviewSettings.Model,
		ReviewEffort:  reviewSettings.Effort,
		ReviewVersion: reviewSettings.Version,
		ReviewTimeout: reviewSettings.Timeout,

		// Landing is deliberately left unfilled here, for a different reason than the four above:
		// Env.Landing is assembled in run.go, immediately before loomrecipe.New, because
		// NewPublish/NewFinalize both open their fabric pair eagerly at construction, and wire()
		// runs for every verb including "status"/"pause" -- the same OpenBisector hazard the
		// comment above already guards against. See landingDeps (landingdeps.go) and the
		// env-landing-filled-in-run-not-wire design decision.
	}

	// c.shedPaths carries the five told values shedengine.Shed itself reads and no shedrecipe.Env
	// registry entry reads. StatusPath and StatusLockPath are deliberately told twice, once here and
	// once above in c.env -- that duplication is inherent to the split between loomrecipe.New's two
	// argument types and must not be collapsed; loomrecipe.New errors if the two copies disagree.
	// Each pair is filled from the single statusPath/statusLockPath evaluation above rather than a
	// second loomengine accessor call, so the two copies cannot drift here.
	c.shedPaths = shedbuild.ShedPaths{
		StatusPath:     statusPath,
		LockPath:       shedrun.RunLock(location, c.runID),
		StatusLockPath: statusLockPath,
		// MaxBounces is left zero so shedengine.Shed's own default applies. "Default" here means
		// the inherited per-producer default every ProducerDef.MaxBounces of 0 falls back to
		// (which itself falls back to shedengine's internal default of ten), not a run-wide
		// total -- the budget itself is per-producer and episode-scoped, counted from the
		// persisted history rather than held in memory.
		CommitStatus: newCommitStatusSeam(loomCommitStatusDeps(location, c.runID)),

		RunID:                   c.runID,
		MissingStatusWayForward: loomMissingStatusWayForward,
	}

	c.location = location
	c.cwd = cwd
	c.cfg = loomCfg
	c.reed = reedEngine
	c.runDeps = runDeps
	c.registry = registry
	c.runner = runner
	c.landingCfg = landingCfg
	c.frictionDir = frictionDir
	// driverStarter and driverPaneProbe wrap the runner and reed engine already constructed above --
	// no second runner and no second reed engine are constructed here.
	c.driverStarter = runnerDriverStarter{runner: runner}
	c.driverSender = runnerDriverStarter{runner: runner}
	c.driverResumeWait = func() { time.Sleep(driverResumeSendInterval) }
	c.driverPaneProbe = newReedDriverPaneProbe(reedEngine)
	return nil
}

// loomMissingStatusWayForward is the told trailing clause for a missing status file: loom's own start verb is what bootstraps one.
const loomMissingStatusWayForward = "way forward: run \"lyx loom start\" first to bootstrap this task"
