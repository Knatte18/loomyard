// anchormarker.go writes, commits and reads from git the `.lym-anchor` markers.
// A worktree's own marker sits at the root of its weft branch.
// It is the one marker writer outside the anchor move, and the verbs that record a marker all write through it.
// It also refuses a marker left empty and derives the anchor of a branch that records none.

package fabricengine

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// refuseEmptyOwnMarker returns nil unless the marker file at the weft worktree root at weftPath is blank.
// It returns an error when the file is present and blank, naming the file and the way forward.
// The way forward is restoring the file from git when the committed marker holds the anchor.
// Otherwise it is writing the anchor (or "." for the repo root) into the file and committing it there;
// the verb is then re-run.
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
// The write goes through an os.Root at weftPath,
// so a marker path replaced by a link cannot redirect it out of the worktree.
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
	rec.Append(fabriclayout.KindFileWritten, filepath.Join(weftPath, lymcwd.AnchorFileName), "")

	sha, committed, err := gitrepo.New(weftPath).StageAndCommit(subject, []string{lymcwd.AnchorFileName})
	if err != nil {
		return "", fmt.Errorf("fabricengine: commit %s in %s: %w", lymcwd.AnchorFileName, weftPath, err)
	}
	if committed {
		rec.Append(fabriclayout.KindCommitCreated, weftPath, sha)
	}
	return sha, nil
}

// committedMarker reads the marker file at the root of rev's tree in the repository at repoDir, through git objects only.
// An absent file reports found false.
// A file that is blank after trimming reports found true with an empty marker,
// so each caller decides what a blank record means.
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
// Without one, the answer is the shallowest directory of rev's tree that holds one of names.
// It is "." when the root holds one, and a tie at one depth goes to the byte-wise smaller path.
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

// recordAdoptedWeftAnchor gives the adopted weft branch checked out at weftPath the marker it lacks.
// It returns the Location of the pair slug at its own anchor.
// A branch whose committed marker is absent or blank gets an anchor derived from the tree at its tip.
// The derivation takes names as the wired names.
// A branch that carries a marker keeps it.
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
