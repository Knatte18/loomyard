// pull.go implements the fast-forward-only pull surface: Pull runs `git pull --ff-only` against
// this Repo's checkout, refusing to create a merge commit when the local branch has diverged from
// its upstream.
// Fetch sits alongside Pull as the fetch-without-merge primitive: it refreshes this checkout's
// remote-tracking refs without ever touching the local branch, the split fabric.Fabric.Pull needs
// to inspect what changed upstream before deciding how to reconcile.

package gitrepo

import "fmt"

// Pull fast-forwards the current branch from upstream via `git pull --ff-only`, refusing a diverged
// branch.
// Errors name the repo path and git's exit code.
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

// Fetch refreshes remote-tracking refs via `git fetch` without moving the local branch, unlike
// Pull.
// Errors name the repo path and git's exit code.
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
