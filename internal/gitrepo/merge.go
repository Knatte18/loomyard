// merge.go holds the single-repo merge primitives: MergeStart and MergeConclude drive a merge.
// The other methods stage files, probe merge state, and resolve refs.
// They also read commits and compute a merge tree, and MergeFFOnly advances a repo fast-forward only.

package gitrepo

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// MergeOutcome names the state a merge leaves a repo in.
type MergeOutcome int

// The four states a merge can leave a repo in.
const (
	MergeStaged          MergeOutcome = iota // merged into index/worktree, uncommitted
	MergeConflicted                          // unmerged index entries present
	MergeFastForwarded                       // HEAD moved; nothing staged, no MERGE_HEAD
	MergeAlreadyUpToDate                     // nothing to do
)

// MergeStart merges ref into the repo without committing, and classifies the resulting state.
// With squash false it runs `git merge --ff --no-commit <ref>`; with squash true it runs `git merge --squash <ref>`.
// The `--ff` is pinned on the command line, not left to config.
// Git's default changes when an operator sets `merge.ff` to `only` or `false`.
// Then every non-fast-forward merge aborts, or every fast-forward makes a merge commit.
// The classification below would otherwise read a state the caller never asked for.
// The squash form needs no pin, because `merge.ff` does not apply to `--squash`.
// A conflicted merge exits non-zero, so the error is a *gitexec.GitError.
// MergeStart classifies on repo state, never on the exit code alone.
// On a GitError it probes ConflictedFiles: a non-empty result means MergeConflicted,
// and an empty one returns the merge error.
// On success it probes the index with `git diff --cached --quiet` and the live merge via MergeHeadPresent.
// It also checks whether HEAD moved since before the call.
// A live MERGE_HEAD means MergeStaged, whatever the index diff says.
// The arm is not redundant: a real merge whose result tree equals HEAD's stages nothing and moves no HEAD,
// yet `git commit` would still land a two-parent commit for it.
// Such a merge happens when the same change reaches both branches independently.
// Classifying it as MergeAlreadyUpToDate would have the caller drop its merge record while MERGE_HEAD stays live.
// The squash form writes no MERGE_HEAD, so an empty squash result is MergeAlreadyUpToDate: there is nothing to commit.
// Otherwise HEAD moved with nothing staged means MergeFastForwarded;
// nothing staged, no MERGE_HEAD and an unmoved HEAD means MergeAlreadyUpToDate;
// anything else means MergeStaged.
// A ref with a leading '-' is rejected as ErrInvalidSHA before git runs.
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
		_, mergeErr = r.runChecked("merge", "--ff", "--no-commit", ref)
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

	mergeHeadPresent, err := r.MergeHeadPresent()
	if err != nil {
		return MergeStaged, err
	}

	headAfter, err := r.CurrentSHA()
	if err != nil {
		return MergeStaged, err
	}

	switch {
	case staged || mergeHeadPresent:
		return MergeStaged, nil
	case headAfter != headBefore:
		return MergeFastForwarded, nil
	default:
		return MergeAlreadyUpToDate, nil
	}
}

// MergeConclude commits a staged merge or staged squash.
// With a non-empty msg it runs `git commit -m <msg>`.
// With an empty one it runs `git commit --no-edit`, which takes the prepared MERGE_MSG or SQUASH_MSG.
// The `--no-edit` is mandatory: without it, `git commit` opens the configured editor.
// A non-interactive caller would then hang forever.
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

// ConflictedFiles lists the unmerged paths, relative to the repo root, via `git diff --name-only --diff-filter=U -z`.
// The `-z` is mandatory: without it git C-quotes any path with bytes outside core.quotepath's default ASCII set.
// A conflicted `ä.txt` then comes back as the literal `"\303\244.txt"`, quotes included, which is no worktree path.
// Callers could then never match that form against a real path.
// `-z` emits raw path bytes, NUL-separated, unconditionally.
// It returns an empty, never nil, slice when there are none.
func (r *Repo) ConflictedFiles() ([]string, error) {
	stdout, err := r.runChecked("diff", "--name-only", "--diff-filter=U", "-z")
	if err != nil {
		return nil, fmt.Errorf("gitrepo: diff --name-only --diff-filter=U -z in %s: %w", r.path, err)
	}

	files := []string{}
	for _, entry := range strings.Split(stdout, "\x00") {
		if entry != "" {
			files = append(files, entry)
		}
	}
	return files, nil
}

// StageResolved stages the repo-root-relative paths a caller has resolved on disk mid-merge, including removals.
// An empty or nil paths slice is a no-op that never runs git.
// It runs `git add -A -- <paths>`, with `-A` pinned on the command line.
// The behavior then does not depend on the git version or config in use.
func (r *Repo) StageResolved(paths []string) error {
	if len(paths) == 0 {
		return nil
	}

	args := append([]string{"add", "-A", "--"}, paths...)
	if _, err := r.runChecked(args...); err != nil {
		return fmt.Errorf("gitrepo: git add -A in %s: %w", r.path, err)
	}
	return nil
}

// StageTrackedChanges stages every tracked modification and deletion, and nothing untracked, via `git add -u`.
// It lets a caller land edits to tracked files mid-merge.
// `-u` takes no pathspec, so it cannot be aimed at an untracked or ignored path, and it never force-adds.
// A tree with nothing tracked to stage is a successful no-op.
func (r *Repo) StageTrackedChanges() error {
	if _, err := r.runChecked("add", "-u"); err != nil {
		return fmt.Errorf("gitrepo: git add -u in %s: %w", r.path, err)
	}
	return nil
}

// MergeHeads returns every commit the live merge merges in, one SHA per MERGE_HEAD entry, in git's recorded order.
// It returns an empty, never nil, slice when no merge is live.
// It answers which merge is in progress, where MergeHeadPresent answers only whether one is.
// A caller concluding a recorded merge must compare these SHAs, since `git commit` commits whatever MERGE_HEAD names.
// Without that check, an operator's plain-git merge would be committed as the caller's own.
// It reads the file rather than shelling a rev-parse query, because MERGE_HEAD may hold several entries.
// Every rev-parse spelling collapses it to the first entry, and for-each-ref and show-ref print nothing for it.
// A first-entry answer would let an octopus merge pass an equality check against the expected SHA.
// The file itself is the only complete source.
// It lives in the per-worktree git directory, which GitDir names.
func (r *Repo) MergeHeads() ([]string, error) {
	gitDir, err := r.GitDir()
	if err != nil {
		return nil, err
	}
	mergeHeadPath := filepath.Join(gitDir, "MERGE_HEAD")

	content, err := os.ReadFile(mergeHeadPath)
	if errors.Is(err, fs.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("gitrepo: read %s: %w", mergeHeadPath, err)
	}

	heads := []string{}
	for _, line := range strings.Split(string(content), "\n") {
		if entry := strings.TrimSpace(line); entry != "" {
			heads = append(heads, entry)
		}
	}
	return heads, nil
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

// HeadDetached reports whether HEAD points straight at a commit instead of at a branch.
// A merge concluded on a detached HEAD lands a commit no ref reaches, and the next checkout discards it silently.
// Callers need it as a precondition, since the other half of a two-sided merge may already have landed.
// Detachment is reported as a boolean rather than an error, so it is never mistaken for a failed read.
// This is a go-git read of on-disk state, so it stays off the gitrepo Client Boundary Invariant's pinned CLI list.
func (r *Repo) HeadDetached() (bool, error) {
	return readGoGit(r, func(repo *git.Repository) (bool, error) {
		head, err := repo.Reference(plumbing.HEAD, false)
		if err != nil {
			return false, fmt.Errorf("gitrepo: read HEAD reference in %s: %w", r.path, err)
		}
		return head.Type() != plumbing.SymbolicReference, nil
	})
}

// MergeFFOnly advances the repo to ref via `git merge --ff-only <ref>`.
// It fails rather than discarding local commits, as `reset --hard` would, when the advance is not a fast-forward.
// A ref with a leading '-' is rejected as ErrInvalidSHA before git runs.
func (r *Repo) MergeFFOnly(ref string) error {
	if strings.HasPrefix(ref, "-") {
		return ErrInvalidSHA
	}
	if _, err := r.runChecked("merge", "--ff-only", ref); err != nil {
		return fmt.Errorf("gitrepo: merge --ff-only %s in %s: %w", ref, r.path, err)
	}
	return nil
}

// CommitParents returns sha's parent SHAs in git's recorded order, first parent first.
// A merge's parents[0] is the branch it was made on, and parents[1:] are the merged-in tips.
// A root commit returns an empty, never nil, slice.
// Callers use exact parentage, not HEAD movement, to tell which merge a commit concludes.
// While a merge record is live, an operator may land an unrelated commit; parentage tells the two apart.
// Returns ErrInvalidSHA when sha is not a valid hex object name.
// This is a go-git read of on-disk state, so it stays off the gitrepo Client Boundary Invariant's pinned CLI list.
func (r *Repo) CommitParents(sha string) ([]string, error) {
	if !validSHA(sha) {
		return nil, ErrInvalidSHA
	}

	commit, err := readGoGit(r, func(repo *git.Repository) (*object.Commit, error) {
		return repo.CommitObject(plumbing.NewHash(sha))
	})
	if err != nil {
		return nil, fmt.Errorf("gitrepo: read commit %s in %s: %w", sha, r.path, err)
	}

	parents := make([]string, 0, len(commit.ParentHashes))
	for _, parent := range commit.ParentHashes {
		parents = append(parents, parent.String())
	}
	return parents, nil
}

// ResolveSHA resolves a ref to a full SHA via go-git.
// The ref may be a branch, a remote-tracking ref such as `origin/<branch>`, or a SHA.
// A resolution failure returns a wrapped error that callers can treat as not found.
// This is a go-git read of on-disk state, so it stays off the gitrepo Client Boundary Invariant's pinned CLI list.
func (r *Repo) ResolveSHA(ref string) (string, error) {
	hash, err := readGoGit(r, func(repo *git.Repository) (*plumbing.Hash, error) {
		return repo.ResolveRevision(plumbing.Revision(ref))
	})
	if err != nil {
		return "", fmt.Errorf("gitrepo: resolve %s in %s: %w", ref, r.path, err)
	}
	return hash.String(), nil
}

// CommitTree returns the tree SHA that sha's commit records.
// It returns ErrInvalidSHA when sha is not a valid hex object name.
// This is a go-git read of on-disk state, so it stays off the gitrepo Client Boundary Invariant's pinned CLI list.
func (r *Repo) CommitTree(sha string) (string, error) {
	if !validSHA(sha) {
		return "", ErrInvalidSHA
	}

	commit, err := readGoGit(r, func(repo *git.Repository) (*object.Commit, error) {
		return repo.CommitObject(plumbing.NewHash(sha))
	})
	if err != nil {
		return "", fmt.Errorf("gitrepo: read commit %s in %s: %w", sha, r.path, err)
	}
	return commit.TreeHash.String(), nil
}

// MergeTree computes the merge of ours and theirs via `git merge-tree --write-tree`.
// It touches neither the index nor the working tree.
// It returns the resulting tree SHA and clean=true when the merge has no conflicts.
// A conflicted merge returns clean=false with an empty tree.
// The conflicted tree would carry markers that no commit should be compared against.
// ours and theirs are validated before git runs, and an invalid one returns ErrInvalidSHA.
// It runs the CLI, since go-git has no merge machinery.
// It is therefore on the gitrepo Client Boundary Invariant's pinned list.
func (r *Repo) MergeTree(ours, theirs string) (tree string, clean bool, err error) {
	if !validSHA(ours) || !validSHA(theirs) {
		return "", false, ErrInvalidSHA
	}

	stdout, err := r.runChecked("merge-tree", "--write-tree", "--no-messages", ours, theirs)
	var gitErr *gitexec.GitError
	switch {
	case err == nil:
		tree = strings.TrimSpace(strings.SplitN(stdout, "\n", 2)[0])
		if !validSHA(tree) {
			return "", false, fmt.Errorf("gitrepo: merge-tree %s %s in %s: unexpected output %q", ours, theirs, r.path, stdout)
		}
		return tree, true, nil
	case errors.As(err, &gitErr) && gitErr.ExitCode == 1:
		return "", false, nil
	default:
		return "", false, fmt.Errorf("gitrepo: merge-tree %s %s in %s: %w", ours, theirs, r.path, err)
	}
}
