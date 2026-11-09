// anchormove.go moves a weft worktree's content from one anchor to another in one commit.
// It is the step that changing an existing worktree's anchor and creating a worktree at an anchor have in common.

package fabricengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// anchorSeams stands in for the failures of a weft move that a test cannot provoke; its zero value is production.
type anchorSeams struct {
	// fail stands in for a failure of the move's commit or of the integration's branch move.
	// It is called with afterCommit false before the commit or the branch move, and with afterCommit true after it.
	// An error it returns is that failure.
	fail func(afterCommit bool) error
	// weftHead reads the commit the weft worktree at weftPath is on; nil reads it from git.
	weftHead func(weftPath string) (string, error)
}

// runFail calls fail when one is set.
func (s anchorSeams) runFail(afterCommit bool) error {
	if s.fail == nil {
		return nil
	}
	return s.fail(afterCommit)
}

// headSHA returns the commit the weft worktree at weftPath is on.
func (s anchorSeams) headSHA(weftPath string) (string, error) {
	if s.weftHead != nil {
		return s.weftHead(weftPath)
	}
	return gitrepo.New(weftPath).CurrentSHA()
}

// moveWeftContent moves the tracked content of each wired name in names from anchor `from` to anchor `to`.
// It works in the weft worktree at weftPath.
// It renames `from/.lym` to `to/.lym` when it exists.
// Then it writes the anchor marker holding `to` through an os.Root at weftPath.
// All of it is committed in one weft commit spanning from/<names>, to/<names> and the root marker.
// The caller holds the weft write lock.
// A name that is not tracked under from is left alone,
// and a path already standing where a name or `.lym` would land is refused before anything moves.
// A failure before the commit returns committed false and leaves the staged and moved paths for the caller to restore;
// a failure after it, from reading the commit or from seams, returns committed true beside the error.
func moveWeftContent(rec *Mutations, weftPath, from, to string, names []string, seams anchorSeams) (bool, error) {
	root, err := os.OpenRoot(weftPath)
	if err != nil {
		return false, fmt.Errorf("fabricengine: open the weft root %s: %w", weftPath, err)
	}
	defer func() { _ = root.Close() }()

	fromSlash, toSlash := filepath.ToSlash(from), filepath.ToSlash(to)
	var moved []string
	for _, name := range names {
		if name == lymdirs.DotLymDirName {
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
	lymFrom, lymTo := anchorJoin(fromSlash, lymdirs.DotLymDirName), anchorJoin(toSlash, lymdirs.DotLymDirName)
	lymPresent := true
	if _, err := root.Lstat(lymFrom); errors.Is(err, fs.ErrNotExist) {
		lymPresent = false
	} else if err != nil {
		return false, fmt.Errorf("fabricengine: inspect %s in %s: %w", lymFrom, weftPath, err)
	}
	if lymPresent {
		if err := requireAbsent(root, lymTo); err != nil {
			return false, err
		}
	}

	if toSlash != "." {
		if err := root.MkdirAll(toSlash, 0o755); err != nil {
			return false, fmt.Errorf("fabricengine: create %s in %s: %w", toSlash, weftPath, err)
		}
	}
	specs := []string{lymcwd.AnchorFileName}
	for _, name := range moved {
		src, dst := anchorJoin(fromSlash, name), anchorJoin(toSlash, name)
		if _, err := gitexec.Run([]string{"mv", "--", src, dst}, weftPath); err != nil {
			return false, fmt.Errorf("fabricengine: move %s to %s in %s: %w", src, dst, weftPath, err)
		}
		rec.Append(KindPathRenamed, filepath.Join(weftPath, filepath.FromSlash(dst)), renamedFrom(weftPath, src))
		specs = append(specs, src, dst)
	}
	if lymPresent {
		if err := root.Rename(lymFrom, lymTo); err != nil {
			return false, fmt.Errorf("fabricengine: move %s to %s in %s: %w", lymFrom, lymTo, weftPath, err)
		}
		rec.Append(KindPathRenamed, filepath.Join(weftPath, filepath.FromSlash(lymTo)), renamedFrom(weftPath, lymFrom))
	}
	if err := root.WriteFile(lymcwd.AnchorFileName, []byte(to+"\n"), 0o644); err != nil {
		return false, fmt.Errorf("fabricengine: write %s in %s: %w", lymcwd.AnchorFileName, weftPath, err)
	}
	rec.Append(KindFileWritten, filepath.Join(weftPath, lymcwd.AnchorFileName), "")

	if err := seams.runFail(false); err != nil {
		return false, err
	}
	commitArgs := append([]string{"commit", "-q", "-m", fmt.Sprintf("fabric anchor: move the weft from %s to %s", from, to), "--"}, specs...)
	if _, err := gitexec.RunWithEnv(commitArgs, weftPath, []string{"GIT_LITERAL_PATHSPECS=1"}); err != nil {
		return false, fmt.Errorf("fabricengine: commit the move of the weft from %s to %s: %w", from, to, err)
	}
	sha, err := seams.headSHA(weftPath)
	if err != nil {
		return true, fmt.Errorf("fabricengine: read the weft's commit after the move of the weft from %s to %s: %w", from, to, err)
	}
	rec.Append(KindCommitCreated, weftPath, sha)
	return true, seams.runFail(true)
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
