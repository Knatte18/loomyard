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

// IsAncestor reports whether sha is an ancestor of ref via
// `git merge-base --is-ancestor`, returning its tri-state exit code: true if
// an ancestor, false if not (both with nil error), or an error on failure.
// sha and ref are validated before reaching git, returning ErrInvalidSHA.
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
