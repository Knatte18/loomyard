// anchormarker.go writes, commits, reads from git and retires the `.lym-anchor` markers: a worktree's own marker at the root of its weft branch, and the legacy hub-wide marker on the board.
// It is the one marker writer outside the anchor move: clone's fresh bind, reconcile's migration and an adopted branch all record their marker through it.
// It also refuses a marker left empty and derives the anchor of a branch that records none.

package fabricengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// boltAnchorRemoved is the verdict retireBoltAnchorMarker reports once the hub-wide marker is gone.
const boltAnchorRemoved = "removed"

// boltAnchorKeptPrefix opens the verdict retireBoltAnchorMarker reports while the hub-wide marker stays, followed by the reason.
const boltAnchorKeptPrefix = "kept: "

// refuseEmptyOwnMarker returns nil when the marker file at the root of the weft worktree at weftPath is absent or holds a value.
// It returns an error when the file is present and blank, naming the file and the way forward.
// The way forward is restoring the file from git when the committed marker holds the anchor.
// Otherwise it is writing the anchor (or "." for the repo root) into the file and committing it there; the verb is then re-run.
// It touches nothing.
func refuseEmptyOwnMarker(weftPath string) error {
	markerPath := filepath.Join(weftPath, lymcwd.AnchorFileName)
	data, err := os.ReadFile(markerPath)
	if err != nil || strings.TrimSpace(string(data)) != "" {
		return nil
	}

	committed, showErr := gitexec.Run([]string{"show", "HEAD:" + lymcwd.AnchorFileName}, weftPath)
	if showErr == nil && strings.TrimSpace(committed) != "" {
		return fmt.Errorf("%s at %s is empty, so its anchor is unreadable; run `git -C %s restore %s` to bring back the committed anchor, then re-run this verb",
			lymcwd.AnchorFileName, markerPath, weftPath, lymcwd.AnchorFileName)
	}
	return fmt.Errorf("%s at %s is empty, so its anchor is unreadable; write the anchor (or \".\" for the repo root) into it and commit it there with `git -C %s`, then re-run this verb",
		lymcwd.AnchorFileName, markerPath, weftPath)
}

// commitAnchorMarker writes anchor plus a newline into the marker file at the root of the weft worktree at weftPath.
// It then commits that one path under subject on the branch checked out there.
// It holds the weft write lock for the write and the commit, pushes nothing, and returns the commit's SHA.
// The write goes through an os.Root at weftPath, so a marker path replaced by a link cannot redirect it out of the worktree.
// When the branch already holds the same bytes it commits nothing and returns an empty SHA.
func commitAnchorMarker(rec *Mutations, weftPath, anchor, subject string) (string, error) {
	lockDir, err := ensureWeftLockDirAt(weftPath)
	if err != nil {
		return "", err
	}
	writeLock, err := lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))
	if err != nil {
		return "", fmt.Errorf("fabricengine: acquire weft write lock: %w", err)
	}
	defer func() { _ = writeLock.Release() }()

	root, err := os.OpenRoot(weftPath)
	if err != nil {
		return "", fmt.Errorf("fabricengine: open weft root %s: %w", weftPath, err)
	}
	defer func() { _ = root.Close() }()

	if err := root.WriteFile(lymcwd.AnchorFileName, []byte(anchor+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("fabricengine: write %s in %s: %w", lymcwd.AnchorFileName, weftPath, err)
	}
	rec.Append(KindFileWritten, filepath.Join(weftPath, lymcwd.AnchorFileName), "")

	sha, committed, err := gitrepo.New(weftPath).StageAndCommit(subject, []string{lymcwd.AnchorFileName})
	if err != nil {
		return "", fmt.Errorf("fabricengine: commit %s in %s: %w", lymcwd.AnchorFileName, weftPath, err)
	}
	if committed {
		rec.Append(KindCommitCreated, weftPath, sha)
	}
	return sha, nil
}

// committedMarker reads the marker file at the root of rev's tree in the repository at repoDir, through git objects only.
// An absent file reports found false.
// A file that is blank after trimming reports found true with an empty marker, so each caller decides what a blank record means.
func committedMarker(repoDir, rev string) (marker string, found bool, err error) {
	data, err := showFile(repoDir, rev, lymcwd.AnchorFileName)
	if err != nil {
		return "", false, fmt.Errorf("read the committed %s at %s in %s: %w", lymcwd.AnchorFileName, rev, repoDir, err)
	}
	if data == nil {
		return "", false, nil
	}
	return strings.TrimSpace(string(data)), true, nil
}

// treeAnchor answers the anchor of the branch at rev in the repository at repoDir.
// A committed marker that is not blank is the answer.
// Without one, the answer is the shallowest directory of rev's tree that holds one of names, "." when the root holds one, a tie at one depth going to the byte-wise smaller path.
// A tree holding none of names answers fallback.
// A committed marker that is no usable anchor is an error naming it.
func treeAnchor(repoDir, rev string, names []string, fallback string) (string, error) {
	marker, found, err := committedMarker(repoDir, rev)
	if err != nil {
		return "", err
	}
	if found && marker != "" {
		anchor, err := lymcwd.ValidateAnchorRel(marker)
		if err != nil {
			return "", fmt.Errorf("the committed %s at %s in %s is unusable: %w", lymcwd.AnchorFileName, rev, repoDir, err)
		}
		return anchor, nil
	}

	listed, err := gitexec.Run([]string{"ls-tree", "-r", "-d", "-z", "--name-only", rev}, repoDir)
	if err != nil {
		return "", fmt.Errorf("list the directories of %s in %s: %w", rev, repoDir, err)
	}
	wired := make(map[string]bool, len(names))
	for _, name := range names {
		wired[name] = true
	}
	holder, holderDepth := "", -1
	for _, dir := range strings.Split(listed, "\x00") {
		if dir == "" || !wired[path.Base(dir)] {
			continue
		}
		parent, depth := path.Dir(dir), 0
		if parent != "." {
			depth = strings.Count(parent, "/") + 1
		}
		if holderDepth < 0 || depth < holderDepth || (depth == holderDepth && parent < holder) {
			holder, holderDepth = parent, depth
		}
	}
	if holderDepth < 0 {
		return fallback, nil
	}
	return filepath.FromSlash(holder), nil
}

// recordAdoptedWeftAnchor gives the adopted weft branch checked out at weftPath the marker it lacks, and returns the Location of the pair slug at its own anchor.
// A branch whose committed marker is absent or blank gets the anchor derived from its tip's tree committed on it, with names as the wired names;
// a branch that carries a marker keeps it.
func recordAdoptedWeftAnchor(rec *Mutations, hubPath, slug, weftPath string, names []string) (*lymcwd.Location, error) {
	marker, _, err := committedMarker(weftPath, "HEAD")
	if err != nil {
		return nil, err
	}
	if marker == "" {
		current, err := lymcwd.NamedWorktree(hubPath, slug)
		if err != nil {
			return nil, err
		}
		anchor, err := treeAnchor(weftPath, "HEAD", names, current.AnchorRel)
		if err != nil {
			return nil, err
		}
		if _, err := commitAnchorMarker(rec, weftPath, anchor, "fabric add: record anchor "+anchor); err != nil {
			return nil, err
		}
	}
	return lymcwd.NamedWorktree(hubPath, slug)
}

// retireBoltAnchorMarker removes the hub-wide marker from the board, and a leftover pre-rename marker beside it, once every worktree's own marker stands in for it.
// The removal is one board commit pushed through Bolt, written only after Bolt is brought up to date, so it never writes over a stale board.
// It reports "" for a hub with no board marker, "removed" once the marker is gone, and "kept: <reason>" while it stays:
// a weft the census finds without a committed marker, or a board write that was skipped.
// A board commit that landed but could not be pushed is reported "removed" with the push failure after it.
func retireBoltAnchorMarker(rec *Mutations, l *lymcwd.Location) (string, error) {
	boardDir := BoardDir(l.HubPath)
	if _, err := os.Stat(filepath.Join(boardDir, lymcwd.AnchorFileName)); errors.Is(err, fs.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("fabricengine: read the board's %s: %w", lymcwd.AnchorFileName, err)
	}

	keeper, err := boltAnchorKeeper(l)
	if err != nil {
		return "", err
	}
	if keeper != "" {
		return boltAnchorKeptPrefix + keeper, nil
	}

	bolt := NewBolt(boardDir)
	res, err := bolt.PullThenCommitWritten([]BoltWrite{{
		Message: "fabric reconcile: retire the hub-wide anchor marker",
		Write:   func() ([]string, error) { return removeBoltAnchorMarkers(boardDir) },
	}}, rec)
	if err != nil {
		return "", err
	}
	if res.Skipped != "" {
		return fmt.Sprintf("%s%s: %s", boltAnchorKeptPrefix, res.Skipped, res.SkipDetail), nil
	}
	if err := bolt.PushRecorded(SyncOptions{}, rec); err != nil {
		return fmt.Sprintf("%s; its board commit is not pushed: %v", boltAnchorRemoved, err), nil
	}
	return boltAnchorRemoved, nil
}

// removeBoltAnchorMarkers deletes both marker files from the board directory through an os.Root, skipping an absent one, and returns the board-relative names it deleted.
func removeBoltAnchorMarkers(boardDir string) ([]string, error) {
	root, err := os.OpenRoot(boardDir)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: open the board %s: %w", boardDir, err)
	}
	defer func() { _ = root.Close() }()

	var removed []string
	for _, name := range []string{lymcwd.AnchorFileName, lymcwd.StaleAnchorFileName} {
		if err := root.Remove(name); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("fabricengine: remove %s from the board: %w", name, err)
		}
		removed = append(removed, name)
	}
	return removed, nil
}

// boltAnchorKeeper runs the census that decides whether the board's marker may go, and answers the first reason it may not, or "" when it may.
// The prime's weft and every local pair's weft must carry a non-blank committed marker at HEAD.
// So must every weft branch, local or fetched, whose warp sibling is on origin, read from its remote-tracking ref.
// A branch whose ref is not among the fetched ones keeps the marker.
// A weft branch with no warp sibling on origin is an orphan and is not counted.
// It reads local refs only and makes no network call.
func boltAnchorKeeper(l *lymcwd.Location) (string, error) {
	entries, err := List(l.WorktreePath())
	if err != nil {
		return "", fmt.Errorf("list worktrees: %w", err)
	}
	for _, entry := range entries {
		if entry.Prunable {
			continue
		}
		weftPath := RecordsWorktreePath(l, filepath.Base(filepath.Clean(filepath.FromSlash(entry.Path))))
		if _, err := os.Stat(weftPath); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return "", err
		}
		marker, _, err := committedMarker(weftPath, "HEAD")
		if err != nil {
			return "", err
		}
		if marker == "" {
			return fmt.Sprintf("the weft at %s carries no committed %s", weftPath, lymcwd.AnchorFileName), nil
		}
	}

	weftRepoRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return "", err
	}
	trackingPrefix := "refs/remotes/" + originRemoteName + "/"
	listed, err := gitexec.Run([]string{"for-each-ref", "--format=%(refname)", "refs/heads/", trackingPrefix}, weftRepoRoot)
	if err != nil {
		return "", fmt.Errorf("list the weft branches of %s: %w", weftRepoRoot, err)
	}
	warp, weft := gitrepo.New(l.WorktreePath()), gitrepo.New(weftRepoRoot)
	counted := make(map[string]bool)
	for _, ref := range strings.Fields(listed) {
		branch := strings.TrimPrefix(strings.TrimPrefix(ref, "refs/heads/"), trackingPrefix)
		warpBranch, isWeft := WeftWarpSlug(branch)
		if !isWeft || counted[branch] {
			continue
		}
		counted[branch] = true

		if _, err := warp.RefSHA(trackingPrefix + warpBranch); errors.Is(err, gitrepo.ErrRefNotFound) {
			continue
		} else if err != nil {
			return "", err
		}
		tracking := trackingPrefix + branch
		if _, err := weft.RefSHA(tracking); errors.Is(err, gitrepo.ErrRefNotFound) {
			return fmt.Sprintf("the weft branch %s is not among the fetched branches of %s", branch, originRemoteName), nil
		} else if err != nil {
			return "", err
		}
		marker, _, err := committedMarker(weftRepoRoot, tracking)
		if err != nil {
			return "", err
		}
		if marker == "" {
			return fmt.Sprintf("the fetched %s/%s carries no committed %s", originRemoteName, branch, lymcwd.AnchorFileName), nil
		}
	}
	return "", nil
}
