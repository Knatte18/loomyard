// parentseams.go fills the parent step's seams that fabric, the board, the ledger and the failed-push records answer.
// It also picks the target a landing's parent push belongs to.

package runpusher

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

// ParentTarget returns the target that pushes a landing on the parent pair at parent:
// the landing target for the prime, and the beam pair's own target otherwise.
func ParentTarget(prime, parent *lymcwd.Location) (Target, error) {
	isPrime, err := fabricengine.IsPrimeWorktree(parent)
	if err != nil {
		return Target{}, fmt.Errorf("runpusher: tell whether %s is the prime: %w", parent.WorktreePath(), err)
	}
	if isPrime {
		return LandingTarget(prime), nil
	}
	return Target{Location: parent, Slug: parent.WorktreeName, Pair: true}, nil
}

// beamPair reports whether t is the pair of a beam, which its members land on.
// A board that cannot be read is logged and reads as none.
func beamPair(t Target) bool {
	if !t.Pair {
		return false
	}
	tasks, err := boardengine.ReadTasks(fabriclayout.BoardDir(t.Location.HubPath))
	if err != nil {
		logger.Warn("runpusher: the board is not read, so the target is served as no beam", "slug", t.Slug, "error", err)
		return false
	}
	for _, task := range tasks {
		if task.Slug == t.Slug {
			return task.Beam
		}
	}
	return false
}

// pairOf returns the location of the run's pair, standing reporting false when its code worktree is gone.
func pairOf(hubPath, run string) (pair *lymcwd.Location, standing bool, err error) {
	pair, err = lymcwd.NamedWorktree(hubPath, run)
	if err != nil {
		return nil, false, err
	}
	_, err = os.Stat(pair.WorktreePath())
	if errors.Is(err, fs.ErrNotExist) {
		return pair, false, nil
	}
	return pair, err == nil, err
}

// parentSeams fills every parent seam of t but PushTip and HandOver, which the caller adds.
// A beam parent also leaves Refresh to the caller. Every read happens at call time.
func parentSeams(t Target) ParentDeps {
	hubPath := t.Location.HubPath
	parentPath := t.Location.WorktreePath()
	key := ledgerKey(t)
	repo := func() *gitrepo.Repo { return gitrepo.New(parentPath) }
	boardDir := fabriclayout.BoardDir(hubPath)

	prime := func() (*lymcwd.Location, error) {
		if t.Kind == KindLanding {
			return t.Location, nil
		}
		name, err := fabricengine.PrimeName(t.Location)
		if err != nil {
			return nil, err
		}
		return lymcwd.NamedWorktree(hubPath, name)
	}
	// ownerOf names the machine that owns the entry slug when it is another than this one, "" otherwise.
	// The empty slug is the root, whose owner is the prime owner.
	ownerOf := func(slug string) (owner, subject string, err error) {
		at, err := prime()
		if err != nil {
			return "", "", err
		}
		parentOf := func(entry string) (string, error) { return hubgeom.ParentOf(at, entry) }
		owners, err := boardengine.ReadOwners(boardDir)
		if err != nil {
			return "", "", err
		}
		here, err := machinename.ReadIdentity()
		if err != nil {
			return "", "", err
		}
		owner, subject = owners.Prime, boardengine.RootSubject
		if slug != "" {
			if owner, err = owners.Effective(slug, parentOf); err != nil {
				return "", "", err
			}
			if subject, err = owners.AdoptSubject(slug, parentOf); err != nil {
				return "", "", err
			}
		}
		if !boardengine.OwnedElsewhere(owner, owners.Registry, here) {
			return "", subject, nil
		}
		return owner, subject, nil
	}
	isPrimeParent := t.Kind == KindLanding
	branch := func() (string, error) { return repo().CurrentBranch() }
	pushStep := fmt.Sprintf("`lym fabric push` in %s", t.Location.AnchorPath())
	var refresh func() error
	if isPrimeParent {
		pushStep = fmt.Sprintf("`git push` in %s", parentPath)
		refresh = func() error { return boardengine.PullBolt(boardDir) }
	}

	return ParentDeps{
		PushStep: pushStep,
		Branch:   branch,
		Refresh:  refresh,
		Fetch:    func() error { return repo().Fetch() },
		OriginHolds: func(tip string) (bool, error) {
			name, err := branch()
			if err != nil {
				return false, err
			}
			upstream, ok, err := repo().Upstream(name)
			if err != nil || !ok {
				return false, err
			}
			return repo().IsAncestor(tip, upstream)
		},
		ParentOwner: func() (string, error) {
			subject := t.Slug
			if isPrimeParent {
				subject = ""
			}
			owner, _, err := ownerOf(subject)
			return owner, err
		},
		RunOwner: ownerOf,
		WorkUnderOtherCommit: func(run string) (bool, error) {
			pair, standing, err := pairOf(hubPath, run)
			if err != nil || !standing {
				return false, err
			}
			at, err := prime()
			if err != nil {
				return false, err
			}
			return fabricengine.OriginParentHoldsTaskWork(at, pair, run)
		},
		LandingCommits: func() (map[string]string, error) { return fabricengine.LandingCommits(t.Location) },
		RunDone: func(run string) (bool, error) {
			unsettled, err := fabricengine.UnsettledLandings(t.Location)
			if err != nil {
				return false, err
			}
			_, open := unsettled[run]
			return !open, nil
		},
		RunPastFinalize: func(run string) (bool, error) {
			pair, standing, err := pairOf(hubPath, run)
			if err != nil || !standing {
				return err == nil, err
			}
			status, found, err := state.ReadJSON[shedengine.Status](shedrun.StatusFile(pair, run), shedrun.StatusLock(pair, run))
			return found && status.State == shedengine.StateDone, err
		},
		Carries: func(tip, commit string) (bool, error) {
			if tip == "" {
				head, err := repo().CurrentSHA()
				if err != nil {
					return false, err
				}
				tip = head
			}
			return repo().IsAncestor(commit, tip)
		},
		AheadByOnly: func(tips []string) (bool, error) { return fabricengine.AheadByOnly(t.Location, tips) },
		UnpushedRecords: func(run string) (string, error) {
			pair, standing, err := pairOf(hubPath, run)
			if err != nil || !standing {
				return "", err
			}
			return fabricengine.OldestUnpushedRecords(pair)
		},
		ResetToOrigin: func() error { return fabricengine.ResetParentToOrigin(t.Location) },
		HoldPair: func(run, records string) error {
			pair, standing, err := pairOf(hubPath, run)
			if err != nil || !standing {
				return err
			}
			if records != "" {
				if err := fabricengine.KeepHeldRecords(pair, run, records); err != nil {
					return err
				}
			}
			pairKey := pushledger.RecordsKey(fabricengine.RecordsWorktree(pair))
			return pushledger.Update(hubPath, pairKey, time.Now(), func(entry pushledger.Entry, _ bool) (pushledger.Entry, bool, error) {
				entry.Worktree, entry.Slug = pair.WorktreePath(), run
				if records != "" {
					entry.HeldRecords = records
				}
				return entry, true, nil
			})
		},
		DoneWrite: func(run string) error {
			board, err := boardengine.OpenHub(hubPath)
			if err != nil {
				return err
			}
			done := boardengine.StatusDone
			return board.SetStatusPushed(run, &done)
		},
		WakePair: func(run string) error {
			pair, standing, err := pairOf(hubPath, run)
			if err != nil || !standing {
				return err
			}
			target := Target{Location: pair, Slug: run, Pair: true}
			err = pushledger.Update(hubPath, ledgerKey(target), time.Now(), func(entry pushledger.Entry, _ bool) (pushledger.Entry, bool, error) {
				entry.Worktree, entry.Slug, entry.PushNow = pair.WorktreePath(), run, true
				return entry, true, nil
			})
			if err != nil {
				return err
			}
			RestartPusher(func() (bool, error) { return Start(target) }, pair.WorktreePath())
			return nil
		},
		RecordRefusal: func(reason, wayForward string) error {
			name, err := branch()
			if err != nil {
				return err
			}
			return fabricengine.RecordPushFailure(hubPath, fabriclayout.FailedPush{
				Repo:       fabriclayout.RepoCode,
				RepoPath:   parentPath,
				Branch:     name,
				Prime:      isPrimeParent,
				Class:      fabriclayout.PushFailureHeldBack,
				Error:      reason,
				WayForward: wayForward,
			}, time.Now())
		},
		ClearRefusal: func() error {
			if isPrimeParent {
				return fabricengine.ClearPrimeCodeRefusal(t.Location)
			}
			name, err := branch()
			if err != nil {
				return err
			}
			return fabricengine.ClearPushFailure(hubPath, fabriclayout.RepoCode, name)
		},
		RefusalReason: func() (string, error) {
			records, err := fabricengine.ListFailedPushes(hubPath)
			if err != nil {
				return "", err
			}
			for _, record := range records {
				if record.Repo != fabriclayout.RepoCode || record.Class != fabriclayout.PushFailureHeldBack {
					continue
				}
				name, err := branch()
				if err != nil {
					return "", fmt.Errorf("runpusher: read the parent branch of %s: %w", key, err)
				}
				if record.Branch == name {
					return record.Error, nil
				}
			}
			return "", nil
		},
	}
}
