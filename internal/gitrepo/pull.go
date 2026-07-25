// pull.go implements the fast-forward-only pull surface: Pull runs
// `git pull --ff-only` against this Repo's checkout, refusing to create a
// merge commit when the local branch has diverged from its upstream.

package gitrepo

import "fmt"

// Pull fast-forwards this Repo's current branch from its configured
// upstream via `git pull --ff-only`. Pull is fast-forward-only by contract:
// a diverged branch — local commits the upstream lacks — is refused by git
// and surfaces here as an error, never as a merge commit; recovering from
// divergence is left as a caller policy, not something gitrepo attempts on
// its own. On a spawn failure the underlying error is wrapped and returned;
// on any other non-zero exit the returned error names the repo path and
// git's exit code without including raw stderr, so a diverged-branch or
// no-remote failure never leaks git's own "fatal:"-prefixed message text.
func (r *Repo) Pull() error {
	_, _, code, err := r.run("pull", "--ff-only")
	if err != nil {
		return fmt.Errorf("gitrepo: pull --ff-only in %s: %w", r.path, err)
	}
	if code != 0 {
		return fmt.Errorf("gitrepo: pull --ff-only in %s: git exited %d", r.path, code)
	}
	return nil
}
