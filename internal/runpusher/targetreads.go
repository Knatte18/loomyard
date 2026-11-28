// targetreads.go reads whether a target is ahead of origin or refused, and prunes the ledger entries of gone worktrees.

package runpusher

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/pushledger"
)

// targetAhead reports whether t holds something origin lacks.
// A target whose ledger entry holds a landing is ahead whatever its branch reads.
// So is a target that owes a hand-over.
// So is one whose branch still carries a dropped commit past origin.
// Only a parent pass makes the steps those two owe.
// The landing target is also ahead while the prime's branch carries a landing commit past origin.
// Only a commit of a run neither done nor dropped counts, so a lost ledger still starts its pusher.
func targetAhead(t Target) (bool, error) {
	entry, _, err := pushledger.Read(t.Location.HubPath, ledgerKey(t))
	if err != nil || len(entry.Landings) > 0 {
		return len(entry.Landings) > 0, err
	}
	if len(entry.HandOvers) > 0 {
		return true, nil
	}
	if len(entry.Dropped) > 0 {
		commits, err := fabricengine.LandingCommits(t.Location)
		if err != nil {
			return false, err
		}
		for run := range entry.Dropped {
			if commits[run] != "" {
				return true, nil
			}
		}
	}
	switch t.Kind {
	case KindLanding:
		unsettled, err := fabricengine.UnsettledLandings(t.Location)
		if err != nil {
			return false, err
		}
		for run := range unsettled {
			if entry.Dropped[run] == "" {
				return true, nil
			}
		}
		return false, nil
	case KindBoard:
		return boardAhead(t.Location.HubPath)
	default:
		return fabricengine.RunCommitsAhead(t.Location)
	}
}

// boardAhead reports whether the board holds an uncommitted change or a commit origin lacks.
// A board write saves its files and leaves the commit to the push, so both count.
// With pushes off only the uncommitted change counts, and with git off nothing does.
func boardAhead(hubPath string) (bool, error) {
	cfg := boardengine.ApplySkipEnv(boardengine.Config{})
	if cfg.SkipGit {
		return false, nil
	}
	bolt := fabricengine.NewBolt(fabriclayout.BoardDir(hubPath))
	uncommitted, err := bolt.HasUncommitted()
	if err != nil || uncommitted || cfg.SkipPush {
		return uncommitted, err
	}
	return bolt.HasUnpushed()
}

// gitEntryName is the `.git` entry every worktree of a repository holds at its root.
const gitEntryName = ".git"

// worktreeGone reports whether no worktree stands at path, which holds when no `.git` entry is under it.
// So a missing path, a file at the path and a directory without the entry all read as gone.
func worktreeGone(path string) (bool, error) {
	_, err := os.Lstat(filepath.Join(path, gitEntryName))
	switch {
	case err == nil:
		return false, nil
	// ENOTDIR is what a file at the path answers.
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ENOTDIR):
		return true, nil
	}
	return false, fmt.Errorf("runpusher: read the worktree %s: %w", path, err)
}

// TargetRefused reports whether a refused push of t's branch stands, which no retry clears.
// The landing target reads the refusals of the prime's code branch, which count against no other target.
// A pair whose worktree no longer stands reads as not refused, since nothing pushes it again.
func TargetRefused(t Target) (bool, error) {
	if t.Pair {
		gone, err := worktreeGone(t.Location.WorktreePath())
		if err != nil || gone {
			return false, err
		}
	}
	switch t.Kind {
	case KindLanding:
		return fabricengine.PrimeCodeRefusalStands(t.Location)
	case KindBoard:
		return boardRefused(t.Location.HubPath)
	default:
		return fabricengine.RefusedPushStands(t.Location)
	}
}

// boardRefused reports whether a failed-push record of a class no retry clears names the board's branch.
func boardRefused(hubPath string) (bool, error) {
	branch, err := gitrepo.New(fabriclayout.BoardDir(hubPath)).CurrentBranch()
	if err != nil {
		return false, fmt.Errorf("runpusher: read the board's branch: %w", err)
	}
	records, err := fabricengine.ListFailedPushes(hubPath)
	if err != nil {
		return false, err
	}
	for _, record := range records {
		if record.Repo == fabriclayout.RepoBolt && record.Branch == branch &&
			!fabricengine.IsPushOwed(&fabricengine.PushFailedError{Record: record}) {
			return true, nil
		}
	}
	return false, nil
}

// PruneLedger removes each ledger entry whose worktree no longer stands, holding no `.git` entry.
// An entry holding landings is kept, since its pending writes wait for the push.
// The board and landing entries name no worktree of their own and are kept.
// A failed worktree read keeps its entry and is returned with every other failure.
func PruneLedger(hubPath string, now time.Time) error {
	entries, err := pushledger.List(hubPath)
	if err != nil {
		return err
	}
	var failures []error
	for key, entry := range entries {
		if key == pushledger.BoardKey || key == pushledger.LandingKey || entry.Worktree == "" || len(entry.Landings) > 0 {
			continue
		}
		gone, err := worktreeGone(entry.Worktree)
		switch {
		case err != nil:
			failures = append(failures, err)
		case gone:
			failures = append(failures, pushledger.Update(hubPath, key, now, func(current pushledger.Entry, found bool) (pushledger.Entry, bool, error) {
				return current, found && len(current.Landings) > 0, nil
			}))
		}
	}
	return errors.Join(failures...)
}
