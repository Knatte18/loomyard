// gitwrap.go re-points webster's git-query helpers at internal/gitrepo,
// replacing the three helpers builderengine/gitquery.go implements locally
// over internal/gitexec directly: headSHA captures a batch's start-SHA,
// changedFiles computes the drift-detection diff source, and dirty is the
// half-done-work signal. Per the Shared Decision
// git-verification-via-gitrepo, every helper here goes through
// gitrepo.Repo except dirty, which gitrepo exposes no porcelain/status
// method for and so wraps gitexec.RunGit directly — the one carved-out
// exception the decision names.

package websterengine

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// headSHA returns worktree's current HEAD commit SHA via
// gitrepo.Repo.CurrentSHA. An unborn HEAD (no commits yet) surfaces as
// gitrepo.ErrNoCommits, checkable via errors.Is against the wrapped error —
// a superset of builderengine.HeadSHA's plain-error behavior on the same
// case, since gitrepo distinguishes "no commits yet" from every other git
// failure while builder's own HeadSHA folds both into one opaque error.
func headSHA(worktree string) (string, error) {
	sha, err := gitrepo.New(worktree).CurrentSHA()
	if err != nil {
		return "", fmt.Errorf("websterengine: head sha in %s: %w", worktree, err)
	}
	return sha, nil
}

// changedFiles returns every file path that differs between sinceSHA and
// HEAD in worktree, via gitrepo.Repo.ChangedFilesSince, with webster's own
// deterministic normalization applied on top: each path is slash-normalized
// (filepath.ToSlash) and the result is sorted lexically.
// gitrepo.ChangedFilesSince itself runs `git diff --name-only -z
// --no-renames` and deliberately does NOT sort or slash-normalize its
// result — exactly right for gitrepo's own per-file-state callers, which
// need both sides of a rename reported verbatim — but webster's deviation
// compare wants a platform-independent, deterministic list, so this
// normalization stays webster-local rather than moving into gitrepo.
func changedFiles(worktree, sinceSHA string) ([]string, error) {
	files, err := gitrepo.New(worktree).ChangedFilesSince(sinceSHA)
	if err != nil {
		return nil, fmt.Errorf("websterengine: changed files since %s in %s: %w", sinceSHA, worktree, err)
	}

	normalized := make([]string, len(files))
	for i, f := range files {
		normalized[i] = filepath.ToSlash(f)
	}
	sort.Strings(normalized)
	return normalized, nil
}

// dirty reports whether worktree has any uncommitted or untracked changes,
// via a non-empty `git status --porcelain`. gitrepo.Repo exposes no
// porcelain/status method — adding one is out of scope for this batch — so
// dirty is webster's own thin wrapper directly over gitexec.RunGit, the one
// exception the Shared Decision git-verification-via-gitrepo carves out.
func dirty(worktree string) (bool, error) {
	stdout, stderr, exitCode, err := gitexec.RunGit([]string{"status", "--porcelain"}, worktree)
	if err != nil {
		return false, fmt.Errorf("websterengine: git status --porcelain in %s: %w", worktree, err)
	}
	if exitCode != 0 {
		return false, fmt.Errorf("websterengine: git status --porcelain in %s failed: %s", worktree, strings.TrimSpace(stderr))
	}
	return strings.TrimSpace(stdout) != "", nil
}
