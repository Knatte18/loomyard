// pull.go implements the fast-forward-only pull and the fetch beside it.
// Pull runs `git pull --ff-only` against this checkout.
// It refuses to create a merge commit when the local branch has diverged from its upstream.
// Fetch refreshes the remote-tracking refs and drops those of branches the remote deleted.
// It prunes remote-tracking refs only and never a local tag.
// It never touches the local branch.
// A caller uses it to inspect what changed upstream before it reconciles.
// Both run git under the repository's remote-refs lock, since both write remote-tracking refs.

package gitrepo

import (
	"errors"
	"fmt"
	"strings"
)

// fetchRemote is the remote whose fetch refspecs Fetch reads to prune.
// A checkout with no remote of this name is fetched but never pruned.
const fetchRemote = "origin"

// remoteTrackingPrefix opens the destination of a fetch refspec that writes remote-tracking refs.
const remoteTrackingPrefix = "refs/remotes/"

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
// It then drops the tracking ref of every branch the remote no longer holds,
// so a reader of the fetched refs never sees a branch deleted on the remote as present.
// The prune covers only the remote's fetch refspecs whose destination is under refs/remotes/,
// so a local tag is never pruned, even one a configured refspec fetches into refs/tags/.
// Errors name the repo path and git's exit code, and give the command that reproduces git's own diagnosis.
// Raw stderr stays out of the error.
func (r *Repo) Fetch() error {
	var code int
	var failedCommand string
	err := r.withRemoteRefs(func() error {
		var runErr error
		failedCommand = "fetch"
		//gitexec:raw — a non-zero exit here IS a failure, but folding git's stderr into the message would break fetch_integration_test.go's test-enforced no-`fatal:`-leak surface; the reproduction pointer stands in for the stderr.
		_, _, code, runErr = r.run("fetch")
		if runErr != nil || code != 0 {
			return runErr
		}
		refspecs, runErr := r.RemoteFetchRefspecs(fetchRemote)
		if errors.Is(runErr, ErrRemoteNotConfigured) {
			return nil
		}
		if runErr != nil {
			return runErr
		}
		pruned := remoteTrackingRefspecs(refspecs)
		if len(pruned) == 0 {
			return nil
		}
		args := append([]string{"fetch", "--prune", "--no-tags", fetchRemote}, pruned...)
		quoted := make([]string, len(pruned))
		for i, refspec := range pruned {
			quoted[i] = "'" + refspec + "'"
		}
		failedCommand = strings.Join(append([]string{"fetch", "--prune", "--no-tags", fetchRemote}, quoted...), " ")
		//gitexec:raw — same justification as the plain fetch above.
		_, _, code, runErr = r.run(args...)
		return runErr
	})
	if err != nil {
		return fmt.Errorf("gitrepo: fetch in %s: %w", r.path, err)
	}
	if code != 0 {
		return fmt.Errorf("gitrepo: fetch in %s: git exited %d (run `git -C %s %s` for git's own diagnosis)", r.path, code, r.path, failedCommand)
	}
	return nil
}

// remoteTrackingRefspecs returns the refspecs whose destination lies under refs/remotes/.
// A refspec with no destination, such as a negative one, never qualifies.
func remoteTrackingRefspecs(refspecs []string) []string {
	var kept []string
	for _, refspec := range refspecs {
		_, destination, hasDestination := strings.Cut(refspec, ":")
		if hasDestination && strings.HasPrefix(destination, remoteTrackingPrefix) {
			kept = append(kept, refspec)
		}
	}
	return kept
}
