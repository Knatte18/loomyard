// gitrepo.go defines the Repo type and its read/commit primitives: New, the
// shared run helper over gitexec.RunGit, CurrentSHA, and StageAndCommit.
// Later cards in this package add ChangedFilesSince and SHAExists on the
// same type.

package gitrepo

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// ErrNoCommits is returned by CurrentSHA when the repository has no commits
// yet (a freshly-initialized repo with an unborn HEAD), so callers get a
// typed signal instead of an ambiguous empty SHA string.
var ErrNoCommits = errors.New("gitrepo: repository has no commits")

// Repo is a typed handle on one local git checkout, identified by its
// filesystem path. Repo does not create, clone, or validate the checkout at
// construction time — see New. Methods on a single Repo are not
// goroutine-safe for concurrent writes to the same underlying checkout;
// cross-process push serialization is handled separately by the coalescing
// pusher's single-pusher lock, and in-process callers must serialize their
// own writes.
type Repo struct {
	path string
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
// stage — see the explicit-file-lists decision) and commits them with msg.
// When the listed files produce no staged change — including when files is
// empty, which stages nothing — StageAndCommit returns ("", false, nil): a
// plain signal, not an error, since "nothing to commit" is an expected,
// inspectable outcome rather than a failure. On a real commit it returns the
// new HEAD SHA with committed=true.
func (r *Repo) StageAndCommit(msg string, files []string) (sha string, committed bool, err error) {
	addArgs := append([]string{"add", "--"}, files...)
	_, stderr, code, err := r.run(addArgs...)
	if err != nil {
		return "", false, err
	}
	if code != 0 {
		return "", false, fmt.Errorf("gitrepo: git add: %s", stderr)
	}

	// `diff --cached --quiet` reports via exit code alone: 0 means the
	// staged tree matches HEAD (nothing to commit), 1 means it differs
	// (proceed to commit). Any other exit is a genuine git failure.
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
