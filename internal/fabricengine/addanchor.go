// addanchor.go lets Add make a new pair at an anchor other than its fork point's, and lets a re-run of a create settle the anchor of a pair that already exists.
// The new pair's weft is forked at the fork point's anchor and moved to the requested one under a move record, which Add removes once the pair is wired there.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// newPairAnchor returns the anchor a new pair forked at fork is to be moved to for the requested anchor, and "" when no move is asked for.
// An empty request and one equal to the fork point's own anchor ask for none.
// A request that is invalid, untracked in the fork point's warp checkout, or reaches it through a link is refused with the text of validateAnchorTarget.
// It reads the warp checkout and changes nothing.
func newPairAnchor(fork *lymcwd.Location, requested string) (string, error) {
	if requested == "" {
		return "", nil
	}
	names, err := RepoWiredNames(fork)
	if err != nil {
		return "", fmt.Errorf("load the wired names to place the new pair at %s: %w", requested, err)
	}
	if err := validateAnchorTarget(fork, requested, names); err != nil {
		return "", err
	}
	anchor, err := lymcwd.ValidateAnchorRel(requested)
	if err != nil {
		return "", err
	}
	if filepath.Clean(anchor) == filepath.Clean(fork.AnchorRel) {
		return "", nil
	}
	return anchor, nil
}

// moveForkedWeftToAnchor moves the content of the new pair slug's freshly forked weft from the anchor of fork to the anchor to, in one weft commit that also writes the marker.
// It takes the move lock and writes the move record first, and leaves the record standing for the caller to remove once the pair is wired at to;
// the lock is released on return, since only the record outlives this step.
// When the fork point tracked run records, which the fork left off the disk, their deletion is staged first and the move commit carries it.
func moveForkedWeftToAnchor(rec *Mutations, fork *lymcwd.Location, slug, to string, runRecordsTracked bool) error {
	weftPath := RecordsWorktreePath(fork, slug)
	names, err := RepoWiredNames(fork)
	if err != nil {
		return fmt.Errorf("load the wired names to move the weft of %s to %s: %w", slug, to, err)
	}
	moveLock, acquired, err := lock.TryAcquireWriteLock(moveLockPath(weftPath))
	if err != nil {
		return fmt.Errorf("take the move lock of %s: %w", weftPath, err)
	}
	if !acquired {
		return fmt.Errorf("the move lock of the new pair %s is held by another process", slug)
	}
	defer func() { _ = moveLock.Release() }()

	record := MoveRecord{A: fork.AnchorRel, B: to, Kind: MoveKindMove, PID: os.Getpid(), StartTime: ownStartTime()}
	if err := writeMoveRecordFile(weftPath, record); err != nil {
		return err
	}
	lockDir, err := ensureWeftLockDirAt(weftPath)
	if err != nil {
		return err
	}
	writeLock, err := lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))
	if err != nil {
		return fmt.Errorf("acquire the weft write lock: %w", err)
	}
	defer func() { _ = writeLock.Release() }()

	if runRecordsTracked {
		root := filepath.ToSlash(ScopedPathspec(fork.AnchorRel, []string{shedrun.RunsRootRel()})[0])
		if _, err := gitexec.Run([]string{"rm", "-r", "--cached", "-q", "--", root}, weftPath); err != nil {
			return fmt.Errorf("stage the deletion of the run records in %s: %w", weftPath, err)
		}
	}
	if _, found, err := committedMarker(weftPath, "HEAD"); err != nil {
		return err
	} else if !found {
		if err := stageNewMarker(weftPath, to); err != nil {
			return err
		}
	}
	_, err = moveWeftContent(rec, weftPath, fork.AnchorRel, to, names)
	return err
}

// stageNewMarker writes anchor into the marker file of the weft worktree at weftPath and stages it, for a branch that records none.
// The move commit names the marker by pathspec, which git matches only for a path it knows.
func stageNewMarker(weftPath, anchor string) error {
	root, err := os.OpenRoot(weftPath)
	if err != nil {
		return fmt.Errorf("open the weft root %s: %w", weftPath, err)
	}
	defer func() { _ = root.Close() }()
	if err := root.WriteFile(lymcwd.AnchorFileName, []byte(anchor+"\n"), 0o644); err != nil {
		return fmt.Errorf("write %s in %s: %w", lymcwd.AnchorFileName, weftPath, err)
	}
	if _, err := gitexec.Run([]string{"add", "--", lymcwd.AnchorFileName}, weftPath); err != nil {
		return fmt.Errorf("stage %s in %s: %w", lymcwd.AnchorFileName, weftPath, err)
	}
	return nil
}

// EnsureAnchor settles the anchor of the existing pair at pair for a re-run of a create.
// A standing move record is recovered first, as `lym fabric anchor` recovers it: an uncommitted move is undone and made again, and a committed one goes on to the wiring.
// A failed recovery names `lym beam create <slug>` as the re-run, and a held move lock is refused with the text of MoveInProgress.
// An anchor equal to the pair's, or an empty one, then rewires the pair's junctions and scratch link and rewrites its launchers, all idempotently.
// Any other anchor is refused with the two commands that move the pair and then finish the create.
// The record is removed here and no push is spawned, since the create spawns the pair's push next.
func (t *Topology) EnsureAnchor(rec *Mutations, pair *lymcwd.Location, anchor string) error {
	slug := pair.WorktreeName
	weftPath := RecordsWorktreePath(pair, slug)
	record, standing, err := ReadMoveRecord(weftPath)
	if err != nil {
		return err
	}
	if standing {
		change, err := t.BeginAnchorChange(pair, record.B)
		if err != nil {
			return err
		}
		defer change.Close()
		if _, err := change.Apply(rec); err != nil {
			return fmt.Errorf("%w; run `lym beam create %s` again to finish the anchor change", err, slug)
		}
		if err := RemoveMoveRecord(weftPath); err != nil {
			return err
		}
	}

	current, err := lymcwd.NamedWorktree(pair.HubPath, slug)
	if err != nil {
		return err
	}
	if anchor != "" {
		want, err := lymcwd.ValidateAnchorRel(anchor)
		if err != nil {
			return err
		}
		if filepath.Clean(want) != filepath.Clean(current.AnchorRel) {
			return fmt.Errorf("the pair %s sits at the anchor %s, not %s; run `lym fabric anchor %s --set %s` to move it, then `lym beam create %s --anchor %s` again to finish the create",
				slug, current.AnchorRel, want, slug, want, slug, want)
		}
	}
	names, err := RepoWiredNames(current)
	if err != nil {
		return fmt.Errorf("load the wired names to wire %s: %w", slug, err)
	}
	if err := WireJunctionsWith(rec, current, slug, names); err != nil {
		return err
	}
	return writeLaunchers(rec, current, slug)
}
