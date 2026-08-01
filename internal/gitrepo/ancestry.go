// ancestry.go implements the reachability primitive rebase detection and the
// nearest-older anchor walk both need: IsAncestor answers "is sha an
// ancestor of ref", CLI-bound via `git merge-base --is-ancestor` because
// SHAExists' object-existence semantics cannot distinguish "this commit is
// still reachable from ref" from "this commit's object merely survived a
// rebase that walked it off history" — see the package's reachability,
// never object-existence decision.

package gitrepo

import (
	"fmt"
	"strings"
)

// IsAncestor reports whether sha is an ancestor of ref, via
// `git merge-base --is-ancestor sha ref`. It maps git's tri-state exit code
// directly rather than folding any non-zero exit into failure: exit 0 means
// sha IS an ancestor of ref (true, nil); exit 1 means sha is NOT an ancestor
// (false, nil) — a genuine, well-formed answer, not an error; any other
// non-zero exit is a real failure (e.g. sha or ref does not exist in this
// repo), returned as an error naming the repo path and git's exit code
// without embedding raw stderr, matching Pull's no-stderr-leak error style.
// A spawn failure wraps and returns the underlying error.
//
// sha is validated with validSHA before ever reaching git, returning
// ErrInvalidSHA (checkable via errors.Is) on a non-hex value, exactly like
// ResetHard. ref is validated only against a leading '-' — which git would
// otherwise parse as a flag rather than a revision — and is deliberately NOT
// required to be a plain hex SHA: callers of this primitive pass it a
// resolved commit SHA today, but the primitive itself must stay usable with
// symbolic refs (branch names, HEAD, etc.), so it uses this lighter guard
// rather than validSHA. Both bad-argument shapes surface the same
// ErrInvalidSHA, so a caller has one typed error to check regardless of
// which argument was malformed.
func (r *Repo) IsAncestor(sha, ref string) (bool, error) {
	if !validSHA(sha) {
		return false, ErrInvalidSHA
	}
	if strings.HasPrefix(ref, "-") {
		return false, ErrInvalidSHA
	}

	_, _, code, err := r.run("merge-base", "--is-ancestor", sha, ref)
	if err != nil {
		return false, fmt.Errorf("gitrepo: merge-base --is-ancestor %s %s in %s: %w", sha, ref, r.path, err)
	}
	switch code {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, fmt.Errorf("gitrepo: merge-base --is-ancestor %s %s in %s: git exited %d", sha, ref, r.path, code)
	}
}
