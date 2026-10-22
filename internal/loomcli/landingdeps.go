// landingdeps.go declares landingDeps, the assembly seam that builds a landingshed.Deps struct from
// every value already resolved by run.go. landingDeps performs no I/O of any kind -- it exists so
// the drift-guard test in landingdeps_test.go stays Tier 1 with no hubforge fixture, mirroring
// wiring.go's own header-comment convention for why wire is extracted.

package loomcli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/alignment"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// landingDeps assembles a landingshed.Deps from values already resolved by the caller (run.go),
// per the assembly-seam-takes-plain-values decision: every argument arrives already resolved, and
// this function does no I/O and returns no error.
//
// runner assigns directly into the Shuttle mergeresolve.Shuttle field: *shuttleengine.Runner already
// satisfies that interface, per the existing compile-time assertion at
// internal/mergeresolve/deps.go:46.
func landingDeps(
	l *lyxcwd.Location,
	recipe string,
	geom websterengine.Geometry,
	taskBranch, originURL, parentBranch string,
	pushSkipped bool,
	pushBranch func() error,
	registry modelspec.Registry,
	runner *shuttleengine.Runner,
	cfg landingshed.Config,
	parentName string,
	languageSkills []string,
	verifyWaitMark func(label string, start time.Time) error,
	stopConflictSession func() (string, error),
	verify verifySource,
	lock landingshed.ParentLockDeps,
	runStart time.Time,
) landingshed.Deps {
	return landingshed.Deps{
		ParentName:      parentName,
		LanguageSkills:  languageSkills,
		WorktreeRoot:    l.WorktreePath(),
		TaskBranch:      taskBranch,
		ParentBranch:    parentBranch,
		DescriptionPath: summaryparser.Path(loomengine.LandingDir(l)),
		StencilsDir:     geom.StencilsDir,
		ScratchDir:      loomengine.LoomScratchDir(l),
		OriginURL:       originURL,
		PushSkipped:     pushSkipped,
		PushBranch:      pushBranch,
		RemoteOnlyCommits: func() (string, []string, error) {
			f, err := fabricengine.Open(l)
			if err != nil {
				return "", nil, err
			}
			return f.RemoteOnlyCommits()
		},
		OpenFabric: func() (*fabricengine.Fabric, error) {
			return fabricengine.Open(l)
		},
		OpenParentFabric: func() (*fabricengine.Fabric, error) {
			return fabricengine.OpenParent(l, parentBranch)
		},
		// CommitStatus commits loom's own phase-machine status file, which Shed rewrites on every
		// producer transition. The per-transition Shed.CommitStatus seam (wiring.go) already keeps
		// the status file current on the ordinary path, so both landing producers calling this
		// immediately before they merge is retained only as the sole protection if a product wires
		// Shed.CommitStatus as nil -- not because the merge guard still inspects the local-only side
		// of the pair, which this task removed from the merge participants entirely, so the guard no
		// longer looks at it.
		//
		// It mirrors wiring.go's CommitDiscussion/CommitPlan closures in every respect: the same
		// CommitAnchoredPaths call, the same throwaway mutation recorder, the same EnvSyncOptions,
		// and the same discard of the (sha, committed) pair in favour of the error alone, which is
		// what makes a second call over an already-committed, already-clean path a no-op rather
		// than a failure. The pathspec is shedrun.StatusRel(l, shedrun.SelfRunID), never a hand-built
		// join naming the _lyx literal, which the Lyxdirs Single-Declarer Invariant forbids.
		CommitStatus: func() error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), l, []string{shedrun.StatusRel(l, shedrun.SelfRunID)}, fmt.Sprintf("loom: status checkpoint for %s", seedSlug(l.WorktreeName)), fabricengine.EnvSyncOptions())
			return err
		},
		CommitParentRecords: parentRecordsCommitter(l, seedSlug(l.WorktreeName)),
		Reconcile:           runReconciler(l, seedSlug(l.WorktreeName)),
		Slug:                seedSlug(l.WorktreeName),
		// ParentHoldsTaskWork marks a Finalize past its parent-side merge, read over the prime and this pair.
		ParentHoldsTaskWork: func() (bool, error) {
			prime, err := primeOfHub(l)
			if err != nil {
				return false, err
			}
			return fabricengine.ParentHoldsTaskWork(prime, l, seedSlug(l.WorktreeName))
		},
		ParentOwner: parentOwner(l, seedSlug(l.WorktreeName)),
		// The origin parent's work and the parent commits origin lacks are read over the parent's own pair, which Finalize's push failure describes.
		OriginParentHoldsWork: func() (bool, error) {
			f, err := fabricengine.Open(l)
			if err != nil {
				return false, err
			}
			tips, err := f.ReadParentTips(parentBranch, true)
			if err != nil {
				return false, err
			}
			if tips.Upstream == "" {
				return false, nil
			}
			return f.HoldsWork(tips.Upstream, "refs/heads/"+taskBranch)
		},
		ParentCommitsOriginLacks: func() ([]string, error) {
			parent, err := fabricengine.OpenParent(l, parentBranch)
			if err != nil {
				return nil, err
			}
			return parent.LocalOnlyCommits(parentBranch)
		},
		// The pre-merge step probes the pair's merge state, aborts the row's own parked merge-in and stops the run's conflict sessions first.
		MergeState: func() (fabricengine.MidMergeState, error) {
			return fabricengine.MidMerge(l)
		},
		AbortMerge: func() error {
			f, err := fabricengine.Open(l)
			if err != nil {
				return err
			}
			_, err = f.MergeAbort()
			return err
		},
		StopConflictSession: stopConflictSession,
		ApprovalPath:        loomengine.LoomApprovalPath(l),
		RejectionPath:       loomengine.LoomRejectionPath(l),
		TaskHead: func() (string, error) {
			f, err := fabricengine.Open(l)
			if err != nil {
				return "", err
			}
			return f.HeadSHA()
		},
		// MarkTaskDone marks this worktree's board task done once its work has landed.
		// landingshed itself names no board path.
		MarkTaskDone: func() error {
			board, err := boardengine.OpenHub(l.HubPath)
			if err != nil {
				return err
			}
			done := "done"
			return board.SetStatus(seedSlug(l.WorktreeName), &done)
		},
		// ConfigChanges reads the task's changes to its per-worktree config files since its fork point.
		// A hub-wide module's file is never read: its worktree copy is not the one in force.
		ConfigChanges: func() (fabricengine.ConfigChanges, error) {
			return fabricengine.ReadConfigChanges(l, taskBranch, parentBranch, perWorktreeConfigRels())
		},
		// Notify posts one orch notice as creel mail for the hub's prime, resolved at call time.
		// A prime with no orch strand recorded is not an error: the notice is logged instead.
		Notify: func(line string) error {
			_, err := orchcli.NotifyPrime(l, loomProgram, line)
			return err
		},
		VerifyCommand:          verify.command,
		PublishCommand:         verify.publishCommand,
		BaseCommand:            verify.baseCommand,
		GateTiers:              verify.gateTiers,
		VerifyFailedWayForward: verify.failedWayForward,
		VerifyFailureTarget:    verify.failureTarget,
		ClassifyVerifyFailure:  verify.classify,
		PublishSkip:            verify.publishSkip,
		FailingTests:           failingTestsOf,
		VerifyDir:              verifytree.Dir(l.AnchorPath()),
		VerifyWaitMark:         verifyWaitMark,
		VerifyHold:             verifytree.NewPauseHold(runner.PauseGeometry(), holdsParentLock(lock.Store, l.WorktreeName)),
		Shuttle:                runner,
		EnginesRun:             runner.EnginesRun,
		Registry:               registry,
		Config:                 cfg,
		Lock:                   lock,
		RunStart:               runStart,
		// The parent reads run over the task pair, opened at call time like the other fabric seams.
		ReadParentTips: parentTipsReader(l, parentBranch),
		HoldsWork: func(base, source string) (bool, error) {
			f, err := fabricengine.Open(l)
			if err != nil {
				return false, err
			}
			return f.HoldsWork(base, source)
		},
		ChangedPaths: func(from, to string) ([]string, error) {
			f, err := fabricengine.Open(l)
			if err != nil {
				return nil, err
			}
			return f.ChangedPaths(from, to)
		},
		MergeBase: func(ref string) (string, error) {
			f, err := fabricengine.Open(l)
			if err != nil {
				return "", err
			}
			return f.MergeBase(ref)
		},
		MergeRecordPath:   landingshed.MergeRecordPath(loomengine.LoomScratchDir(l)),
		MergeReviewedPath: landingshed.MergeReviewedPath(loomengine.LoomScratchDir(l)),
		Alignment:         landingAlignmentDeps(l, recipe, geom.StencilsDir, registry, runner, runner.PauseGeometry()),
	}
}

// parentTipsReader returns the parent-tip reader the landing and the darn rows share:
// it opens the task pair's fabric at call time and reads parentBranch's tips, fetching the remotes first when fetch is true.
func parentTipsReader(l *lyxcwd.Location, parentBranch string) func(fetch bool) (fabricengine.ParentTips, error) {
	return func(fetch bool) (fabricengine.ParentTips, error) {
		f, err := fabricengine.Open(l)
		if err != nil {
			return fabricengine.ParentTips{}, err
		}
		return f.ReadParentTips(parentBranch, fetch)
	}
}

// mergeableTips returns the parent tips a Merge-In would merge: the local tip alone when pushes are skipped, and the local tip and the upstream tip otherwise, empty tips dropped.
func mergeableTips(tips fabricengine.ParentTips, pushSkipped bool) []string {
	candidates := []string{tips.Local}
	if !pushSkipped {
		candidates = append(candidates, tips.Upstream)
	}
	var merged []string
	for _, tip := range candidates {
		if tip != "" {
			merged = append(merged, tip)
		}
	}
	return merged
}

// alignmentNoteFile is the alignment note's file name in the PR-Gate point's alignment directory; the attempt listing skips a file there.
const alignmentNoteFile = "note.json"

// landingAlignmentDeps builds PR-Align's seams for the worktree l.
// Its Config loads loom's config on each call, so an edit to skip_alignment or alignment_pr_approve takes effect at the next step;
// its session works under the run's PR-Gate alignment directory, which also holds the alignment note.
// A darn run's prompt names no plan directory and carries darn's measure; every other recipe keeps the plan directory and loom's measure.
func landingAlignmentDeps(l *lyxcwd.Location, recipe, stencilsDir string, registry modelspec.Registry, runner *shuttleengine.Runner, pause pauserecord.Geometry) landingshed.AlignmentDeps {
	root := shedrun.AlignmentDir(l, shedrun.SelfRunID, string(alignment.PointPRGate))
	slug := seedSlug(l.WorktreeName)
	planDir, measure := planparser.PlanDir(l.AnchorPath()), alignment.MeasureLoom
	if recipe == shedrun.RecipeDarn {
		planDir, measure = "", alignment.MeasureDarn
	}
	return landingshed.AlignmentDeps{
		Config: func() (landingshed.AlignmentConfig, error) {
			cfg, err := loomengine.LoadConfig(fabricengine.ConfigDirs(l), "loom")
			if err != nil {
				return landingshed.AlignmentConfig{}, err
			}
			if alignmentOff(l, cfg) {
				return landingshed.AlignmentConfig{Off: true}, nil
			}
			settings, err := alignmentSettings(cfg, registry)
			if err != nil {
				return landingshed.AlignmentConfig{}, err
			}
			return landingshed.AlignmentConfig{PRApprove: cfg.AlignmentPRApprove, Settings: settings}, nil
		},
		Session: alignment.NewSession(alignment.SessionConfig{
			Root:        root,
			StencilsDir: stencilsDir,
			Shuttle:     alignment.RunnerShuttle(runner),
			Now:         time.Now,
			Pause:       pause,
		}),
		Snapshot: alignmentSnapshot(loomengine.DiscussionDecisionRecord(l), slug, hubBoardTasks(l)),
		Prompt: alignment.PromptInput{
			Slug:               slug,
			Worktree:           l.WorktreePath(),
			DecisionRecordPath: loomengine.DiscussionDecisionRecord(l),
			SupportLogPath:     loomengine.DiscussionSupportLog(l),
			PlanDir:            planDir,
			MeasureStencil:     measure,
		},
		NotePath: filepath.Join(root, alignmentNoteFile),
	}
}

// pauseCovered returns the parent lock store's Covered seam: whether the hub's pause record covers a slug now.
// A geometry or record that cannot be read covers nothing, so a corrupt file never freezes the lock queue.
func pauseCovered(l *lyxcwd.Location) func(slug string) bool {
	return func(slug string) bool {
		geometry, err := hubgeom.PauseGeometry(l)
		if err != nil {
			logger.Warn("loomcli: pause geometry unreadable, treating the run as uncovered", "slug", slug, "cause", err)
			return false
		}
		coverage, err := pauserecord.ReadCoverage(geometry.RecordPath, slug, time.Now())
		if err != nil {
			logger.Warn("loomcli: pause record unreadable, treating the run as uncovered", "slug", slug, "cause", err)
			return false
		}
		return coverage.Covered
	}
}

// parentLockDeps builds the parent lock seams for the run runID of the worktree l.
// The store sits in the hub-level ephemeral tree, reached by path and never junctioned into a pair;
// the wait record lives in the run's scratch directory, and the wait ends when the run status file's pause_after, read under its status lock, names the run's current producer: the lock-taking row waiting in the queue.
// The store reads the pause record to tell which slug a pause covers; an unreadable record covers nothing and logs one Warn.
func parentLockDeps(l *lyxcwd.Location, runID, statusPath, statusLockPath string) landingshed.ParentLockDeps {
	return landingshed.ParentLockDeps{
		Store: &parentlock.Store{
			Dir:     parentlock.Dir(fabricengine.BoardDir(l.HubPath)),
			Covered: pauseCovered(l),
		},
		Owner:    loopLockOwner(l, runID),
		WaitPath: parentlock.WaitRecordPath(loomengine.LoomScratchDir(l)),
		PauseRequested: func() (bool, error) {
			status, found, err := state.ReadJSONNoMkdir[shedengine.Status](statusPath, statusLockPath)
			if err != nil || !found {
				return false, err
			}
			return status.PauseAfter != "" && status.PauseAfter == status.CurrentProducer, nil
		},
	}
}

// holdsParentLock returns the function that reports whether a live parent lock of store is owned by slug, which a verify hold reads on every poll.
// A nil store reads not held, and an error of the store reads not held, logged once per distinct error.
func holdsParentLock(store *parentlock.Store, slug string) func() bool {
	var (
		mutex     sync.Mutex
		lastError string
	)
	return func() bool {
		if store == nil {
			return false
		}
		held, err := store.HeldBy(slug)
		mutex.Lock()
		defer mutex.Unlock()
		if err != nil {
			if err.Error() != lastError {
				lastError = err.Error()
				logger.Warn("loomcli: the parent locks cannot be listed, so the verify hold reads none held", "cause", err)
			}
			return false
		}
		lastError = ""
		return held
	}
}

// loopLockOwner returns the seam that names the parent lock's owner for the run runID of the worktree l.
// The process is the run's detached loop when its pid record names a live loop, so the lock follows the loop across its step children;
// otherwise it is the calling step process, so a step run outside a loop holds the lock only while it lives.
// The teardown's mark in the pid file names no loop.
func loopLockOwner(l *lyxcwd.Location, runID string) func() (parentlock.Owner, error) {
	return func() (parentlock.Owner, error) {
		process := proc.SelfRecord()
		rec, found, err := shedverbs.ReadLoopPIDRecord(shedrun.LoopPIDFile(l, runID))
		if err != nil {
			logger.Warn("loomcli: loop pid record unreadable, taking the parent lock as the calling process", "cause", err)
		} else if found && rec.LoopID != shedverbs.TeardownLoopID && parentlock.ProcessAlive(rec.Loop.PID, rec.Loop.StartTime) {
			process = rec.Loop
		}
		owner := runLockIdentity(l, runID)
		owner.PID = process.PID
		owner.StartTime = process.StartTime
		return owner, nil
	}
}

// verifySource pairs the readers of the gate calls and the failed-verify way-forward clause one recipe tells its verify gate and landing producers.
type verifySource struct {
	// command returns the call the landing sites run, read each time it is called.
	command func() (string, error)
	// publishCommand returns the call of the `publish` gate, "" for an empty list.
	publishCommand func() (string, error)
	// baseCommand returns the call whose verified-tree entry every landing and publish pass keeps, and the round gate's told command.
	baseCommand func() (string, error)
	// gateTiers lists the tier numbers of a named gate, read from the hub's gate config at each call.
	gateTiers func(gate string) ([]int, error)
	// failedWayForward is the clause a recorded failed-verify Stuck reason ends with.
	failedWayForward string
	// failureTarget is the row the recipe routes a failed Publish verify back to.
	failureTarget string
	// classify chooses a failure record's target and the reason's way forward per failure, in place of failedWayForward and failureTarget.
	// Nil for loom's source.
	classify func(ctx context.Context, failure verifytree.PublishFailure) landingshed.FailureRoute
	// publishSkip answers whether Publish skips the `publish` gate for this call, read at each Publish.
	// Nil for loom's source, which never skips.
	publishSkip func() (bool, error)
}

// planVerifySource returns loom's verify source: the hub's `landing`, `publish` and `final` gate calls, and the Webster-Burler-Review route back to a fix.
// Each reader loads the hub's gate config over the pair's config dirs each time it is called, never at construction:
// landingDeps runs at bootstrap, so a gate.yaml edit between two steps must take effect.
// A config or gate-list refusal is returned with its own way forward, not mapped to an empty command.
func planVerifySource(l *lyxcwd.Location) verifySource {
	dirs := fabricengine.ConfigDirs(l)
	return verifySource{
		command:          gateCallSource(dirs, "landing"),
		publishCommand:   gateCallSource(dirs, "publish"),
		baseCommand:      gateCallSource(dirs, "final"),
		gateTiers:        gateTiersLookup(dirs),
		failedWayForward: `run "lyx loom goto --to Webster-Burler-Review", then "lyx loom resume", in the task worktree; the Webster-Review round reads the Publish failure record and its gate runs the failing tests`,
		failureTarget:    loomshed.NameWebsterBurlerReview,
	}
}

// parentRecordsCommitter returns the seam Finalize commits the parent pair's own run records through before its parent-side merge.
// The parent pair is resolved at call time, so a parent that came or went after bootstrap is read as it stands.
// A pair with no parent, and a parent whose run-records directory holds no file, commit nothing: git refuses a pathspec that matches nothing.
// slug names the landing run in the commit message.
func parentRecordsCommitter(l *lyxcwd.Location, slug string) func() error {
	return func() error {
		parent, err := hubgeom.ResolveParent(l)
		if err != nil {
			return fmt.Errorf("resolve the parent pair: %w", err)
		}
		if parent.Worktree == "" {
			return nil
		}
		parentLocation, err := lyxcwd.ResolveWorktree(fabricengine.WorktreePath(l, parent.Worktree))
		if err != nil {
			return fmt.Errorf("locate the parent pair %q: %w", parent.Worktree, err)
		}
		runsRoot := shedrun.RunsRootRel()
		if !holdsFile(filepath.Join(parentLocation.AnchorPath(), runsRoot)) {
			return nil
		}
		_, _, err = fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), parentLocation, []string{runsRoot}, loomengine.LandingCheckpointSubject(slug), fabricengine.EnvSyncOptions())
		return err
	}
}

// runReconciler returns the seam the landing rows judge the run against origin through, the run-scope origin reconcile of slug.
// The prime is resolved at call time, and the seams come from pairteardown over it with the hub's options.
func runReconciler(l *lyxcwd.Location, slug string) func() (fabricengine.OriginReport, error) {
	return func() (fabricengine.OriginReport, error) {
		prime, err := primeOfHub(l)
		if err != nil {
			return fabricengine.OriginReport{}, err
		}
		teardown, err := pairteardown.New(prime)
		if err != nil {
			return fabricengine.OriginReport{}, err
		}
		report, err := fabricengine.ReconcileOrigin(prime, fabricengine.ReconcileScope{Run: slug}, teardown.OriginSeams(pairteardown.HubSeamOptions(prime)), fabricengine.ReconcileOptions{})
		return reportOfRun(report, slug), err
	}
}

// parentOwner returns the seam Finalize reads the owner of the parent of the run slug through.
// The parent's slug is resolved from the pair's origin record, and its owner is read from the local board the run's reconcile left:
// the prime owner for the prime, and the parent pair's effective owner otherwise.
// A parent no other machine owns reads as owned here.
func parentOwner(l *lyxcwd.Location, slug string) func() (landingshed.ParentOwnership, error) {
	return func() (landingshed.ParentOwnership, error) {
		here, err := machinename.Name()
		if err != nil {
			return landingshed.ParentOwnership{}, err
		}
		prime, err := primeOfHub(l)
		if err != nil {
			return landingshed.ParentOwnership{}, err
		}
		parentOf := func(slug string) (string, error) {
			return fabricengine.ParentSlug(prime, slug)
		}
		parentSlug, err := parentOf(slug)
		if err != nil {
			return landingshed.ParentOwnership{}, err
		}
		owners, err := boardengine.ReadOwners(fabricengine.BoardDir(l.HubPath))
		if err != nil {
			return landingshed.ParentOwnership{}, err
		}
		owner := owners.Prime
		if parentSlug != "" {
			if owner, err = owners.Effective(parentSlug, parentOf); err != nil {
				return landingshed.ParentOwnership{}, err
			}
		}
		return landingshed.ParentOwnership{Slug: parentSlug, Owner: owner, Here: !boardengine.OwnedElsewhere(owner, here)}, nil
	}
}

// reportOfRun returns report with its entries narrowed to the one of slug.
// A run-scope report also classifies the run's parent pair, whose stop or end is not the run's own.
func reportOfRun(report fabricengine.OriginReport, slug string) fabricengine.OriginReport {
	own := report.Entries[:0:0]
	for _, entry := range report.Entries {
		if entry.Slug == slug {
			own = append(own, entry)
		}
	}
	report.Entries = own
	return report
}

// failingTestsOf returns the test identities of a failed verify's log as import path and test path, in first-seen order.
// Package and opaque identities name no test and are left out.
func failingTestsOf(log string) []verifytree.FailedTest {
	var tests []verifytree.FailedTest
	for _, failure := range websterengine.ParseVerifyFailures(log) {
		if failure.Kind != websterengine.FailureKindTest {
			continue
		}
		tests = append(tests, verifytree.FailedTest{
			Package: failure.Package,
			Test:    strings.TrimPrefix(failure.ID, failure.Package+"."),
		})
	}
	return tests
}

// driverWaitMark returns the callback landingshed marks the verify wait through, built over two seams so a test needs no tmux:
// status reads reed's strand table and setMark sets or clears a strand's pane wait mark.
// The table is read at call time,
// and the mark goes on the driver strand only while that strand is live and not retiring;
// a strand that is gone, dead or replaced makes the call a logged no-op returning nil.
// A failure of either seam is returned for the gate to log.
func driverWaitMark(
	status func() (reedengine.StatusResult, error),
	setMark func(guid, label string, start time.Time) error,
) func(label string, start time.Time) error {
	return func(label string, start time.Time) error {
		result, err := status()
		if err != nil {
			return err
		}
		strand, found := findDriverStrand(result.Strands)
		if !found || !strand.Live || strand.Retiring {
			logger.Info("loomcli: no live driver strand to carry the verify wait mark", "label", label)
			return nil
		}
		return setMark(strand.GUID, label, start)
	}
}

// conflictSessionStopper returns the seam landingshed stops the run's conflict sessions through, built over two seams so a test needs no tmux:
// status reads reed's strand table and stop ends one strand by guid.
// The table is read at call time, and every live, non-retiring strand whose role is the conflict role or a numbered form of it is stopped.
// The first stop that fails ends the pass and is returned with that strand's guid;
// a failed table read is returned with an empty guid, and a pass that stops every match, or finds none, returns an empty guid and nil.
func conflictSessionStopper(
	status func() (reedengine.StatusResult, error),
	stop func(guid string) error,
) func() (string, error) {
	return func() (string, error) {
		result, err := status()
		if err != nil {
			return "", err
		}
		for _, strand := range result.Strands {
			if !strand.Live || strand.Retiring {
				continue
			}
			name, err := agentname.Parse(strand.Name)
			if err != nil || !agentname.MatchesRole(name.Role, mergeresolve.ConflictRole) {
				continue
			}
			if err := stop(strand.GUID); err != nil {
				return strand.GUID, err
			}
		}
		return "", nil
	}
}

// perWorktreeConfigRels returns the anchor-relative config file of every module that is not hub-wide.
func perWorktreeConfigRels() []string {
	var rels []string
	for _, m := range configreg.Modules() {
		if !m.HubWide {
			rels = append(rels, configengine.ConfigFileRel(m.Name))
		}
	}
	return rels
}
