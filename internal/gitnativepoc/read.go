// read.go implements gitnativepoc's read-surface: go-git-backed counterparts
// to internal/gitrepo's read and inspection methods (CurrentSHA, SHAExists,
// ChangedFilesSince, SnapshotSHA, remoteName, hasUnpushed,
// isStrictDescendant), each built directly on go-git's object model rather
// than by shelling out to git. Every method here is exercised against the
// matching gitrepo.Repo method (or a direct git fixture, for gitrepo's
// unexported helpers) as the differential oracle in read_test.go, per the
// plan's differential-oracle Shared Decision; the MIGRATE/CLI-BOUND verdict
// for each surface is recorded as a comment on its test, not here.

package gitnativepoc

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/merkletrie"
)

// ErrNoCommits mirrors gitrepo.ErrNoCommits: CurrentSHA returns it when the
// checkout's HEAD is unborn (a freshly-initialized repo with no commits
// yet), so callers get a typed signal instead of an ambiguous empty SHA.
var ErrNoCommits = errors.New("gitnativepoc: repository has no commits")

// ErrInvalidSHA mirrors gitrepo.ErrInvalidSHA: ChangedFilesSince returns it
// when a caller-supplied sha argument is not a plain hex object name,
// surfaced before the string ever reaches go-git's revision resolver.
var ErrInvalidSHA = errors.New("gitnativepoc: invalid SHA")

// shaPattern mirrors gitrepo's shaPattern: a plain abbreviated-or-full hex
// object name (4 to 64 hex digits), deliberately excluding symbolic
// revisions such as HEAD or refs. It is re-declared here rather than
// imported from gitrepo so this package never depends on gitrepo's
// unexported surface.
var shaPattern = regexp.MustCompile(`^[0-9a-fA-F]{4,64}$`)

// validSHA reports whether sha is a plain hex object name, mirroring
// gitrepo.validSHA's hex-shape guard.
func validSHA(sha string) bool {
	return shaPattern.MatchString(sha)
}

// CurrentSHA returns the SHA of the checkout's current HEAD commit,
// mirroring gitrepo.CurrentSHA's contract via go-git's Repository.Head
// instead of `git rev-parse HEAD`. MIGRATE: go-git surfaces the unborn-HEAD
// case (a freshly-initialized repo with no commits) as
// plumbing.ErrReferenceNotFound, which this method translates into the
// package's own typed sentinel so callers never depend on a
// go-git-specific error value.
func (r *Repo) CurrentSHA() (string, error) {
	head, err := r.repo.Head()
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return "", ErrNoCommits
		}
		return "", err
	}
	return head.Hash().String(), nil
}

// SHAExists reports whether sha names a commit reachable in this Repo,
// mirroring gitrepo.SHAExists' failure-swallowing posture: a non-hex sha, a
// missing object, or an object that exists but is not a commit all fold
// into false rather than an error, since callers only use the result as a
// staleness signal ("when in doubt, rebuild") and never need to
// distinguish why a SHA came back absent. MIGRATE: go-git's
// Repository.ResolveRevision already peels an abbreviated-or-full hex name
// (and a tag, though callers here only ever pass commit SHAs) down to a
// commit object, matching `git rev-parse --verify --quiet <sha>^{commit}`.
func (r *Repo) SHAExists(sha string) bool {
	if !validSHA(sha) {
		return false
	}
	_, err := r.repo.ResolveRevision(plumbing.Revision(sha))
	return err == nil
}

// ChangedFilesSince returns the repo-relative paths that differ between sha
// and HEAD, considering committed history only, mirroring
// gitrepo.ChangedFilesSince's three tricky contracts via go-git's tree diff
// instead of `git diff --name-only -z --no-renames`:
//   - MIGRATE: a non-ASCII path comes back verbatim — go-git's tree entries
//     are raw UTF-8 bytes with no CLI-only C-quoting layer (the `-z` flag's
//     job) to strip.
//   - MIGRATE: a rename is reported as both its old path (deleted) and its
//     new path (added), never folded into one entry — achieved by calling
//     object.DiffTree directly rather than Tree.Diff/DiffTreeWithOptions,
//     which perform rename detection by default since go-git v5.1.0; DiffTree
//     is the --no-renames equivalent.
//   - A non-hex sha returns ErrInvalidSHA (checkable via errors.Is) without
//     resolving or diffing anything.
func (r *Repo) ChangedFilesSince(sha string) ([]string, error) {
	if !validSHA(sha) {
		return nil, ErrInvalidSHA
	}

	fromTree, err := r.commitTree(sha)
	if err != nil {
		return nil, fmt.Errorf("gitnativepoc: resolve tree for %s: %w", sha, err)
	}

	head, err := r.repo.Head()
	if err != nil {
		return nil, fmt.Errorf("gitnativepoc: resolve HEAD: %w", err)
	}
	headTree, err := r.commitTree(head.Hash().String())
	if err != nil {
		return nil, fmt.Errorf("gitnativepoc: resolve tree for HEAD: %w", err)
	}

	changes, err := object.DiffTree(fromTree, headTree)
	if err != nil {
		return nil, fmt.Errorf("gitnativepoc: diff trees: %w", err)
	}

	var files []string
	for _, change := range changes {
		action, err := change.Action()
		if err != nil {
			return nil, fmt.Errorf("gitnativepoc: classify change: %w", err)
		}
		switch action {
		case merkletrie.Delete:
			files = append(files, change.From.Name)
		default:
			// Insert and Modify both carry the current path on the To side;
			// DiffTree never reports renames (see the MIGRATE note above), so
			// Modify's From and To names are always identical.
			files = append(files, change.To.Name)
		}
	}
	return files, nil
}

// commitTree resolves rev (a full or abbreviated hex object name) to its
// commit's tree, the shared step CurrentSHA-adjacent methods need before
// diffing or otherwise inspecting a commit's content.
func (r *Repo) commitTree(rev string) (*object.Tree, error) {
	hash, err := r.repo.ResolveRevision(plumbing.Revision(rev))
	if err != nil {
		return nil, err
	}
	commit, err := r.repo.CommitObject(*hash)
	if err != nil {
		return nil, err
	}
	return commit.Tree()
}
