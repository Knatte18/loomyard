// merge.go implements the single-repo merge primitives fabricengine's two-sided coordination
// composes: MergeStart (normal and squash) with four-way outcome classification, MergeConclude,
// ConflictedFiles, MergeHeadPresent, HeadDetached, the fast-forward-only MergeFFOnly advance, and
// the general ref->SHA resolver ResolveSHA.

package gitrepo

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

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

// MergeStart runs `git merge --ff --no-commit <ref>` (squash false) or `git merge --squash <ref>`
// (squash true) and classifies the result.
// The `--ff` spelling is mandatory rather than redundant: it is git's default only until an operator
// sets `merge.ff = only` or `merge.ff = false` in their config, at which point every non-fast-forward
// merge aborts with `Not possible to fast-forward` (or every fast-forward fabricates a merge commit)
// and the four-way classification below is reading a repo state fabric never asked for.
// Pinning it on the command line is the same posture MergeConclude takes with --no-edit: the
// behaviour fabric depends on is stated, never inherited from whatever config the caller happens to
// be running under. The squash form needs no such pin — `merge.ff` does not apply to `--squash`.
// A conflicted merge exits non-zero, so runChecked returns *gitexec.GitError;
// MergeStart classifies on repo state, never on exit code alone, and adds no raw gitexec site.
// It captures HEAD before the call, then on any error uses errors.As to recover the GitError and
// probes ConflictedFiles: a non-empty result means MergeConflicted;
// otherwise the error is genuine and returned.
// On success it probes staged state via `git diff --cached --quiet`, live merge state via
// MergeHeadPresent, and HEAD movement.
// A live MERGE_HEAD means MergeStaged whatever the index diff says, and that arm is not redundant
// with the staged one: a real, non-fast-forward merge whose result tree happens to equal HEAD's own
// tree — the shape produced whenever the same change reached both branches independently, by
// cherry-pick, backport, or a duplicated hand-edit — stages nothing and moves no HEAD, yet git has
// genuinely started a merge and `git commit` would land a proper two-parent commit for it.
// Classifying that as MergeAlreadyUpToDate made fabric report a clean no-op, delete its own
// merge-state record, and abandon a live MERGE_HEAD in the checkout — a record-versus-git
// disagreement no fabric verb could then clear.
// The squash form writes no MERGE_HEAD, so the probe is vacuous there and a squash with an empty
// result keeps classifying as MergeAlreadyUpToDate, which is the honest answer: nothing to commit.
// Otherwise HEAD moved with nothing staged means MergeFastForwarded;
// nothing staged, no MERGE_HEAD and HEAD unmoved means MergeAlreadyUpToDate;
// anything else means MergeStaged.
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
// `git diff --name-only --diff-filter=U -z`.
// The `-z` spelling is mandatory rather than cosmetic: without it git C-quotes any path whose
// bytes fall outside core.quotepath's default ASCII set (a conflicted `ä.txt` comes back as the
// literal `"\303\244.txt"`, quotes included), which is not a real worktree path — fabricengine's
// visible-tree mapping then misclassifies a perfectly mappable conflict as unmergeable, and no
// caller-supplied real path can ever match the quoted form. `-z` emits raw path bytes,
// NUL-separated, unconditionally.
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

// StageResolved stages paths, repo-root-relative, that a caller has resolved on disk mid-merge,
// including removals.
// An empty or nil paths is a no-op, returning nil without invoking git at all.
// It runs `git add -A -- <paths>`, the -A form rather than the plain form StageAndCommit uses,
// because a delete/modify conflict is legitimately resolved by the file being gone and the removal
// must stage rather than error.
// The `-A` is a version pin, not a behavioural difference on any git in use today, and the
// distinction matters because the two readings suggest different things to a maintainer. Plain
// `git add <pathspec>` acquired removal-staging in git 2.0 (2014); before that it ignored deletions
// and the two forms genuinely diverged on exactly this case. On a modern git they are equivalent
// here — verified directly: a modify/delete conflict resolved by deleting the file stages with plain
// `git add -- <path>`, exit 0, unmerged set empty afterwards, on git 2.53. So no test can separate
// the two forms, and the earlier claim that the plain form "errors on a missing pathspec" was simply
// false against the git this repo runs on.
// It stays `-A` for the same reason MergeStart pins `--ff` and MergeConclude pins `--no-edit`: the
// behaviour fabric depends on is stated on the command line, never inherited from whatever git
// version or config the caller happens to be running under.
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

// MergeHeads enumerates EVERY commit the live merge is merging in — the full contents of MERGE_HEAD,
// one SHA per entry, in git's own recorded order — returning an empty, never nil, slice when no merge
// is live.
// It answers WHICH merge is in progress, where MergeHeadPresent answers only THAT one is. A caller
// concluding a recorded merge needs the former: `git commit` commits whatever MERGE_HEAD names, so
// without comparing that against the merge the caller thinks it is finishing, an unrelated merge an
// operator started with plain git is committed and claimed as the caller's own.
//
// It is deliberately not built on `git rev-parse --verify --quiet MERGE_HEAD`, and that is the whole
// reason this method reads the file rather than shelling a second query. MERGE_HEAD is multi-valued
// for an octopus merge, and every rev-parsing spelling collapses it to the FIRST entry: on git 2.53,
// `rev-parse --verify --quiet MERGE_HEAD` and `rev-list --no-walk MERGE_HEAD` both print one SHA for a
// two-head MERGE_HEAD, while `for-each-ref MERGE_HEAD` and `show-ref MERGE_HEAD` print nothing at all
// (it is not under refs/). A first-entry-only answer would let `git merge --no-commit <expected> <decoy>`
// pass an equality test against the expected SHA, which is exactly the octopus a caller must reject.
// The file itself is the only complete source, and `git rev-parse --git-path MERGE_HEAD` is git's own
// supported way to locate it — it resolves correctly for a linked worktree, where the file lives under
// `.git/worktrees/<name>/` rather than beside the repo's own `.git`.
// The path is printed relative to the git invocation's directory when the repo is the main worktree, so
// a relative answer is joined onto this Repo's path.
func (r *Repo) MergeHeads() ([]string, error) {
	stdout, err := r.runChecked("rev-parse", "--git-path", "MERGE_HEAD")
	if err != nil {
		return nil, fmt.Errorf("gitrepo: rev-parse --git-path MERGE_HEAD in %s: %w", r.path, err)
	}

	mergeHeadPath := strings.TrimSpace(stdout)
	if !filepath.IsAbs(mergeHeadPath) {
		mergeHeadPath = filepath.Join(r.path, mergeHeadPath)
	}

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
// fabric's merge verbs need this as a precondition: a merge concluded on a detached HEAD lands a
// commit no ref reaches, so the next checkout discards it silently while the paired repo's half of
// the same merge stays landed.
// CurrentBranch cannot answer it — that method collapses detachment into an error indistinguishable
// from a genuine read failure — so this is a separate, boolean-returning probe.
// Like ResolveSHA it is a go-git read of on-disk state, so it stays off the gitrepo Client Boundary
// Invariant's pinned CLI list.
func (r *Repo) HeadDetached() (bool, error) {
	repo, err := r.goGit()
	if err != nil {
		return false, err
	}

	r.goGitMu.RLock()
	defer r.goGitMu.RUnlock()

	head, err := repo.Reference(plumbing.HEAD, false)
	if err != nil {
		return false, fmt.Errorf("gitrepo: read HEAD reference in %s: %w", r.path, err)
	}
	return head.Type() != plumbing.SymbolicReference, nil
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

// CommitParents returns sha's parent SHAs in git's own recorded order — first parent first, so
// parents[0] is the branch the merge was made ON and parents[1:] are the merged-in tips.
// A root commit returns an empty, never nil, slice.
// It exists so a caller can tell a merge commit apart from an ordinary one, and tell WHICH merge a
// given merge commit is, by exact parentage rather than by inference from HEAD movement:
// fabricengine's conclude-adoption arm needs positive evidence that the commit sitting on a
// checkout is this merge's own conclude and not some unrelated commit an operator landed while the
// merge record was live.
// Returns ErrInvalidSHA when sha is not a valid hex object name, mirroring FileAtRevision's own
// argument pre-check.
// This is a go-git read of on-disk state, so it stays off the gitrepo Client Boundary Invariant's
// pinned CLI list.
func (r *Repo) CommitParents(sha string) ([]string, error) {
	if !validSHA(sha) {
		return nil, ErrInvalidSHA
	}

	repo, err := r.goGit()
	if err != nil {
		return nil, err
	}

	commit, err := lookupObjectRetrying(r, repo, func() (*object.Commit, error) {
		return repo.CommitObject(plumbing.NewHash(sha))
	})
	if err != nil {
		return nil, fmt.Errorf("gitrepo: read commit %s in %s: %w", sha, r.path, err)
	}

	r.goGitMu.RLock()
	defer r.goGitMu.RUnlock()

	parents := make([]string, 0, len(commit.ParentHashes))
	for _, parent := range commit.ParentHashes {
		parents = append(parents, parent.String())
	}
	return parents, nil
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
