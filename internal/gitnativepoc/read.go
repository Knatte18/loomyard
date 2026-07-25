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
	"regexp"

	"github.com/go-git/go-git/v5/plumbing"
)

// ErrNoCommits mirrors gitrepo.ErrNoCommits: CurrentSHA returns it when the
// checkout's HEAD is unborn (a freshly-initialized repo with no commits
// yet), so callers get a typed signal instead of an ambiguous empty SHA.
var ErrNoCommits = errors.New("gitnativepoc: repository has no commits")

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
