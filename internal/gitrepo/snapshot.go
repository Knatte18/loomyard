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
// Beyond the character-class check, it separately rejects ".." — legal under
// the character class but reserved by git ref syntax as a path-traversal-like
// separator — so a key can never produce an ambiguous or malformed ref.
func validSnapshotKey(key string) bool {
	return snapshotKeyPattern.MatchString(key) && !strings.Contains(key, "..")
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
	stdout, _, code, err := r.run("rev-parse", "--verify", "--quiet", ref)
	if err != nil {
		return "", err
	}
	if code != 0 {
		// The ref does not exist yet; absent is a normal state, not a failure.
		return "", nil
	}
	return strings.TrimSpace(stdout), nil
}

// SetSnapshotSHA records sha as the value for key, advancing
// refs/loomyard/snapshot/<key> locally and then pushing it to the repo's
// remote fast-forward-only. On a rejected push — another clone already
// advanced this key past sha — SetSnapshotSHA fetches and adopts the
// remote's value into the local ref and returns nil rather than an error:
// per discussion.md's safe model, a key advances along a single monotonic
// line, so a rejection means someone else processed further and their SHA is
// the correct one to take. Any other push failure returns an error
// including git's stderr. A non-hex sha returns ErrInvalidSHA (checkable via
// errors.Is) without spawning git — an option-shaped value must never reach
// update-ref, where e.g. "-d" would delete the ref instead of setting it.
func (r *Repo) SetSnapshotSHA(key, sha string) error {
	if !validSnapshotKey(key) {
		return ErrInvalidSnapshotKey
	}
	if !validSHA(sha) {
		return ErrInvalidSHA
	}

	ref := snapshotRef(key)
	_, stderr, code, err := r.run("update-ref", ref, sha)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("gitrepo: git update-ref %s: %s", ref, stderr)
	}

	remote := r.remoteName()
	_, stderr, code, err = r.run("push", remote, ref)
	if err != nil {
		return err
	}
	if code == 0 {
		return nil
	}

	if !containsAny(stderr, rebaseRetryTriggers) {
		return fmt.Errorf("gitrepo: git push %s %s: %s", remote, ref, stderr)
	}

	// The remote already has a newer value for this key; adopt it into our
	// local ref rather than treating the rejection as a failure — the
	// rejecting clone processed further along the same monotonic line.
	_, stderr, code, err = r.run("fetch", remote, "+"+ref+":"+ref)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("gitrepo: git fetch %s +%s:%s (adopt-on-conflict): %s", remote, ref, ref, stderr)
	}
	return nil
}
