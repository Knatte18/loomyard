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
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/parentreview"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/statuscommit"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/verifytree"
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
	// Push pushes the run records and, in a task pair, the task branch.
	// An error from one side's push names that side.
	// A push lock that stayed busy names no side.
	Push func() error
	// SetBoardStatus writes status onto the run's board entry, leaving an entry that is absent or already done untouched.
	// Nil writes nothing.
	SetBoardStatus func(status string) error
}

// statusCommitPathspec returns the fabric-sibling pathspec one status commit stages: shedrun.StatusRel(location, runID), plus each of loomengine.LoomReviewsDirRel(), loomengine.LoomDurableDirRel() and shedrun.DriveReportsRel(location, runID) when its directory holds at least one non-directory entry anywhere beneath it.
// The loom durable directory is staged whole rather than its friction directory alone:
// staging the parent carries the friction directory and every timestamped archive sibling in one entry,
// where a friction-only entry would miss the archives.
// Existence alone is not enough: shedrecipe's Bouncer and BurlerRound entries os.MkdirAll every review row's run directory at recipe build time,
// so from a run's first transition the reviews root exists holding only empty segment directories.
// `git add -- <dir>` accepts an empty directory,
// but StageAndCommit's `git commit -- <pathspec>` then fails with "pathspec did not match any file(s) known to git", which the seam would turn into a hard error;
// a directory with no file is nothing to commit, never an error, so every absent, unreadable, non-directory or file-less outcome omits the entry.
// The directory pathspec is whole rather than per-file because StageAndCommit runs `git add -- <pathspec>`:
// an archive commits both the new timestamped sibling and the covered files' removal from the friction directory,
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

// planCommitPathspec returns the pathspec the plan commit stages: the plan directory, and webster's durable directory too when it holds a file or Plan-Write's rotation archived a run record under the plan.
// Webster's directory is tracked, so a record moved out of it leaves deletions that only that pathspec stages, together with the archive, in the same commit.
// It is left out otherwise, since git refuses a pathspec that matches nothing in the index or the working tree.
func planCommitPathspec(location *lyxcwd.Location) []string {
	paths := []string{planparser.PlanDirRel()}
	archived := loomshed.ArchivedPlanWebsterDirs(planparser.PlanDir(location.AnchorPath()))
	if holdsFile(websterengine.Dir(location.AnchorPath())) || slices.ContainsFunc(archived, holdsFile) {
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

// loomCommitStatusDeps builds a commitStatusDeps over location and runID, filling each field from fabric: MergeActive from fabricengine.MergeStateActive, Commit from fabricengine.CommitAnchoredPaths scoped to statusCommitPathspec (the status file, plus the review round record, the loom durable directory and the drive reports when each holds a file), and Push from fabricengine.PushPairAnchored, which pushes the run records and, in a task pair, the task branch.
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
			_, err := fabricengine.PushPairAnchored(location, fabricengine.EnvSyncOptions(), fabricengine.StatusPushLockWait)
			return err
		},
		SetBoardStatus: func(status string) error {
			board, err := boardengine.OpenHub(location.HubPath)
			if err != nil {
				return err
			}
			return board.SetRunStatus(shedrun.ResolveRunID(location, runID), status)
		},
	}
}

// boardStatus is the board entry status for a run transition: the shed state and the producer it stands at, so the README shows where each run is.
// A finished run writes nothing, since landing marks the entry done itself.
func boardStatus(producer, state string) (string, bool) {
	if state == string(shedengine.StateDone) {
		return "", false
	}
	return boardengine.RunStatus(state, producer), true
}

// newCommitStatusSeam builds the shedengine.Shed.CommitStatus closure from deps.
// It first writes the run's board status when that status changed, and a failed board write only warns:
// the board is the hub's overview, never the run's own bookkeeping.
// It then calls the shared statuscommit core, which owns the skip-while-mid-merge, commit-hard-errors and push-warns dispositions, with loom's own commit and log prefixes and no after-commit callback.
func newCommitStatusSeam(deps commitStatusDeps) func(producer, state string) error {
	core := statuscommit.New(statuscommit.Deps{
		MergeActive: deps.MergeActive,
		Commit:      deps.Commit,
		Push:        deps.Push,
	}, "loom", "loomcli", nil)
	lastBoardStatus := ""
	return func(producer, state string) error {
		if status, ok := boardStatus(producer, state); ok && deps.SetBoardStatus != nil && status != lastBoardStatus {
			if err := deps.SetBoardStatus(status); err != nil {
				logger.Warn("loomcli: board status write failed, next transition retries", "status", status, "error", err)
			} else {
				lastBoardStatus = status
			}
		}
		return core(producer, state)
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
	c.env.PlanIndex = planglyph.NewSlottedIndex(fabricengine.NewReferenceRule(), hubgeom.GateSlots(location), gateslot.WaitDir(location.AnchorPath()))
}

// committedAnchoredReader returns the seam that reads an anchor-relative file as committed at HEAD for location, with found false when HEAD has no such file.
// Building it opens nothing: the fabric is read on each call.
func committedAnchoredReader(location *lyxcwd.Location) func(anchorRel string) ([]byte, bool, error) {
	return func(anchorRel string) ([]byte, bool, error) {
		return fabricengine.CommittedAnchoredFile(location, anchorRel)
	}
}

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

// commitDiscussion commits the discussion artifacts of location's worktree through the fabric.
// It is the body of the CommitDiscussion seam, shared with `lyx loom decision add`, which commits the record it appends to.
func commitDiscussion(location *lyxcwd.Location) error {
	_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, discussionCommitPathspec(location), fmt.Sprintf("loom: discussion artifacts for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())
	return err
}

// writeCarryOver writes entry into the decision record of location's worktree and commits the record alone.
// It is the body of the CarryOver seam.
// An unchanged record commits nothing, and a write or commit error is returned as the seam's error.
func writeCarryOver(location *lyxcwd.Location, entry discussionparser.CarryOver) error {
	if err := discussionparser.WriteCarryOver(loomengine.DiscussionDecisionRecord(location), entry); err != nil {
		return err
	}
	_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{loomengine.DiscussionDecisionRecordRel()}, fmt.Sprintf("loom: review carry-over for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())
	return err
}

// newParentReviewConfig builds the Discussion-Write parent-review gate's told input.
// The reviewer is the Parent hubgeom.ResolveParent returns, empty when the run has no parent;
// a resolver error is a wiring error.
// ReviewerLive is nil, so the gate treats the reviewer as live, while the parent names no worktree (no parent);
// otherwise it reads the parent worktree's reed session through reedCfg.
func newParentReviewConfig(location *lyxcwd.Location, reedCfg reedengine.Config, cfg loomengine.Config, stencilsDir string) (parentreview.GateConfig, error) {
	parent, err := hubgeom.ResolveParent(location)
	if err != nil {
		return parentreview.GateConfig{}, fmt.Errorf("loom: resolve the parent review reviewer: %w", err)
	}
	var reviewerLive func() (bool, error)
	if parent.Worktree != "" {
		reviewerLive = func() (bool, error) {
			return parentWorktreeReviewerLive(location, reedCfg, parent)
		}
	}
	slug := seedSlug(location.WorktreeName)
	decisionRecord := loomengine.DiscussionDecisionRecord(location)
	supportLog := loomengine.DiscussionSupportLog(location)
	return parentreview.GateConfig{
		Store:          parentreview.Store{Root: loomengine.LoomParentReviewDir(location), LockDir: loomengine.LoomParentReviewLockDir(location)},
		Slug:           slug,
		Reviewer:       parent.Name,
		ReviewerLive:   reviewerLive,
		DecisionRecord: decisionRecord,
		SupportLog:     supportLog,
		WaitBound:      time.Duration(cfg.ParentReviewWaitMin) * time.Minute,
		RenderDelivery: func(briefPath string) (string, error) {
			return loomengine.ParentReviewDeliveryPrompt(stencilsDir, slug, briefPath, parent.Name)
		},
		RenderBrief: func() (string, error) {
			return loomengine.ParentReviewBrief(stencilsDir, slug, decisionRecord, supportLog)
		},
	}, nil
}

// parentWorktreeReviewerLive reports whether parent's reviewer has a live strand in its worktree's reed session.
// The parent worktree is resolved on each call, never at wiring time, so a parent started after the run is seen.
func parentWorktreeReviewerLive(location *lyxcwd.Location, reedCfg reedengine.Config, parent hubgeom.Parent) (bool, error) {
	parentLoc, err := lyxcwd.ResolveWorktree(fabricengine.WorktreePath(location, parent.Worktree))
	if err != nil {
		return false, fmt.Errorf("loom: resolve parent worktree %q: %w", parent.Worktree, err)
	}
	geom, err := hubgeom.ReedGeometry(parentLoc)
	if err != nil {
		return false, fmt.Errorf("loom: reed geometry of parent worktree %q: %w", parent.Worktree, err)
	}
	return strandLive(parent.Name, reedengine.New(reedCfg, geom).Status)
}

// strandLive reports whether status lists a live strand named name.
// An error wrapping reedengine.ErrNoSession answers not live with no error, since a parent with no session has no reviewer;
// any other status error is returned.
func strandLive(name string, status func() (reedengine.StatusResult, error)) (bool, error) {
	res, err := status()
	if errors.Is(err, reedengine.ErrNoSession) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, s := range res.Strands {
		if s.Name == name && s.Live {
			return true, nil
		}
	}
	return false, nil
}

// wire builds the whole engine stack onto c from location and cwd: every module config anchored at
// location.AnchorPath(), the reed engine and shuttle runner, the assembled websterengine.RunDeps, and
// the assembled shedrecipe.Env/shedbuild.ShedPaths pair wrapping it.
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
	// landing is hub-wide: Publish and Finalize read the hub's file, never a pair's copy.
	landingCfg, err := landingshed.LoadConfig(fabricengine.BoardDir(location.HubPath), "landing")
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
	judgeSettings, err := loomengine.ResolveJudge(loomCfg, registry)
	if err != nil {
		return err
	}

	reedGeom, err := hubgeom.ReedGeometry(location)
	if err != nil {
		return err
	}
	c.parentName = reedGeom.ParentName
	reedEngine := reedengine.New(reedCfg, reedGeom)
	claudeEngine := claudeengine.NewFromConfig(shuttleCfg)
	runner := shuttleengine.NewRunner(reedEngine, claudeEngine, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)
	runner.SetNotifier(func(line string) error { return orchcli.NotifyPrime(location, line) })

	websterGeom := hubgeom.WebsterGeometry(location)
	websterGeom.Index = planglyph.NewSlottedIndex(fabricengine.NewReferenceRule(), websterGeom.GateSlots, websterGeom.GateWaitDir)

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
	burlerEngine := burlerengine.New(burlerengine.RunnerShuttle(runner), hubgeom.BurlerGeometry(location), burlerCfg, websterGeom.StencilsDir, frictionDir)

	runDeps := websterengine.RunDeps{
		Starter:    runnerMasterStarter{runner: runner},
		Stopper:    runner,
		Engine:     claudeEngine,
		ShuttleCfg: shuttleCfg,
		Roles:      roles,
		Config:     websterCfg,
		Batcher:    activeBatcher,
		Geom:       websterGeom,
		// FrictionDir covers the Master and verify-fix prompts only: the per-batch fork and recovery
		// prompts are composed in a separate `lyx webster` process (begin-batch/recover-batch), which
		// resolves the same value itself in internal/webstercli, per the
		// webstercli-resolves-the-friction-directory-in-hub-mode Shared Decision.
		FrictionDir: frictionDir,
		// The reference matcher is pinned to a real fabricengine.NewRefScanner(location), built
		// eagerly because that constructor only compiles a regexp and cannot fail. It must never be
		// the never-matching stand-in: that stand-in is permitted only in standalone, where there is
		// no wired fabric for the guard to protect, and loom is hub-only.
		RefMatcher: fabricengine.NewRefScanner(location),
		// ParentBranch lets the verify gate's fix-commit check accept a clean parent merge made while fixing, as webstercli's own wiring does.
		ParentBranch: func() (string, error) {
			origin, found, err := fabricengine.ReadOrigin(location)
			if err != nil {
				return "", err
			}
			if !found {
				return "", fmt.Errorf("the pair has no fabric origin record")
			}
			return origin.ParentBranch, nil
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

	parentReviewCfg, err := newParentReviewConfig(location, reedCfg, loomCfg, websterGeom.StencilsDir)
	if err != nil {
		return err
	}

	c.env = shedrecipe.Env{
		ParentReview: parentReviewCfg,
		Cwd:          cwd,
		AnchorPath:   anchorPath,
		WorktreeRoot: location.WorktreePath(),
		VerifyDir:    verifytree.Dir(anchorPath),
		GateSlots:    hubgeom.GateSlots(location),
		PublishFailure: func() string {
			return loomshed.PublishFailureNote(location.WorktreePath(), verifytree.Dir(anchorPath))
		},
		StatusPath:         statusPath,
		StatusLockPath:     statusLockPath,
		DecisionRecordPath: loomengine.DiscussionDecisionRecord(location),
		SupportLogPath:     loomengine.DiscussionSupportLog(location),
		ParentName:         c.parentName,
		WebsterDeps:        runDeps,
		PlanIndex:          websterGeom.Index,
		// ReflectFriction is a method value over the receiver, so frictionDir is read when the row runs, not when wire runs.
		ReflectFriction: c.reflectFrictionRow,
		// WebsterRun is set explicitly to websterengine.Run, per the
		// env-webster-run-is-filled-explicitly Shared Decision: websterEntry errors on a nil
		// WebsterRun, unlike loomshed.Deps.WebsterRun, which shedadapters.NewWebsterProducer
		// defaulted when left nil.
		WebsterRun: websterengine.Run,
		// CommitWebster mirrors CommitPlan below: the pathspec is webster's whole durable
		// directory, so Master's outcome.yaml and summary.md and the verify gate's report land in
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
		// Seats runs the MultiLLM rows' seats over the same runner.
		Seats: seatengine.New(seatengine.RunnerShuttle(runner), seatengine.Geometry{
			WorktreeRoot: location.WorktreePath(),
			AnchorPath:   anchorPath,
			StencilsDir:  websterGeom.StencilsDir,
			ParentName:   c.parentName,
			Shortname:    reedGeom.NameShortname,
			Slug:         reedGeom.NameSlug,
		}),
		Models: registry,
		// DiscussionSpec is evaluated per Call, not resolved here, so the stencil is read at call
		// time -- what the Stencil Ownership Invariant requires. autonomous is now
		// !loomCfg.DiscussionInteractive, read fresh on every wire() call. Nothing compares it
		// against the mode a live run was started with: the resume decision is made purely on
		// live-agent evidence, so flipping the key between a crash and a resume is permitted and
		// benign -- it means only that the next spawn is interviewed differently.
		DiscussionSpec: func() (shuttleengine.Spec, error) {
			return loomengine.DiscussionSpec(location, websterGeom.StencilsDir, c.parentName, loomCfg, registry, seedSlug(location.WorktreeName), !loomCfg.DiscussionInteractive)
		},
		// DiscussionTable is evaluated per Call for the same stencil-ownership reason as DiscussionSpec.
		// It is wired whatever discussion_producer holds; only a row built on DiscussionSeats evaluates it.
		DiscussionTable: func() (seatengine.Table, error) {
			return loomengine.DiscussionTable(location, websterGeom.StencilsDir, loomCfg, registry, seedSlug(location.WorktreeName))
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
		CommitDiscussion: func() error { return commitDiscussion(location) },
		// DescriptionPath is the change description Describe writes and its gate and the landing
		// rows read.
		DescriptionPath: summaryparser.Path(loomengine.LandingDir(location)),
		// DescribeSpec is evaluated per Call, so the stencil is read at call time. It opens the
		// fabric lazily: wire() also runs for status/pause, where opening the fabric must not
		// happen: opening stat-checks the paired sibling, and this pre-run must not fail
		// "status"/"pause" against a healthy-but-unwired location.
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
				ParentName:          c.parentName,
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
			return loomengine.PlanSpec(location, websterGeom.StencilsDir, websterGeom.SpecsDir, c.parentName, loomCfg, registry)
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
		// the Lyxdirs Single-Declarer Invariant forbids in production path-construction context;
		// planCommitPathspec adds webster's durable directory when Plan-Write's rotation moved a run record out of it.
		CommitPlan: func() error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, planCommitPathspec(location), fmt.Sprintf("loom: plan artifacts for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())
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
		// CarryOver files each review segment's still-open findings in the decision record and commits that record alone, so no other dirty file rides along.
		CarryOver: func(entry discussionparser.CarryOver) error { return writeCarryOver(location, entry) },
		// SkipPlanReview lets Plan-Bouncer approve an exempt rework generation without a judge; it reads committed state on demand and opens nothing at wire time.
		SkipPlanReview: func() (bool, error) { return loomshed.PlanReviewSkippable(reworkDeps) },
		// ReworkSpec is evaluated per Call like PlanSpec above, so the stencil is read at call time.
		// The values the session is told arrive from the PR-Rework producer, which decides them.
		ReworkSpec: func(told loomshed.ReworkTold) (shuttleengine.Spec, error) {
			return loomengine.ReworkSpec(location, websterGeom.StencilsDir, websterGeom.SpecsDir, c.parentName, loomCfg, registry, told.FirstCard, told.PriorPlanDir)
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

		// Slug and SegmentBounces tell each Bouncer row the verbs' slug and the live bounce budget its CIRCLING Reason names.
		Slug:           seedSlug(location.WorktreeName),
		SegmentBounces: segmentBounces(statusPath, statusLockPath, loomCfg.ReviewMaxBounces),

		ReviewMaxBounces:         loomCfg.ReviewMaxBounces,
		ReviewCirclingCheckpoint: loomCfg.ReviewCirclingCheckpoint,

		ReviewModels:  reviewSettings.Models,
		ReviewTimeout: reviewSettings.Timeout,
		FixStart:      reviewSettings.FixStart,
		RowReviewModels: map[string]burlerengine.RoundModels{
			loomshed.NameDiscussionBurler: reviewSettings.Discussion,
			loomshed.NamePlanBurler:       reviewSettings.Plan,
			loomshed.NameWebsterBurler:    reviewSettings.Webster,
		},
		// A segment's fan reaches both of its rows from one key, so its Bouncer and its round agree by construction.
		// Webster-Review stays solo: its forks may run no git and its subject exists only through git.
		RowClusterFans: map[string]string{
			loomshed.NameDiscussionBouncer: loomCfg.DiscussionFan,
			loomshed.NameDiscussionBurler:  loomCfg.DiscussionFan,
			loomshed.NamePlanBouncer:       loomCfg.PlanFan,
			loomshed.NamePlanBurler:        loomCfg.PlanFan,
		},

		JudgeModel:   judgeSettings.Model,
		JudgeEffort:  judgeSettings.Effort,
		JudgeVersion: judgeSettings.Version,

		// Landing is deliberately left unfilled here, for a different reason than the four above:
		// Env.Landing is assembled in run.go, immediately before loomrecipe.New, because
		// NewPublish/NewFinalize both open their fabric pair eagerly at construction, and wire()
		// runs for every verb including "status"/"pause" -- the same open-at-wire hazard the
		// DescribeSpec comment above guards against. See landingDeps (landingdeps.go) and the
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
	c.reflectionShuttle = runner
	c.landingCfg = landingCfg
	c.frictionDir = frictionDir
	// driverStarter and driverPaneProbe wrap the runner and reed engine already constructed above --
	// no second runner and no second reed engine are constructed here.
	c.driverStarter = runnerDriverStarter{runner: runner}
	c.driverSender = runnerDriverStarter{runner: runner}
	c.driverResumeWait = func() { time.Sleep(driverResumeSendInterval) }
	c.driverPaneProbe = newReedDriverPaneProbe(reedEngine)
	c.driverDirectory = newReedDriverDirectory(reedEngine)
	c.bouncerSubdir = loomrecipe.BouncerRunSubdir
	return nil
}

// segmentBounces returns the Env.SegmentBounces seam over the status file at statusPath, locked by statusLockPath.
// The history is read on each call, never at wire time, because it grows during the run; an absent status file reports not-in-segment.
// reviewMaxBounces is the budget loomrecipe.New applies to the review rows, so the reported budget is the one Shed blocks on.
func segmentBounces(statusPath, statusLockPath string, reviewMaxBounces int) func(row string) (int, int, bool, error) {
	return func(row string) (int, int, bool, error) {
		st, found, err := state.ReadJSONStrict[shedengine.Status](statusPath, statusLockPath)
		// A run directory not yet created fails the lock open with not-exist; that is an absent status file too.
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return 0, 0, false, err
		}
		if err != nil || !found {
			return 0, 0, false, nil
		}
		routing, err := loomrecipe.Routing(reviewMaxBounces)
		if err != nil {
			return 0, 0, false, err
		}
		count, budget, inSegment := routing.Bounces(row, st.History)
		return count, budget, inSegment, nil
	}
}

// loomMissingStatusWayForward is the told trailing clause for a missing status file: loom's own start verb is what bootstraps one.
const loomMissingStatusWayForward = "way forward: run \"lyx loom start\" first to bootstrap this task"
