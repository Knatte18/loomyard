// landingdeps.go declares landingDeps, the assembly seam that builds a landingshed.Deps struct from
// every value already resolved by run.go. landingDeps performs no I/O of any kind -- it exists so
// the drift-guard test in landingdeps_test.go stays Tier 1 with no hubforge fixture, mirroring
// wiring.go's own header-comment convention for why wire is extracted.

package loomcli

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
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
	geom websterengine.Geometry,
	taskBranch, originURL, parentBranch string,
	pushSkipped bool,
	pushBranch func() error,
	registry modelspec.Registry,
	runner *shuttleengine.Runner,
	cfg landingshed.Config,
	parentName string,
	verifyWaitMark func(label string, start time.Time) error,
	stopConflictSession func() (string, error),
	verify verifySource,
) landingshed.Deps {
	return landingshed.Deps{
		ParentName:      parentName,
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
		// Notify queues one orch notice for the hub's prime, resolved at call time.
		// A prime with no orch strand recorded is not an error: QueueNotice logs the notice instead.
		Notify: func(line string) error {
			return orchcli.NotifyPrime(l, line)
		},
		VerifyCommand:          verify.command,
		VerifyFailedWayForward: verify.failedWayForward,
		FailingTests:           failingTestsOf,
		VerifyDir:              verifytree.Dir(l.AnchorPath()),
		GateSlots:              hubgeom.GateSlots(l),
		VerifyWaitMark:         verifyWaitMark,
		Shuttle:                runner,
		Registry:               registry,
		Config:                 cfg,
	}
}

// verifySource pairs the verify-command reader and the failed-verify way-forward clause one recipe tells its verify gate and landing producers.
type verifySource struct {
	// command returns the verify command line, read each time it is called.
	command func() (string, error)
	// failedWayForward is the clause a recorded failed-verify Stuck reason ends with.
	failedWayForward string
}

// planVerifySource returns loom's verify source: the plan's `## verify:` command, and the Webster-Burler route back to a fix.
// command reads the plan each time it is called, never at construction:
// landingDeps runs at bootstrap, before the plan exists on a fresh run,
// so a value captured there would be empty and the gate would silently skip.
// A read or parse error is returned, not mapped to an empty command;
// only a parsed plan with no "## verify:" section yields "".
func planVerifySource(l *lyxcwd.Location) verifySource {
	return verifySource{
		command: func() (string, error) {
			plan, err := planparser.ParsePlan(planparser.PlanDir(l.AnchorPath()))
			if err != nil {
				return "", fmt.Errorf("loom: read plan verify command: %w", err)
			}
			return plan.Verify, nil
		},
		failedWayForward: `run "lyx loom goto --to Webster-Burler", then "lyx loom resume", in the task worktree; the Webster-Review round reads the Publish failure record and its gate runs the failing tests`,
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
		_, _, err = fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), parentLocation, []string{runsRoot}, fmt.Sprintf("loom: run records checkpoint for landing %s", slug), fabricengine.EnvSyncOptions())
		return err
	}
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
