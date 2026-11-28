// addanchor.go places a new pair at a requested anchor.
// A new two-repository pair's weft is forked at the fork point's anchor.
// It is moved to the requested anchor under a move record.
// `Add` removes that record once the pair is wired there.
// A one-repository pair's records move on disk with no record, and ride on the pair's first records commit.

package fabricengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// newPairAnchor returns the anchor that a new pair forked at fork moves to, or "" when no move is asked for.
// An empty request and one equal to the fork point's own anchor ask for none.
// A request that is invalid, untracked in the fork point's warp checkout, or reaches it through a link returns an error.
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

// moveForkedWeftToAnchor moves the content of the new pair's forked weft from the anchor of fork to the anchor to.
// It takes the move lock and writes the move record first.
// The move is one weft commit, which also writes the marker when the branch records none.
// It leaves the record standing for the caller to remove once the pair is wired at to.
// Both locks are released on return, since only the record outlives this step.
// When the fork point tracked run records, which the fork left off the disk, their deletion is staged first.
// The move commit carries that deletion.
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
	_, err = moveWeftContent(rec, weftPath, fork.AnchorRel, to, names, anchorSeams{})
	return err
}

// moveForkedRecordsToAnchor moves the tracked content of each wired name in names.
// It moves it from the anchor of fork to the anchor to.
// It works on disk in the one worktree of a one-repository pair at worktree, and stages the marker holding to.
// Nothing is committed: it returns the paths under the old and the new anchor that the pair's first records commit takes.
// A name not tracked under the old anchor is left alone, and a path already standing where a name would land is refused.
func moveForkedRecordsToAnchor(worktree string, fork *lymcwd.Location, to string, names []string) ([]string, error) {
	root, err := os.OpenRoot(worktree)
	if err != nil {
		return nil, fmt.Errorf("open the worktree root %s: %w", worktree, err)
	}
	defer func() { _ = root.Close() }()

	from, toSlash := filepath.ToSlash(fork.AnchorRel), filepath.ToSlash(to)
	var specs []string
	for _, name := range names {
		if name == lymdirs.DotLymDirName {
			continue
		}
		src, dst := anchorJoin(from, name), anchorJoin(toSlash, name)
		tracked, err := PathTracked(worktree, src)
		if err != nil {
			return nil, err
		}
		if !tracked {
			continue
		}
		if err := requireAbsent(root, dst); err != nil {
			return nil, err
		}
		specs = append(specs, src)
		if _, err := root.Lstat(src); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("inspect %s in %s: %w", src, worktree, err)
		}
		if toSlash != "." {
			if err := root.MkdirAll(toSlash, 0o755); err != nil {
				return nil, fmt.Errorf("create %s in %s: %w", toSlash, worktree, err)
			}
		}
		if err := root.Rename(src, dst); err != nil {
			return nil, fmt.Errorf("move %s to %s in %s: %w", src, dst, worktree, err)
		}
		specs = append(specs, dst)
	}
	if err := stageNewMarker(worktree, to); err != nil {
		return nil, err
	}
	return specs, nil
}

// stageNewMarker writes anchor into the marker file of the weft worktree at weftPath and stages it.
// It is for a branch that records no marker.
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
