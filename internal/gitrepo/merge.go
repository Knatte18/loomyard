// merge.go implements the single-repo merge primitives fabricengine's two-sided coordination
// composes: MergeStart (normal and squash) with four-way outcome classification, MergeConclude,
// ConflictedFiles, MergeHeadPresent, the fast-forward-only MergeFFOnly advance, and the general
// ref->SHA resolver ResolveSHA.

package gitrepo

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// MergeOutcome classifies what state MergeStart left the repo in.
type MergeOutcome int

// The four outcomes MergeStart can classify a repo into.
const (
	MergeStaged          MergeOutcome = iota // merged into index/worktree, uncommitted
	MergeConflicted                          // unmerged index entries present
	MergeFastForwarded                       // HEAD moved; nothing staged, no MERGE_HEAD
	MergeAlreadyUpToDate                     // nothing to do
)

// MergeStart runs `git merge --no-commit <ref>` (squash false) or `git merge --squash <ref>`
// (squash true) and classifies the result.
// A conflicted merge exits non-zero, so runChecked returns *gitexec.GitError;
// MergeStart classifies on repo state, never on exit code alone, and adds no raw gitexec site.
// It captures HEAD before the call, then on any error uses errors.As to recover the GitError and
// probes ConflictedFiles: a non-empty result means MergeConflicted;
// otherwise the error is genuine and returned.
// On success it probes staged state via `git diff --cached --quiet` and HEAD movement: HEAD moved
// with nothing staged means MergeFastForwarded;
// nothing staged and HEAD unmoved means MergeAlreadyUpToDate;
// otherwise MergeStaged.
// A ref with a leading '-' is rejected as ErrInvalidSHA before any git spawn, mirroring
// IsAncestor's argument pre-check.
func (r *Repo) MergeStart(ref string, squash bool) (MergeOutcome, error) {
	if strings.HasPrefix(ref, "-") {
		return MergeStaged, ErrInvalidSHA
	}

	headBefore, err := r.CurrentSHA()
	if err != nil {
		return MergeStaged, err
	}

	var mergeErr error
	if squash {
		_, mergeErr = r.runChecked("merge", "--squash", ref)
	} else {
		_, mergeErr = r.runChecked("merge", "--no-commit", ref)
	}

	if mergeErr != nil {
		var gitErr *gitexec.GitError
		if !errors.As(mergeErr, &gitErr) {
			return MergeStaged, fmt.Errorf("gitrepo: merge %s in %s: %w", ref, r.path, mergeErr)
		}

		conflicted, probeErr := r.ConflictedFiles()
		if probeErr != nil {
			return MergeStaged, fmt.Errorf("gitrepo: merge %s in %s: %w", ref, r.path, mergeErr)
		}
		if len(conflicted) > 0 {
			return MergeConflicted, nil
		}
		return MergeStaged, fmt.Errorf("gitrepo: merge %s in %s: %w", ref, r.path, mergeErr)
	}

	_, stagedErr := r.runChecked("diff", "--cached", "--quiet")
	var gitErr *gitexec.GitError
	var staged bool
	switch {
	case stagedErr == nil:
		staged = false
	case errors.As(stagedErr, &gitErr) && gitErr.ExitCode == 1:
		staged = true
	default:
		return MergeStaged, fmt.Errorf("gitrepo: diff --cached --quiet in %s: %w", r.path, stagedErr)
	}

	headAfter, err := r.CurrentSHA()
	if err != nil {
		return MergeStaged, err
	}

	switch {
	case staged:
		return MergeStaged, nil
	case headAfter != headBefore:
		return MergeFastForwarded, nil
	default:
		return MergeAlreadyUpToDate, nil
	}
}

// MergeConclude commits a staged merge or staged squash.
// With a non-empty msg it runs `git commit -m <msg>`;
// with an empty msg it runs `git commit --no-edit`, which takes git's prepared
// MERGE_MSG/SQUASH_MSG without opening an editor — the --no-edit spelling is mandatory, since a bare
// `git commit` with no -m would launch the configured editor and hang a non-interactive caller
// forever.
func (r *Repo) MergeConclude(msg string) error {
	var err error
	if msg != "" {
		_, err = r.runChecked("commit", "-m", msg)
	} else {
		_, err = r.runChecked("commit", "--no-edit")
	}
	if err != nil {
		return fmt.Errorf("gitrepo: merge conclude commit in %s: %w", r.path, err)
	}
	return nil
}

// ConflictedFiles enumerates unmerged paths, repo-root-relative, via
// `git diff --name-only --diff-filter=U`.
// It returns an empty, never nil, slice when there are none.
func (r *Repo) ConflictedFiles() ([]string, error) {
	stdout, err := r.runChecked("diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil, fmt.Errorf("gitrepo: diff --name-only --diff-filter=U in %s: %w", r.path, err)
	}

	files := []string{}
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// MergeHeadPresent reports whether MERGE_HEAD exists, via
// `git rev-parse --verify --quiet MERGE_HEAD`.
func (r *Repo) MergeHeadPresent() (bool, error) {
	_, err := r.runChecked("rev-parse", "--verify", "--quiet", "MERGE_HEAD")
	var gitErr *gitexec.GitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &gitErr) && gitErr.ExitCode == 1:
		return false, nil
	default:
		return false, fmt.Errorf("gitrepo: rev-parse --verify --quiet MERGE_HEAD in %s: %w", r.path, err)
	}
}

// MergeFFOnly advances the repo to ref via `git merge --ff-only <ref>`, failing loudly (never
// silently discarding local commits the way `reset --hard` would) when the advance is not a
// fast-forward.
// A ref with a leading '-' is rejected as ErrInvalidSHA before any git spawn, mirroring
// IsAncestor's argument pre-check.
func (r *Repo) MergeFFOnly(ref string) error {
	if strings.HasPrefix(ref, "-") {
		return ErrInvalidSHA
	}
	if _, err := r.runChecked("merge", "--ff-only", ref); err != nil {
		return fmt.Errorf("gitrepo: merge --ff-only %s in %s: %w", ref, r.path, err)
	}
	return nil
}

// ResolveSHA resolves an arbitrary ref — a branch, a remote-tracking ref such as
// "origin/<branch>", or a SHA — to a full SHA via go-git.
// A resolution failure returns a wrapped error the caller can treat as "not found".
// This is a go-git read of on-disk state, so it stays off the gitrepo Client Boundary Invariant's
// pinned CLI list.
func (r *Repo) ResolveSHA(ref string) (string, error) {
	repo, err := r.goGit()
	if err != nil {
		return "", err
	}

	hash, err := lookupObjectRetrying(r, repo, func() (*plumbing.Hash, error) {
		return repo.ResolveRevision(plumbing.Revision(ref))
	})
	if err != nil {
		return "", fmt.Errorf("gitrepo: resolve %s in %s: %w", ref, r.path, err)
	}
	return hash.String(), nil
}
