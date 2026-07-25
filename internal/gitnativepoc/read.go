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
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
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

// ErrInvalidSnapshotKey mirrors gitrepo.ErrInvalidSnapshotKey: SnapshotSHA
// returns it when key does not satisfy validSnapshotKey, surfaced before the
// key ever becomes part of a ref name.
var ErrInvalidSnapshotKey = errors.New("gitnativepoc: invalid snapshot key")

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

// snapshotKeyPattern mirrors gitrepo's snapshotKeyPattern: a snapshot key
// must start with an alphanumeric character and contain only alphanumerics,
// '.', '_', or '-' thereafter.
var snapshotKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// validSnapshotKey reports whether key is safe to embed in a git ref name,
// mirroring gitrepo.validSnapshotKey's character-class-plus-shape checks: it
// separately rejects ".." (path-traversal-like), a trailing ".", and a
// ".lock" suffix (reserved for git's own ref lock files) — shapes the
// character class alone would still allow through.
func validSnapshotKey(key string) bool {
	return snapshotKeyPattern.MatchString(key) &&
		!strings.Contains(key, "..") &&
		!strings.HasSuffix(key, ".") &&
		!strings.HasSuffix(key, ".lock")
}

// snapshotRefPrefix is the ref namespace snapshot keys live under, mirroring
// gitrepo's snapshotRef.
const snapshotRefPrefix = "refs/loomyard/snapshot/"

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

// remoteName resolves the remote the current branch is configured to track
// via go-git's HEAD symbolic reference and branch config, falling back to
// "origin" when no such configuration exists, mirroring gitrepo.remoteName
// exactly (including its fallback rationale — see gitrepo's snapshot.go)
// via go-git's Reference and Config lookups instead of
// `git symbolic-ref --short HEAD` plus `git config --get branch.<b>.remote`.
func (r *Repo) remoteName() string {
	head, err := r.repo.Reference(plumbing.HEAD, false)
	if err != nil || head.Type() != plumbing.SymbolicReference {
		return "origin"
	}
	branch := head.Target().Short()

	cfg, err := r.repo.Config()
	if err != nil {
		return "origin"
	}
	b, ok := cfg.Branches[branch]
	if !ok || b.Remote == "" {
		return "origin"
	}
	return b.Remote
}

// SnapshotSHA returns the SHA currently recorded for key under
// refs/loomyard/snapshot/<key>, or ("", nil) if no such ref has ever been
// set, mirroring gitrepo.SnapshotSHA's contract. MIGRATE: go-git's
// Repository.Fetch accepts the same custom
// +refs/loomyard/snapshot/*:refs/loomyard/snapshot/* refspec gitrepo shells
// out for (verified against a real bare remote during this spike); the fetch
// is best-effort — its error is ignored, exactly like gitrepo swallowing an
// unreachable-remote `git fetch` failure — so the read degrades to the local
// ref rather than failing outright. Only a verified-absent ref reads as
// ("", nil); a corrupt/non-repo store surfaces as an error.
func (r *Repo) SnapshotSHA(key string) (string, error) {
	if !validSnapshotKey(key) {
		return "", ErrInvalidSnapshotKey
	}

	remote := r.remoteName()
	// Best-effort: ignore the fetch error entirely (unreachable remote,
	// nothing new to fetch, no such remote configured at all) — an
	// unreachable remote must not block a read that can fall back to the
	// local ref.
	r.repo.Fetch(&git.FetchOptions{
		RemoteName: remote,
		RefSpecs:   []config.RefSpec{config.RefSpec("+" + snapshotRefPrefix + "*:" + snapshotRefPrefix + "*")},
	})

	ref, err := r.repo.Reference(plumbing.ReferenceName(snapshotRefPrefix+key), true)
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			// `--verify --quiet` reports a missing ref as exit 1 with no
			// output on the CLI side; the ref simply does not exist yet —
			// absent is a normal state, not a failure.
			return "", nil
		}
		return "", fmt.Errorf("gitnativepoc: resolve %s: %w", snapshotRefPrefix+key, err)
	}
	return ref.Hash().String(), nil
}

// hasUnpushed reports whether HEAD is ahead of its upstream tracking
// branch, mirroring gitrepo.hasUnpushed's contract: no upstream configured
// reports true (unpushed), not an error, so the first push — which
// establishes the tracking branch — still happens rather than being skipped
// as "nothing to do". go-git's revision parser recognizes "@{u}" syntax, but
// Repository.ResolveRevision never implements the resulting AtUpstream case
// (confirmed empirically during this spike; ResolveRevision(plumbing.Revision("@{u}"))
// always fails, even against a real upstream-tracking clone) — CLI-BOUND for
// that specific syntax. This method reaches the same *behavioural* result by
// resolving the upstream ref manually from branch config and comparing
// commit reachability directly, which is MIGRATE: `git rev-list --count
// @{u}..HEAD`'s set-difference semantics are reproduced by first walking the
// upstream commit's own history to build its full reachable-commit set, then
// walking from HEAD with that whole set passed as object.NewCommitPreorderIter's
// seenExternal map — NewCommitPreorderIter's ignore list only seeds the
// literal listed hashes into its "seen" map, not their ancestors, so seeding
// just the upstream hash itself (rather than everything reachable from it)
// would wrongly count HEAD as ahead whenever HEAD is a strict ancestor of
// upstream (purely behind, nothing to push) — confirmed by reading
// plumbing/object/commit_walker.go in the pinned module. Precomputing the
// full ancestor set first fixes that: this holds for a diverged/rebased
// history too, not just a fast-forward one, because reachability (not linear
// position) is what both sides actually compute.
func (r *Repo) hasUnpushed() (bool, error) {
	symbolicHead, err := r.repo.Reference(plumbing.HEAD, false)
	if err != nil || symbolicHead.Type() != plumbing.SymbolicReference {
		// A detached HEAD or an unreadable ref has no branch to carry
		// tracking configuration, so there is nothing to compare against —
		// treat it the same as "no upstream configured".
		return true, nil
	}
	branch := symbolicHead.Target().Short()

	cfg, err := r.repo.Config()
	if err != nil {
		return true, nil
	}
	b, ok := cfg.Branches[branch]
	if !ok || b.Remote == "" || b.Merge == "" {
		return true, nil
	}

	upstreamRef, err := r.repo.Reference(plumbing.NewRemoteReferenceName(b.Remote, b.Merge.Short()), true)
	if err != nil {
		// Tracking is configured but the remote-tracking ref itself is
		// missing locally (never fetched, for instance) — nothing to
		// compare against yet, so fall back to the same "no upstream"
		// treatment gitrepo's `@{u}..HEAD` failure path uses.
		return true, nil
	}

	head, err := r.repo.Head()
	if err != nil {
		return false, err
	}
	headCommit, err := r.repo.CommitObject(head.Hash())
	if err != nil {
		return false, err
	}
	upstreamCommit, err := r.repo.CommitObject(upstreamRef.Hash())
	if err != nil {
		return false, err
	}

	// Walk the full history reachable from upstream first, so the HEAD walk
	// below can exclude everything upstream can already reach — not just the
	// upstream tip itself — reproducing @{u}..HEAD's set-difference rather
	// than a single-hash exclusion.
	reachableFromUpstream := make(map[plumbing.Hash]bool)
	upstreamIter := object.NewCommitPreorderIter(upstreamCommit, nil, nil)
	if err := upstreamIter.ForEach(func(c *object.Commit) error {
		reachableFromUpstream[c.Hash] = true
		return nil
	}); err != nil {
		return false, err
	}

	ahead := 0
	iter := object.NewCommitPreorderIter(headCommit, reachableFromUpstream, nil)
	if err := iter.ForEach(func(*object.Commit) error {
		ahead++
		return nil
	}); err != nil {
		return false, err
	}
	return ahead != 0, nil
}

// isStrictDescendant reports whether descendant is a commit strictly ahead
// of ancestor along reachable history — ancestor is reachable from
// descendant and the two are not the same commit — mirroring
// gitrepo.isStrictDescendant via a go-git commit walk instead of
// `git merge-base --is-ancestor`. Any resolution or walk failure reports
// false, matching gitrepo's "not provably ahead — do not retry" posture.
// MIGRATE.
func (r *Repo) isStrictDescendant(ancestor, descendant string) bool {
	if ancestor == descendant {
		return false
	}

	descHash, err := r.repo.ResolveRevision(plumbing.Revision(descendant))
	if err != nil {
		return false
	}
	ancestorHash, err := r.repo.ResolveRevision(plumbing.Revision(ancestor))
	if err != nil {
		return false
	}
	descCommit, err := r.repo.CommitObject(*descHash)
	if err != nil {
		return false
	}

	found := false
	iter := object.NewCommitPreorderIter(descCommit, nil, nil)
	err = iter.ForEach(func(c *object.Commit) error {
		if c.Hash == *ancestorHash {
			found = true
			return storer.ErrStop
		}
		return nil
	})
	if err != nil {
		return false
	}
	return found
}
