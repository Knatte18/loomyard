// runread.go reads the tracked records and the machine-local inputs of a run.
// The run FSM derives a run's state and its act rule from them.

package pairteardown

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/runfsm"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// runReader holds one function per substrate a run's FSM inputs are read from, so a test drives every branch with fakes.
type runReader struct {
	// board reads the status of the slug's board entry.
	board func(slug string) (status *string, found bool, err error)
	// batten reads batten's run record of the slug.
	batten func(slug string) (shedengine.Status, bool, error)
	// taskBranchOnOrigin reports whether the slug's task branch has a remote-tracking ref.
	taskBranchOnOrigin func(slug string) (bool, error)
	// pairHere reports whether the slug's task worktree stands on this machine.
	pairHere func(slug string) (bool, error)
	// childHere reads the child run record from the task worktree.
	childHere func(slug string) (shedengine.Status, bool, error)
	// childOnOrigin reads the child run record from origin's records branch.
	childOnOrigin func(slug string) (shedengine.Status, bool, error)
	// owners reads the owners and the machine registry the local Bolt records.
	owners func() (boardengine.Owners, error)
	// identity reads this machine's identity, which the registry compares a recorded owner against.
	identity func() (machinename.Identity, error)
	// parentOf names the slug of the pair slug was forked from, "" for the prime.
	parentOf func(slug string) (string, error)
	// pauseCovers reports whether this machine's pause record covers the slug.
	pauseCovers func(slug string) (bool, error)
	// owedPush lists the failed-push records that hold a push of the slug's run owed, newest first.
	owedPush func(slug string) ([]fabriclayout.FailedPush, error)
	// codeTip reads the code tip the slug's last transition on origin names, and whether origin lacks it.
	codeTip func(slug string) (fabriclayout.CodeTipReading, error)
	// localHoldsTip reports whether the local code branch of the slug's pair holds the code tip.
	// Only a read of a pair standing here calls it.
	localHoldsTip func(slug, tip string) (bool, error)
	// skipPush is true when the environment skips every push, so origin never holds the parent merge.
	skipPush bool
	// originParentHolds reports whether origin's fetched parent branch holds the task's work.
	originParentHolds func(slug string) (bool, error)
	// localParentHolds reports whether the local parent branch holds the task's work.
	localParentHolds func(slug string) (bool, error)
	// restepped reports whether the task worktree's settle records re-step the stop that status names.
	restepped func(slug string, status shedengine.Status) (bool, error)
}

// ReadRun reads the tracked records of the run of slug, over the hub of prime.
// It reads the machine-local inputs of the act rule too.
// The inputs carry no State, which runfsm.Judge fills from the records.
// Every read error is returned naming the read and the slug.
func ReadRun(prime *lymcwd.Location, slug string) (runfsm.Records, runfsm.Inputs, error) {
	return hubRunReader(prime).read(slug)
}

// ReadRunPulled brings the hub's Bolt level with origin, then reads the run of slug as ReadRun does.
// A hold, an owner or a registry entry another machine pushed is therefore read.
// An unreachable origin reads the local Bolt and goes on, and so does a hub with no Bolt checkout.
// Any other failed pull returns an error naming the pull's detail and the way forward, and reads nothing.
func ReadRunPulled(prime *lymcwd.Location, slug string) (runfsm.Records, runfsm.Inputs, error) {
	boltDir := fabriclayout.BoltDir(prime.HubPath)
	if _, err := os.Stat(boltDir); err == nil {
		if err := boardengine.PullBolt(boltDir); err != nil && !errors.Is(err, boardengine.ErrOwnerOriginUnreachable) {
			return runfsm.Records{}, runfsm.Inputs{}, fmt.Errorf(
				"read the run of %s: the board could not be pulled: %w; way forward: bring the board level as it names, then run the verb again",
				slug, err)
		}
	}
	return ReadRun(prime, slug)
}

// hubRunReader wires a runReader over the hub of prime.
func hubRunReader(prime *lymcwd.Location) runReader {
	boardDir := fabriclayout.BoardDir(prime.HubPath)
	taskLocation := func(slug string) (*lymcwd.Location, error) {
		return lymcwd.ResolveWorktree(fabricengine.WorktreePath(prime, slug))
	}
	return runReader{
		board: func(slug string) (*string, bool, error) {
			board, err := boardengine.OpenHub(prime.HubPath)
			if err != nil {
				return nil, false, err
			}
			task, found, err := board.GetTask(slug)
			if err != nil || !found {
				return nil, false, err
			}
			return task.Status, true, nil
		},
		batten: func(slug string) (shedengine.Status, bool, error) { return ReadBattenRecord(prime, slug) },
		taskBranchOnOrigin: func(slug string) (bool, error) {
			return fabricengine.FetchedTaskBranchOnOrigin(prime, slug)
		},
		pairHere: func(slug string) (bool, error) {
			if _, err := os.Stat(fabricengine.WorktreePath(prime, slug)); err != nil {
				if os.IsNotExist(err) {
					return false, nil
				}
				return false, err
			}
			_, err := taskLocation(slug)
			return err == nil, nil
		},
		childHere: func(slug string) (shedengine.Status, bool, error) {
			task, err := taskLocation(slug)
			if err != nil {
				return shedengine.Status{}, false, err
			}
			return readStatus(shedrun.StatusFile(task, shedrun.SelfRunID), shedrun.StatusLock(task, shedrun.SelfRunID))
		},
		childOnOrigin: func(slug string) (shedengine.Status, bool, error) {
			data, found, err := fabricengine.OriginRunStatus(prime, slug)
			if err != nil || !found {
				return shedengine.Status{}, false, err
			}
			status, err := shedengine.ParseStatusTolerant(data)
			if err != nil {
				// The read above accepted the slug as a run id, so building the path does not fail.
				// The path lacks the anchor of origin's records branch, which only the read knows.
				_, statusRel, _ := shedrun.TreeRunFileRels(slug)
				return shedengine.Status{}, false, fmt.Errorf("parse the child run record of %s on origin, %s under the records branch's anchor: %w", slug, statusRel, err)
			}
			return status, true, nil
		},
		owners: func() (boardengine.Owners, error) { return boardengine.ReadOwners(boardDir) },
		identity: func() (machinename.Identity, error) {
			identity, err := machinename.ReadIdentity()
			if err != nil {
				return machinename.Identity{}, fmt.Errorf("read this machine's name: %w", err)
			}
			return identity, nil
		},
		parentOf: func(slug string) (string, error) { return hubgeom.ParentOf(prime, slug) },
		pauseCovers: func(slug string) (bool, error) {
			geometry, err := hubgeom.PauseGeometry(prime)
			if err != nil {
				return false, err
			}
			coverage, err := pauserecord.ReadCoverage(geometry.RecordPath, slug, time.Now())
			return coverage.Covered, err
		},
		owedPush: func(slug string) ([]fabriclayout.FailedPush, error) {
			return fabricengine.FailedPushesOwedBy(prime.HubPath, slug)
		},
		codeTip: func(slug string) (fabriclayout.CodeTipReading, error) { return fabricengine.OriginCodeTip(prime, slug) },
		localHoldsTip: func(slug, tip string) (bool, error) {
			return fabricengine.HeadContains(fabricengine.WorktreePath(prime, slug), tip)
		},
		skipPush: fabricengine.EnvSyncOptions().SkipPush,
		originParentHolds: func(slug string) (bool, error) {
			task, err := taskLocation(slug)
			if err != nil {
				return false, err
			}
			return fabricengine.OriginParentHoldsTaskWork(prime, task, slug)
		},
		localParentHolds: func(slug string) (bool, error) {
			task, err := taskLocation(slug)
			if err != nil {
				return false, err
			}
			return fabricengine.ParentHoldsTaskWork(prime, task, slug)
		},
		restepped: func(slug string, status shedengine.Status) (bool, error) {
			task, err := taskLocation(slug)
			if err != nil {
				return false, err
			}
			records, err := sheddriver.ReadSettleRecords(shedrun.SettleRecords(task, shedrun.SelfRunID))
			if err != nil {
				return false, err
			}
			return sheddriver.RestepRecorded(records, status), nil
		},
	}
}

// read reads the records and inputs of the run of slug; see ReadRun.
func (r runReader) read(slug string) (runfsm.Records, runfsm.Inputs, error) {
	var records runfsm.Records
	var inputs runfsm.Inputs
	fail := func(what string, err error) (runfsm.Records, runfsm.Inputs, error) {
		return runfsm.Records{}, runfsm.Inputs{}, fmt.Errorf("read %s of %s: %w", what, slug, err)
	}

	boardStatus, boardFound, err := r.board(slug)
	if err != nil {
		return fail("the board entry", err)
	}
	if boardFound {
		records.Released = boardengine.IsEmptyStatus(boardStatus)
		records.BoardDone = boardStatus != nil && *boardStatus == boardengine.StatusDone
	}

	records.Batten, records.BattenFound, err = r.batten(slug)
	if err != nil {
		return fail("batten's run record", err)
	}
	battenAtLanding := false
	if records.BattenFound {
		row := records.Batten.CurrentProducer
		battenAtLanding = row == landRow || row == worktreeTeardownRow
		records.BattenStarted = row == runShedRow || battenAtLanding
		if records.Batten.State == shedengine.StateCancelled {
			if records.TaskBranchOnOrigin, err = r.taskBranchOnOrigin(slug); err != nil {
				return fail("the task branch on origin", err)
			}
		}
	}

	pairHere, err := r.pairHere(slug)
	if err != nil {
		return fail("whether the task worktree stands here", err)
	}
	if pairHere {
		records.Child, records.ChildFound, err = r.childHere(slug)
	} else {
		records.Child, records.ChildFound, err = r.childOnOrigin(slug)
	}
	if err != nil {
		return fail("the child run record", err)
	}
	records.ChildLanding = battenAtLanding ||
		(records.ChildFound && (records.Child.CurrentProducer == loomshed.NameFinalize || records.Child.State == shedengine.StateDone))

	if err := r.readOwner(slug, &inputs); err != nil {
		return fail("the owner", err)
	}
	if inputs.PauseCovers, err = r.pauseCovers(slug); err != nil {
		return fail("the pause record", err)
	}

	owed, err := r.owedPush(slug)
	if err != nil {
		return fail("the owed push", err)
	}
	if len(owed) > 0 {
		inputs.PushOwed = true
		inputs.PushOwedError = owed[0].Error
		inputs.PushOwedWorktree = owed[0].RepoPath
		inputs.PushOwedWayForward = fabricengine.FailedPushWayForward(owed[0])
	}
	tip, err := r.codeTip(slug)
	if err != nil {
		return fail("the code tip", err)
	}
	if tip.Missing {
		// A pair here whose code branch holds the tip owes it to origin.
		// Its own pusher pushes the code side after the records side.
		heldHere := false
		if pairHere {
			if heldHere, err = r.localHoldsTip(slug, tip.Tip); err != nil {
				return fail("whether the local code branch holds the code tip", err)
			}
		}
		if !heldHere {
			inputs.CodeTipMissing = true
			inputs.CodeTip = tip.Tip
			inputs.CodeTipMachine = tip.Machine
			inputs.CodeTipPairAbsent = !pairHere
		}
	}

	// A local landing reaches the local parent first, and origin's only once the parent's push is made.
	// Either one holding the work puts the run past its merge, so a resume never drives the merge again.
	parentHolds := func() (bool, error) {
		if !r.skipPush {
			if held, err := r.originParentHolds(slug); err != nil || held {
				return held, err
			}
		}
		return r.localParentHolds(slug)
	}
	switch {
	case battenAtLanding && !pairHere:
		// A pair is removed only once origin holds the work, so there is no parent read left to make.
		inputs.PastParentMerge = true
	case battenAtLanding:
		inputs.PastParentMerge, err = parentHolds()
	case records.ChildFound && pairHere:
		inputs.PastParentMerge, err = battenshed.FinalizePastMerge(
			records.Child,
			func(status shedengine.Status) (bool, error) { return r.restepped(slug, status) },
			parentHolds,
		)
	}
	if err != nil {
		return fail("whether the work reached its parent", err)
	}
	return records, inputs, nil
}

// readOwner fills inputs with the slug's effective owner and the subject adopt takes to move it.
// It also fills whether the registry reads that owner as another machine.
// It fills whether a Bolt hold holds the slug, with the hold's machine.
func (r runReader) readOwner(slug string, inputs *runfsm.Inputs) error {
	here, err := r.identity()
	if err != nil {
		return err
	}
	owners, err := r.owners()
	if err != nil {
		return err
	}
	if inputs.Owner, err = owners.Effective(slug, r.parentOf); err != nil {
		return err
	}
	if inputs.AdoptSubject, err = owners.AdoptSubject(slug, r.parentOf); err != nil {
		return err
	}
	inputs.OwnedElsewhere = boardengine.OwnedElsewhere(inputs.Owner, owners.Registry, here)
	if hold, held := owners.HoldOf(slug); held {
		inputs.Held = true
		inputs.HoldMachine = hold.Machine
	}
	return nil
}
