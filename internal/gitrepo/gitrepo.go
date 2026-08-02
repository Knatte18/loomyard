// gitrepo.go defines the Repo type and its read/commit primitives: New, the
// shared run helper over gitexec.RunGit, CurrentSHA, StageAndCommit,
// CommitEmpty, StageAllAndCommit, ChangedFilesSince, and SHAExists.

package gitrepo

import (
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/merkletrie"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// ErrNoCommits is returned by CurrentSHA when the repository has no commits.
var ErrNoCommits = errors.New("gitrepo: repository has no commits")

// ErrInvalidSHA is returned when a caller-supplied SHA is not a valid hex
// object name, surfaced before reaching git.
var ErrInvalidSHA = errors.New("gitrepo: invalid SHA")

// ErrIndexNotEmpty is returned by CommitEmpty when the index has staged changes.
var ErrIndexNotEmpty = errors.New("gitrepo: index is not empty")

// shaPattern matches a plain abbreviated-or-full hex object name (4-64 digits).
var shaPattern = regexp.MustCompile(`^[0-9a-fA-F]{4,64}$`)

// validSHA reports whether sha is a valid hex object name.
func validSHA(sha string) bool {
	return shaPattern.MatchString(sha)
}

// Repo is a typed handle on one local git checkout. Methods on a single Repo
// are not goroutine-safe for concurrent writes; callers must serialize.
type Repo struct {
	path string

	goGitMu             sync.RWMutex
	goGitRepo           *git.Repository
	goGitOK             bool
	lastPackFingerprint string
}

// New returns a Repo wrapping the git checkout at path. It performs no I/O.
func New(path string) *Repo {
	return &Repo{path: path}
}

// run executes a git subcommand against this Repo's checkout.
func (r *Repo) run(args ...string) (stdout, stderr string, code int, err error) {
	return gitexec.RunGit(args, r.path)
}

// CurrentSHA returns the SHA of HEAD, or ErrNoCommits if no commits exist.
func (r *Repo) CurrentSHA() (string, error) {
	repo, err := r.goGit()
	if err != nil {
		return "", err
	}

	r.goGitMu.RLock()
	defer r.goGitMu.RUnlock()

	head, err := repo.Head()
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return "", ErrNoCommits
		}
		return "", fmt.Errorf("gitrepo: resolve HEAD: %w", err)
	}
	return head.Hash().String(), nil
}

// StageAndCommit stages the given files and commits them with msg. It returns
// ("", false, nil) when no staged changes exist (not an error). Returns the new
// HEAD SHA with committed=true on a successful commit. Files must be plain
// relative paths; files is never wildcard-staged.
func (r *Repo) StageAndCommit(msg string, files []string) (sha string, committed bool, err error) {
	if len(files) == 0 {
		return "", false, nil
	}

	addArgs := []string{"add", "--"}
	addArgs = append(addArgs, files...)
	_, stderr, code, err := r.run(addArgs...)
	if err != nil {
		return "", false, err
	}
	if code != 0 {
		return "", false, fmt.Errorf("gitrepo: git add: %s", stderr)
	}

	diffArgs := append([]string{"diff", "--cached", "--quiet", "--"}, files...)
	_, stderr, code, err = r.run(diffArgs...)
	if err != nil {
		return "", false, err
	}
	switch code {
	case 0:
		return "", false, nil
	case 1:
	default:
		return "", false, fmt.Errorf("gitrepo: git diff --cached --quiet: %s", stderr)
	}

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

// CommitEmpty records an empty commit (tree identical to its parent's) via
// `git commit --allow-empty`. It refuses if the index is dirty, returning
// ErrIndexNotEmpty. Unlike StageAndCommit, it always commits when it succeeds.
func (r *Repo) CommitEmpty(msg string) (sha string, err error) {
	_, err = r.CurrentSHA()
	switch {
	case err == nil:
		_, stderr, code, runErr := r.run("diff", "--cached", "--quiet")
		if runErr != nil {
			return "", runErr
		}
		switch code {
		case 0:
		case 1:
			return "", ErrIndexNotEmpty
		default:
			return "", fmt.Errorf("gitrepo: git diff --cached --quiet: %s", stderr)
		}
	case errors.Is(err, ErrNoCommits):
		stdout, stderr, code, runErr := r.run("ls-files", "--cached")
		if runErr != nil {
			return "", runErr
		}
		if code != 0 {
			return "", fmt.Errorf("gitrepo: git ls-files --cached: %s", stderr)
		}
		if strings.TrimSpace(stdout) != "" {
			return "", ErrIndexNotEmpty
		}
	default:
		return "", err
	}

	_, stderr, code, err := r.run("commit", "--allow-empty", "-m", msg)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("gitrepo: git commit --allow-empty: %s", stderr)
	}

	sha, err = r.CurrentSHA()
	if err != nil {
		return "", err
	}
	return sha, nil
}

// StageAllAndCommit stages every change via `git add -A` and commits with msg.
// Return semantics mirror StageAndCommit: ("", false, nil) when nothing to
// commit, otherwise the new HEAD SHA with committed=true.
func (r *Repo) StageAllAndCommit(msg string) (sha string, committed bool, err error) {
	_, stderr, code, err := r.run("add", "-A")
	if err != nil {
		return "", false, err
	}
	if code != 0 {
		return "", false, fmt.Errorf("gitrepo: git add -A: %s", stderr)
	}

	_, stderr, code, err = r.run("diff", "--cached", "--quiet")
	if err != nil {
		return "", false, err
	}
	switch code {
	case 0:
		return "", false, nil
	case 1:
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

// SHAExists reports whether sha names a reachable commit. Errors and invalid
// SHAs fold into false, treating it as a staleness signal.
func (r *Repo) SHAExists(sha string) bool {
	if !validSHA(sha) {
		return false
	}

	repo, err := r.goGit()
	if err != nil {
		return false
	}

	_, err = lookupObjectRetrying(r, repo, func() (*object.Commit, error) {
		return commitByHash(repo, sha)
	})
	return err == nil
}

// commitByHash resolves sha to its commit object, peeling like
// `git rev-parse <sha>^{commit}`. Tree or blob hashes report error.
func commitByHash(repo *git.Repository, sha string) (*object.Commit, error) {
	if len(sha) == hex.EncodedLen(len(plumbing.ZeroHash)) {
		return repo.CommitObject(plumbing.NewHash(sha))
	}

	hash, err := repo.ResolveRevision(plumbing.Revision(sha))
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return nil, plumbing.ErrObjectNotFound
		}
		return nil, err
	}
	return repo.CommitObject(*hash)
}

// CurrentBranch returns the short name of the branch HEAD points to, or an
// error if HEAD is detached.
func (r *Repo) CurrentBranch() (string, error) {
	repo, err := r.goGit()
	if err != nil {
		return "", err
	}

	r.goGitMu.RLock()
	defer r.goGitMu.RUnlock()

	head, err := repo.Reference(plumbing.HEAD, false)
	if err != nil {
		return "", fmt.Errorf("gitrepo: read HEAD reference: %w", err)
	}
	if head.Type() != plumbing.SymbolicReference {
		return "", fmt.Errorf("gitrepo: HEAD is detached at %s, not on a branch", head.Hash())
	}
	return head.Target().Short(), nil
}

// CheckoutDetached moves HEAD to sha without updating any branch ref, or
// returns ErrInvalidSHA if sha is invalid.
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

// RestoreBranch moves HEAD back onto ref, ending the detached-HEAD state.
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

// ChangedFilesSince returns repo-relative paths that differ between sha and
// HEAD, considering committed history only. Returns ErrInvalidSHA if sha is
// invalid. The returned order is not contractual.
func (r *Repo) ChangedFilesSince(sha string) ([]string, error) {
	if !validSHA(sha) {
		return nil, ErrInvalidSHA
	}

	repo, err := r.goGit()
	if err != nil {
		return nil, err
	}

	fromTree, err := lookupObjectRetrying(r, repo, func() (*object.Tree, error) {
		return treeForRev(repo, sha)
	})
	if err != nil {
		return nil, fmt.Errorf("gitrepo: resolve tree for %s: %w", sha, err)
	}

	r.goGitMu.RLock()
	head, err := repo.Head()
	r.goGitMu.RUnlock()
	if err != nil {
		return nil, fmt.Errorf("gitrepo: resolve HEAD: %w", err)
	}

	headTree, err := lookupObjectRetrying(r, repo, func() (*object.Tree, error) {
		return treeForRev(repo, head.Hash().String())
	})
	if err != nil {
		return nil, fmt.Errorf("gitrepo: resolve tree for HEAD: %w", err)
	}

	changes, err := object.DiffTree(fromTree, headTree)
	if err != nil {
		return nil, fmt.Errorf("gitrepo: diff trees: %w", err)
	}

	var files []string
	for _, change := range changes {
		action, err := change.Action()
		if err != nil {
			return nil, fmt.Errorf("gitrepo: classify change: %w", err)
		}
		switch action {
		case merkletrie.Delete:
			files = append(files, change.From.Name)
		default:
			// Insert and Modify both carry the current path on the To side;
			// DiffTree never reports renames (see above), so Modify's From
			// and To names are always identical.
			files = append(files, change.To.Name)
		}
	}
	return files, nil
}

// treeForRev resolves rev to its commit's tree.
func treeForRev(repo *git.Repository, rev string) (*object.Tree, error) {
	commit, err := commitByHash(repo, rev)
	if err != nil {
		return nil, err
	}
	return commit.Tree()
}
