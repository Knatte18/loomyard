// anchormove.go moves a weft worktree's content from one anchor to another in one commit.
// It is the one step a change of an existing worktree's anchor and the creation of a worktree at an anchor share.

package fabricengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// anchorFailHook, nil in production, stands in for a failure of the move's commit or of the integration's branch move.
// It is called with afterCommit false just before the commit or the branch move, and with afterCommit true just after; an error it returns is that failure.
// A test that sets it must not be parallel.
var anchorFailHook func(afterCommit bool) error

// weftHeadSHA reads the commit the weft worktree at weftPath is on.
// It is a variable so a test can fail the read that follows the move commit.
var weftHeadSHA = func(weftPath string) (string, error) {
	return gitrepo.New(weftPath).CurrentSHA()
}

// runAnchorFailHook calls anchorFailHook when one is set.
func runAnchorFailHook(afterCommit bool) error {
	if anchorFailHook == nil {
		return nil
	}
	return anchorFailHook(afterCommit)
}

// moveWeftContent moves the tracked content of each wired name in names from under the anchor from to under the anchor to in the weft worktree at weftPath.
// It renames from/.lyx to to/.lyx when it exists, and then writes the marker to last through an os.Root at weftPath.
// All of it is committed in one weft commit spanning from/<names>, to/<names> and the root marker.
// The caller holds the weft write lock.
// A name that is not tracked under from is left alone, and a path already standing where a name or `.lyx` would land is refused before anything moves.
// A failure before the commit returns committed false and leaves the staged and moved paths for the caller to restore;
// a failure after it, from reading the commit or from the hook, returns committed true beside the error.
func moveWeftContent(rec *Mutations, weftPath, from, to string, names []string) (bool, error) {
	root, err := os.OpenRoot(weftPath)
	if err != nil {
		return false, fmt.Errorf("fabricengine: open the weft root %s: %w", weftPath, err)
	}
	defer func() { _ = root.Close() }()

	fromSlash, toSlash := filepath.ToSlash(from), filepath.ToSlash(to)
	var moved []string
	for _, name := range names {
		if name == lyxdirs.DotLyxDirName {
			continue
		}
		tracked, err := PathTracked(weftPath, anchorJoin(fromSlash, name))
		if err != nil {
			return false, err
		}
		if !tracked {
			continue
		}
		if err := requireAbsent(root, anchorJoin(toSlash, name)); err != nil {
			return false, err
		}
		moved = append(moved, name)
	}
	lyxFrom, lyxTo := anchorJoin(fromSlash, lyxdirs.DotLyxDirName), anchorJoin(toSlash, lyxdirs.DotLyxDirName)
	lyxPresent := true
	if _, err := root.Lstat(lyxFrom); errors.Is(err, fs.ErrNotExist) {
		lyxPresent = false
	} else if err != nil {
		return false, fmt.Errorf("fabricengine: inspect %s in %s: %w", lyxFrom, weftPath, err)
	}
	if lyxPresent {
		if err := requireAbsent(root, lyxTo); err != nil {
			return false, err
		}
	}

	if toSlash != "." {
		if err := root.MkdirAll(toSlash, 0o755); err != nil {
			return false, fmt.Errorf("fabricengine: create %s in %s: %w", toSlash, weftPath, err)
		}
	}
	specs := []string{lyxcwd.AnchorFileName}
	for _, name := range moved {
		src, dst := anchorJoin(fromSlash, name), anchorJoin(toSlash, name)
		if _, err := gitexec.Run([]string{"mv", "--", src, dst}, weftPath); err != nil {
			return false, fmt.Errorf("fabricengine: move %s to %s in %s: %w", src, dst, weftPath, err)
		}
		rec.Append(KindPathRenamed, filepath.Join(weftPath, filepath.FromSlash(dst)), renamedFrom(weftPath, src))
		specs = append(specs, src, dst)
	}
	if lyxPresent {
		if err := root.Rename(lyxFrom, lyxTo); err != nil {
			return false, fmt.Errorf("fabricengine: move %s to %s in %s: %w", lyxFrom, lyxTo, weftPath, err)
		}
		rec.Append(KindPathRenamed, filepath.Join(weftPath, filepath.FromSlash(lyxTo)), renamedFrom(weftPath, lyxFrom))
	}
	if err := root.WriteFile(lyxcwd.AnchorFileName, []byte(to+"\n"), 0o644); err != nil {
		return false, fmt.Errorf("fabricengine: write %s in %s: %w", lyxcwd.AnchorFileName, weftPath, err)
	}
	rec.Append(KindFileWritten, filepath.Join(weftPath, lyxcwd.AnchorFileName), "")

	if err := runAnchorFailHook(false); err != nil {
		return false, err
	}
	commitArgs := append([]string{"commit", "-q", "-m", fmt.Sprintf("fabric anchor: move the weft from %s to %s", from, to), "--"}, specs...)
	if _, err := gitexec.RunWithEnv(commitArgs, weftPath, []string{"GIT_LITERAL_PATHSPECS=1"}); err != nil {
		return false, fmt.Errorf("fabricengine: commit the move of the weft from %s to %s: %w", from, to, err)
	}
	sha, err := weftHeadSHA(weftPath)
	if err != nil {
		return true, fmt.Errorf("fabricengine: read the weft's commit after the move of the weft from %s to %s: %w", from, to, err)
	}
	rec.Append(KindCommitCreated, weftPath, sha)
	return true, runAnchorFailHook(true)
}

// requireAbsent returns an error unless nothing stands at rel in the weft root, naming the path.
func requireAbsent(root *os.Root, rel string) error {
	_, err := root.Lstat(rel)
	switch {
	case err == nil:
		return fmt.Errorf("fabricengine: %s already exists in the weft, so the content cannot move there; move it out of the way and run the verb again", rel)
	case errors.Is(err, fs.ErrNotExist):
		return nil
	default:
		return fmt.Errorf("fabricengine: inspect %s in the weft: %w", rel, err)
	}
}
