// snapshot.go implements per-consumer snapshot-SHA tracking stored as git refs
// under refs/loomyard/snapshot/<key>, pushed to the repo's remote so state is
// shared across clones rather than confined to one worktree. Writes are
// fast-forward-only with adopt-on-conflict; reads fetch first but degrade to
// the local ref on a fetch failure.

package gitrepo

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrInvalidSnapshotKey is returned by SnapshotSHA and SetSnapshotSHA when key
// does not satisfy validSnapshotKey — surfaced before the key ever becomes
// part of a ref name, so a typo fails loudly instead of producing a corrupt
// or colliding ref.
var ErrInvalidSnapshotKey = errors.New("gitrepo: invalid snapshot key")

// snapshotKeyPattern matches a snapshot key: it must start with an
// alphanumeric character and contain only alphanumerics, '.', '_', or '-'
// thereafter. This excludes whitespace, the ref-illegal characters
// (~ ^ : ? * [ \), and leading/trailing '/' by construction (the pattern
// never matches '/' at all).
var snapshotKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// validSnapshotKey reports whether key is safe to embed in a git ref name.
// Beyond the character-class check, it separately rejects the shapes that are
// legal under the character class but refused by git ref syntax — ".." (a
// path-traversal-like separator), a trailing ".", and a ".lock" suffix
// (reserved for git's own ref lock files) — so a key can never produce an
// ambiguous or malformed ref: an invalid key always surfaces as
// ErrInvalidSnapshotKey before reaching git, never as a raw git error.
func validSnapshotKey(key string) bool {
	return snapshotKeyPattern.MatchString(key) &&
		!strings.Contains(key, "..") &&
		!strings.HasSuffix(key, ".") &&
		!strings.HasSuffix(key, ".lock")
}

// snapshotRef returns the fully-qualified ref name a snapshot key is stored
// under. It performs no validation itself; callers must check
// validSnapshotKey first.
func snapshotRef(key string) string {
	return "refs/loomyard/snapshot/" + key
}

// remoteName resolves the remote that the current branch is configured to
// track (via git symbolic-ref and branch.<name>.remote), falling back to
// "origin" when no such configuration exists — matching the assumption
// throughout gitrepo that every real consumer's repo has a conventional
// single "origin" remote unless it has explicitly set up branch tracking.
// When that assumption is violated (only remote named differently, no
// tracking), the fallback names a nonexistent remote: SnapshotSHA's
// best-effort fetch then degrades silently to the local ref while
// SetSnapshotSHA's push fails loudly — see the package doc's snapshot
// remote model for why that asymmetry is acceptable.
func (r *Repo) remoteName() string {
	stdout, _, code, err := r.run("symbolic-ref", "--short", "HEAD")
	if err != nil || code != 0 {
		return "origin"
	}
	branch := strings.TrimSpace(stdout)

	stdout, _, code, err = r.run("config", "--get", "branch."+branch+".remote")
	if err != nil || code != 0 {
		return "origin"
	}
	remote := strings.TrimSpace(stdout)
	if remote == "" {
		return "origin"
	}
	return remote
}

// SnapshotSHA returns the SHA currently recorded for key under
// refs/loomyard/snapshot/<key>, or ("", nil) if no such ref has ever been
// set. It first attempts a best-effort fetch of the whole snapshot
// namespace from the repo's remote so the read reflects any advance made by
// another clone; a fetch failure (offline, transient network issue) is
// swallowed rather than surfaced, and the read degrades to the last-known
// local ref value — consistent with SHAExists' failure-swallowing posture,
// since a slightly-stale snapshot at worst re-processes already-done work.
// Only a verified-absent ref reads as ("", nil); a checkout that cannot be
// read at all (path is not a git repository, corrupt ref store) surfaces as
// an error instead, so a miswired consumer is not told "no snapshot" forever.
func (r *Repo) SnapshotSHA(key string) (string, error) {
	if !validSnapshotKey(key) {
		return "", ErrInvalidSnapshotKey
	}

	remote := r.remoteName()
	// Best-effort: ignore both the spawn error and a non-zero exit code, since
	// an unreachable remote must not block a read that can fall back to the
	// local ref.
	r.run("fetch", remote, "+refs/loomyard/snapshot/*:refs/loomyard/snapshot/*")

	ref := snapshotRef(key)
	stdout, stderr, code, err := r.run("rev-parse", "--verify", "--quiet", ref)
	if err != nil {
		return "", err
	}
	switch code {
	case 0:
		return strings.TrimSpace(stdout), nil
	case 1:
		// `--verify --quiet` reports a missing ref as exit 1 with no output;
		// the ref does not exist yet — absent is a normal state, not a failure.
		return "", nil
	default:
		// Any other exit (128: not a git repository, corrupt ref store) is a
		// genuine failure — folding it into "absent" would let a miswired
		// read-only consumer reprocess from scratch forever with no signal.
		return "", fmt.Errorf("gitrepo: git rev-parse --verify %s: %s", ref, stderr)
	}
}

// snapshotPushMaxAttempts bounds the adopt-on-conflict retry loop in
// SetSnapshotSHA so it always terminates. Every attempt past the first is
// taken only when the caller is strictly newer than the value it just adopted,
// so the loop advances monotonically toward sha and in practice needs at most
// as many attempts as there are concurrent writers; the cap is a safety net
// against pathological sustained contention. Hitting it is behaviourally
// identical to giving up — the value is re-derived and re-set on the caller's
// next cycle, per the self-correcting snapshot model — so a generous cap costs
// nothing.
const snapshotPushMaxAttempts = 8

// SetSnapshotSHA records sha as the value for key, advancing
// refs/loomyard/snapshot/<key> locally and then pushing it to the repo's
// remote fast-forward-only. On a rejected push SetSnapshotSHA fetches and
// adopts the remote's value into the local ref rather than treating the
// rejection as an error — per discussion.md's safe model, a key advances
// along a single monotonic line, so a rejection normally means someone else
// processed further and their SHA is the correct one to take. One rejection
// shape breaks that assumption: transient contention — a remote-side creation
// race that rejects the loser with "reference already exists" regardless of
// ancestry, or a lost ref-lock race under concurrent writers — can reject a
// push whose sha is actually strictly newer than the adopted value. When the
// adopted value turns out to be a strict ancestor of sha, SetSnapshotSHA is
// therefore genuinely further along, so it re-advances the local ref and
// retries, looping (bounded by snapshotPushMaxAttempts) until its push lands,
// the remote genuinely advances to a value that is not a strict ancestor of
// sha (adopt it and stop), or the cap is reached. A single retry is not enough
// under three or more concurrent writers, where the one retry can itself lose
// transient contention and silently drop a strictly-newer value. Any other
// push failure returns an error including git's stderr. A non-hex sha returns
// ErrInvalidSHA (checkable via errors.Is) without spawning git — an
// option-shaped value must never reach update-ref, where e.g. "-d" would
// delete the ref instead of setting it.
func (r *Repo) SetSnapshotSHA(key, sha string) error {
	if !validSnapshotKey(key) {
		return ErrInvalidSnapshotKey
	}
	if !validSHA(sha) {
		return ErrInvalidSHA
	}

	ref := snapshotRef(key)
	remote := r.remoteName()

	for attempt := 0; attempt < snapshotPushMaxAttempts; attempt++ {
		rejected, err := r.advanceAndPushSnapshotRef(remote, ref, sha)
		if err != nil || !rejected {
			return err
		}

		// The push was rejected; adopt the remote's current value so the local
		// ref converges — and so we can tell a genuine advance past sha from
		// transient contention that merely raced us.
		if err := r.adoptSnapshotRef(remote, ref); err != nil {
			return err
		}

		adopted, _, code, err := r.run("rev-parse", "--verify", "--quiet", ref)
		if err != nil {
			return err
		}
		// If the adopted value is not a strict ancestor of sha, another writer
		// genuinely processed at least as far as us: the monotonic line says
		// their value is the correct one to keep, so stop with it adopted.
		if code != 0 || !r.isStrictDescendant(strings.TrimSpace(adopted), sha) {
			return nil
		}
		// Otherwise sha strictly descends from the adopted value — we lost only
		// transient contention while actually being further along — so loop and
		// retry the push rather than silently dropping the strictly-newer value.
	}
	return nil
}

// advanceAndPushSnapshotRef sets ref to sha locally and pushes it to
// remote. It reports rejected=true when the push failed with a recoverable
// rejection (the rebaseRetryTriggers set — non-fast-forward or a creation
// race's "reference already exists"), leaving the caller to run the
// adopt-on-conflict protocol; any other failure is returned as an error.
func (r *Repo) advanceAndPushSnapshotRef(remote, ref, sha string) (rejected bool, err error) {
	_, stderr, code, err := r.run("update-ref", ref, sha)
	if err != nil {
		return false, err
	}
	if code != 0 {
		return false, fmt.Errorf("gitrepo: git update-ref %s: %s", ref, stderr)
	}

	_, stderr, code, err = r.run("push", remote, ref)
	if err != nil {
		return false, err
	}
	if code == 0 {
		return false, nil
	}
	if !containsAny(stderr, rebaseRetryTriggers) {
		return false, fmt.Errorf("gitrepo: git push %s %s: %s", remote, ref, stderr)
	}
	return true, nil
}

// adoptSnapshotRef force-fetches ref's current value on remote into the
// local ref — the adopt half of SetSnapshotSHA's adopt-on-conflict protocol.
func (r *Repo) adoptSnapshotRef(remote, ref string) error {
	_, stderr, code, err := r.run("fetch", remote, "+"+ref+":"+ref)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("gitrepo: git fetch %s +%s:%s (adopt-on-conflict): %s", remote, ref, ref, stderr)
	}
	return nil
}

// isStrictDescendant reports whether descendant is a commit strictly ahead
// of ancestor along one line of history: ancestor is reachable from
// descendant and the two are not the same commit. Any git failure (e.g. an
// object missing locally) reports false, which callers treat as "not
// provably ahead — do not retry".
func (r *Repo) isStrictDescendant(ancestor, descendant string) bool {
	if ancestor == descendant {
		return false
	}
	_, _, code, err := r.run("merge-base", "--is-ancestor", ancestor, descendant)
	return err == nil && code == 0
}
