// pull.go implements the fast-forward-only pull and the fetch beside it.
// Pull runs `git pull --ff-only` against this checkout.
// It refuses to create a merge commit when the local branch has diverged from its upstream.
// Fetch refreshes the remote-tracking refs and never touches the local branch.
// A caller uses it to inspect what changed upstream before it reconciles.
// Both run git under the repository's remote-refs lock, since both write remote-tracking refs.

package gitrepo

import "fmt"

// Pull fast-forwards the current branch from upstream via `git pull --ff-only` and refuses a diverged branch.
// Errors name the repo path and git's exit code, and give the command that reproduces git's own diagnosis.
// Raw stderr stays out of the error, and pull_test.go enforces that.
// The reproduction command is what keeps a nonzero exit diagnosable instead of a bare number.
func (r *Repo) Pull() error {
	var code int
	err := r.withRemoteRefs(func() (runErr error) {
		//gitexec:raw — a non-zero exit here IS a failure, but folding git's stderr into the message would break pull_test.go's test-enforced no-`fatal:`-leak surface; the reproduction pointer stands in for the stderr.
		_, _, code, runErr = r.run("pull", "--ff-only")
		return runErr
	})
	if err != nil {
		return fmt.Errorf("gitrepo: pull --ff-only in %s: %w", r.path, err)
	}
	if code != 0 {
		return fmt.Errorf("gitrepo: pull --ff-only in %s: git exited %d (run `git -C %s pull --ff-only` for git's own diagnosis)", r.path, code, r.path)
	}
	return nil
}

// Fetch refreshes remote-tracking refs via `git fetch` and leaves the local branch where it is.
// Errors name the repo path and git's exit code, and give the command that reproduces git's own diagnosis.
// Raw stderr stays out of the error.
func (r *Repo) Fetch() error {
	var code int
	err := r.withRemoteRefs(func() (runErr error) {
		//gitexec:raw — a non-zero exit here IS a failure, but folding git's stderr into the message would break fetch_integration_test.go's test-enforced no-`fatal:`-leak surface; the reproduction pointer stands in for the stderr.
		_, _, code, runErr = r.run("fetch")
		return runErr
	})
	if err != nil {
		return fmt.Errorf("gitrepo: fetch in %s: %w", r.path, err)
	}
	if code != 0 {
		return fmt.Errorf("gitrepo: fetch in %s: git exited %d (run `git -C %s fetch` for git's own diagnosis)", r.path, code, r.path)
	}
	return nil
}
