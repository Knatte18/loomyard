// pull.go implements the fast-forward-only pull surface: Pull runs
// `git pull --ff-only` against this Repo's checkout, refusing to create a
// merge commit when the local branch has diverged from its upstream. Fetch
// sits alongside Pull as the fetch-without-merge primitive: it refreshes
// this checkout's remote-tracking refs without ever touching the local
// branch, the split fabric.Fabric.Pull needs to inspect what changed
// upstream before deciding how to reconcile.

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

// Fetch refreshes this Repo's remote-tracking refs (e.g. `@{u}` /
// `origin/<branch>`) via `git fetch`, without merging or moving the local
// branch — unlike Pull's `git pull --ff-only`, which fast-forwards HEAD
// itself. Fetch exists for a caller that needs to inspect what changed
// upstream (via IsAncestor or a diff against the refreshed remote-tracking
// ref) before deciding how, or whether, to advance the local branch. On a
// spawn failure the underlying error is wrapped and returned; on any other
// non-zero exit the returned error names the repo path and git's exit code
// without including raw stderr, matching Pull's no-stderr-leak error style.
func (r *Repo) Fetch() error {
	_, _, code, err := r.run("fetch")
	if err != nil {
		return fmt.Errorf("gitrepo: fetch in %s: %w", r.path, err)
	}
	if code != 0 {
		return fmt.Errorf("gitrepo: fetch in %s: git exited %d", r.path, code)
	}
	return nil
}
