// gitrepo.go defines the Repo type and its read/commit primitives: New, the
// shared run helper over gitexec.RunGit, CurrentSHA, StageAndCommit,
// StageAllAndCommit, ChangedFilesSince, and SHAExists.

package gitrepo

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/go-git/go-git/v5"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// ErrNoCommits is returned by CurrentSHA when the repository has no commits
// yet (a freshly-initialized repo with an unborn HEAD), so callers get a
// typed signal instead of an ambiguous empty SHA string.
var ErrNoCommits = errors.New("gitrepo: repository has no commits")

// ErrInvalidSHA is returned by ChangedFilesSince and SetSnapshotSHA (and
// folded into false by SHAExists, per its bool-swallowing posture) when a
// caller-supplied SHA argument is not a plain hex object name — surfaced
// before the string ever reaches git, where an option-shaped value would be
// parsed as a flag (`update-ref <ref> -d` deletes the ref instead of setting
// it).
var ErrInvalidSHA = errors.New("gitrepo: invalid SHA")

// shaPattern matches a plain abbreviated-or-full hex object name: 4 hex
// digits (git's minimum abbreviation) up to 64 (a full SHA-256 name). It
// deliberately excludes symbolic revisions (HEAD, refs, ranges) — gitrepo's
// SHA-taking methods contract on stored SHAs, not general revision syntax.
var shaPattern = regexp.MustCompile(`^[0-9a-fA-F]{4,64}$`)

// validSHA reports whether sha is a plain hex object name that is safe to
// embed in a git command line as an argument.
func validSHA(sha string) bool {
	return shaPattern.MatchString(sha)
}

// Repo is a typed handle on one local git checkout, identified by its
// filesystem path. Repo does not create, clone, or validate the checkout at
// construction time — see New. Methods on a single Repo are not
// goroutine-safe for concurrent writes to the same underlying checkout;
// cross-process push serialization is handled separately by the coalescing
// pusher's single-pusher lock, and in-process callers must serialize their
// own writes.
//
// goGitMu, goGitRepo, and goGitOK back the lazily-opened, cached go-git
// handle described in gogit.go's goGit. See gogit.go's godoc for the full
// locking discipline every go-git-backed method (this batch and every batch
// that migrates a read onto go-git) must follow.
type Repo struct {
	path string

	// goGitMu guards goGitRepo and goGitOK, and is the single lock every
	// go-git-backed call on this Repo coordinates through — see gogit.go. No
	// migrated read calls goGit yet in this batch (that starts in batch 3),
	// so golangci-lint's default (untagged) build sees no production caller
	// for either field; each carries a trailing nolint for that reason,
	// matching gitnativepoc/read.go's identical hasUnpushed precedent —
	// gogit_test.go's integration-tagged coverage is the only caller until
	// then.
	goGitMu sync.RWMutex //nolint:unused // only exercised by the //go:build integration-tagged gogit_test.go
	// goGitRepo is the cached go-git handle, valid only when goGitOK is true.
	goGitRepo *git.Repository //nolint:unused // only exercised by the //go:build integration-tagged gogit_test.go
	// goGitOK records that goGitRepo was populated by a *successful* open.
	// Deliberately not a sync.Once: only a successful open may be cached, so a
	// Repo constructed (via New) before the checkout exists at its path can
	// still succeed on a later call once the checkout has been created —
	// New's documented no-I/O, cannot-fail contract says nothing about the
	// checkout existing yet, and a permanently-cached failure would break
	// that.
	goGitOK bool //nolint:unused // only exercised by the //go:build integration-tagged gogit_test.go
}

// New returns a Repo wrapping the git checkout at path. New performs no
// validation and no I/O — it only stores path — so it cannot fail. The
// checkout is assumed to already exist; creating or cloning a repo is
// fabric's topology concern, not gitrepo's.
func New(path string) *Repo {
	return &Repo{path: path}
}

// run executes a git subcommand against this Repo's checkout, delegating to
// gitexec.RunGit with r.path as the working directory. It is the single
// choke point every method on Repo uses to invoke git, so the
// gitexec-is-the-only-exec-layer decision holds without repetition.
func (r *Repo) run(args ...string) (stdout, stderr string, code int, err error) {
	return gitexec.RunGit(args, r.path)
}

// CurrentSHA returns the SHA of the checkout's current HEAD commit. It
// returns ErrNoCommits (checkable via errors.Is) when the repository has no
// commits yet — an unborn HEAD is not a genuine failure, just an empty
// history — and a plain error, including git's stderr, for any other
// non-zero git exit or spawn failure.
func (r *Repo) CurrentSHA() (string, error) {
	stdout, stderr, code, err := r.run("rev-parse", "HEAD")
	if err != nil {
		// A spawn failure (git not found, etc.) is not something CurrentSHA
		// can interpret further; surface it unchanged.
		return "", err
	}
	if code != 0 {
		// An unborn HEAD reports as a non-zero exit with one of these two
		// stderr shapes depending on git version; treat both as ErrNoCommits
		// rather than a generic failure so callers can errors.Is() against it.
		if strings.Contains(stderr, "ambiguous argument 'HEAD'") || strings.Contains(stderr, "unknown revision") {
			return "", ErrNoCommits
		}
		return "", fmt.Errorf("gitrepo: rev-parse HEAD: %s", stderr)
	}
	return strings.TrimSpace(stdout), nil
}

// StageAndCommit stages exactly the given files (never a wildcard/`add -A`
// stage — see the explicit-file-lists decision) and commits exactly those
// files with msg. Index entries staged outside this call are neither
// inspected nor committed — they stay staged, untouched — so an automated
// commit can never sweep up a half-staged edit someone else (a human sharing
// the worktree) left in the index; in a worktree mid-merge, git refuses the
// pathspec-scoped commit outright, surfacing an error instead of silently
// completing the merge under msg. When the listed files produce no staged
// change — including when files is empty, which stages nothing and spawns no
// git at all — StageAndCommit returns ("", false, nil): a plain signal, not
// an error, since "nothing to commit" is an expected, inspectable outcome
// rather than a failure. On a real commit it returns the new HEAD SHA with
// committed=true. Each files entry is a git pathspec relative to the repo
// root, not a literal filename: git still interprets pathspec magic after
// `--` (in the add, the staged-change check, and the commit alike), so an
// entry starting with ':' is treated as a magic signature (and a file
// literally named that way cannot be staged as-is) — callers pass plain
// relative paths and must not rely on magic.
func (r *Repo) StageAndCommit(msg string, files []string) (sha string, committed bool, err error) {
	// An empty list stages nothing, so there is never anything of the
	// caller's to commit; return the documented no-op signal before any git
	// spawn — an unscoped `git commit` here would otherwise commit whatever
	// happened to be staged already.
	if len(files) == 0 {
		return "", false, nil
	}

	addArgs := append([]string{"add", "--"}, files...)
	_, stderr, code, err := r.run(addArgs...)
	if err != nil {
		return "", false, err
	}
	if code != 0 {
		return "", false, fmt.Errorf("gitrepo: git add: %s", stderr)
	}

	// `diff --cached --quiet -- <files>` reports via exit code alone: 0 means
	// the staged tree matches HEAD within the listed pathspecs (nothing of
	// ours to commit), 1 means it differs (proceed to commit). The pathspec
	// scoping keeps entries staged outside this call from counting as "ours".
	// Any other exit is a genuine git failure.
	diffArgs := append([]string{"diff", "--cached", "--quiet", "--"}, files...)
	_, stderr, code, err = r.run(diffArgs...)
	if err != nil {
		return "", false, err
	}
	switch code {
	case 0:
		return "", false, nil
	case 1:
		// Staged changes exist within the listed files; fall through to commit.
	default:
		return "", false, fmt.Errorf("gitrepo: git diff --cached --quiet: %s", stderr)
	}

	// Commit scoped to the same pathspecs: git commits the listed paths'
	// current contents (identical to what the add above just staged) and
	// leaves any other staged entry staged and uncommitted.
	commitArgs := append([]string{"commit", "-m", msg, "--"}, files...)
	_, stderr, code, err = r.run(commitArgs...)
	if err != nil {
		return "", false, err
	}
	if code != 0 {
		return "", false, fmt.Errorf("gitrepo: git commit: %s", stderr)
	}

	sha, err = r.CurrentSHA()
	if err != nil {
		return "", false, err
	}
	return sha, true, nil
}

// StageAllAndCommit stages every working-tree change via `git add -A` and
// commits whatever lands in the index with msg — the wildcard sibling of
// StageAndCommit, which never wildcard-stages. It exists as board's
// Sync/commitDirty opt-in exception, not a relaxation of the explicit-list
// default: every other consumer (fabric, raddle, codeintel) must keep using
// StageAndCommit's explicit file list. Return semantics mirror
// StageAndCommit exactly: when nothing is staged after the add (a clean
// working tree), it returns ("", false, nil) rather than an error, since
// "nothing to commit" is an expected, inspectable outcome; on a real commit
// it returns the new HEAD SHA with committed=true.
func (r *Repo) StageAllAndCommit(msg string) (sha string, committed bool, err error) {
	_, stderr, code, err := r.run("add", "-A")
	if err != nil {
		return "", false, err
	}
	if code != 0 {
		return "", false, fmt.Errorf("gitrepo: git add -A: %s", stderr)
	}

	// `diff --cached --quiet` (unscoped) reports via exit code alone: 0 means
	// the staged tree matches HEAD (nothing to commit), 1 means it differs
	// (proceed to commit). Unlike StageAndCommit, there is no pathspec to
	// scope to — the wildcard add above staged everything, so the diff check
	// is correspondingly unscoped.
	_, stderr, code, err = r.run("diff", "--cached", "--quiet")
	if err != nil {
		return "", false, err
	}
	switch code {
	case 0:
		return "", false, nil
	case 1:
		// Staged changes exist; fall through to commit.
	default:
		return "", false, fmt.Errorf("gitrepo: git diff --cached --quiet: %s", stderr)
	}

	_, stderr, code, err = r.run("commit", "-m", msg)
	if err != nil {
		return "", false, err
	}
	if code != 0 {
		return "", false, fmt.Errorf("gitrepo: git commit: %s", stderr)
	}

	sha, err = r.CurrentSHA()
	if err != nil {
		return "", false, err
	}
	return sha, true, nil
}

// SHAExists reports whether sha names a commit reachable in this Repo. A
// git failure (spawn error or non-zero exit — a missing/garbage SHA is a
// non-zero exit here) is swallowed into false rather than surfaced as an
// error: callers treat "false" as a staleness signal ("when in doubt,
// rebuild") regardless of whether the SHA was simply absent or the check
// itself failed, so distinguishing the two would buy nothing. A non-hex sha
// is folded into the same false for the same reason, without spawning git.
func (r *Repo) SHAExists(sha string) bool {
	if !validSHA(sha) {
		return false
	}
	_, _, code, err := r.run("rev-parse", "--verify", "--quiet", sha+"^{commit}")
	return err == nil && code == 0
}

// CurrentBranch returns the short name of the branch HEAD currently points
// at (e.g. "main"), used by the integration bisect to capture the branch it
// must restore to BEFORE detaching HEAD for a candidate checkout. It returns
// a wrapped error when HEAD is detached (or on any other git failure) rather
// than an empty string, since a caller that failed to capture a branch name
// has no safe ref to hand RestoreBranch afterwards.
func (r *Repo) CurrentBranch() (string, error) {
	stdout, stderr, code, err := r.run("symbolic-ref", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("gitrepo: symbolic-ref --short HEAD: %s", stderr)
	}
	return strings.TrimSpace(stdout), nil
}

// CheckoutDetached moves HEAD to sha without updating any branch ref,
// leaving the working tree at that commit's contents — the bisect's
// per-candidate checkout step, run in place in the single worktree rather
// than a separate clone. sha is validated with the same validSHA check as
// ChangedFilesSince and SHAExists before it ever reaches git, returning
// ErrInvalidSHA (checkable via errors.Is) on a non-hex value rather than
// letting an option-shaped string be parsed as a checkout flag.
func (r *Repo) CheckoutDetached(sha string) error {
	if !validSHA(sha) {
		return ErrInvalidSHA
	}
	_, stderr, code, err := r.run("checkout", "--detach", sha)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("gitrepo: git checkout --detach %s: %s", sha, stderr)
	}
	return nil
}

// RestoreBranch moves HEAD back onto ref (the branch name CurrentBranch
// captured before the bisect loop's first CheckoutDetached call), ending
// the detached-HEAD state the bisect left the worktree in.
func (r *Repo) RestoreBranch(ref string) error {
	_, stderr, code, err := r.run("checkout", ref)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("gitrepo: git checkout %s: %s", ref, stderr)
	}
	return nil
}

// ChangedFilesSince returns the repo-relative paths that differ between sha
// and HEAD, considering committed history only — uncommitted working-tree
// or staged edits are never inspected, matching the snapshot model's
// SHA-to-SHA determinism. A missing or invalid sha is a genuine failure
// here (unlike SHAExists' bool-swallowing posture): callers are expected to
// check SHAExists first and treat a missing SHA as staleness. A non-hex sha
// returns ErrInvalidSHA (checkable via errors.Is) without spawning git.
func (r *Repo) ChangedFilesSince(sha string) ([]string, error) {
	if !validSHA(sha) {
		return nil, ErrInvalidSHA
	}
	// -z terminates each path with NUL and disables core.quotePath's C-style
	// escaping, so a non-ASCII filename comes back verbatim instead of as a
	// quoted "\303\245"-style string that matches nothing on disk.
	// --no-renames keeps a rename reported as both its old path (deleted) and
	// its new path (added); git's default rename detection would fold the pair
	// into one entry and --name-only would then drop the old path, leaving a
	// per-file-state consumer with stale state for it forever.
	stdout, stderr, code, err := r.run("diff", "--name-only", "-z", "--no-renames", sha+"..HEAD")
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("gitrepo: git diff --name-only %s..HEAD: %s", sha, stderr)
	}

	var files []string
	for _, path := range strings.Split(stdout, "\x00") {
		if path == "" {
			continue
		}
		files = append(files, path)
	}
	return files, nil
}
