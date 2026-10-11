// pushretry.go — the runner and sleep seam behind Add's two branch pushes.
// The bounded retry of a plain, never-forced `git push -u origin <branch>` is gitrepo's: Add's branch pushes delegate to it.

package fabricengine

import (
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// pushSeam holds the runner and the sleep behind Add's branch pushes.
// A nil field means production: gitexec.Run and gitrepo's real backoff sleep.
type pushSeam struct {
	run   func(args []string, cwd string) (string, error)
	sleep func(delay time.Duration)
}

func (s pushSeam) runPush(args []string, cwd string) (string, error) {
	if s.run == nil {
		return gitexec.Run(args, cwd)
	}
	return s.run(args, cwd)
}

// pushBranchWithRetry runs `git push -u origin <branch>` in dir and returns the last error.
// The retry of a remote refusal of branch is gitrepo.PushWithRetry's, so an exhausted one wraps gitrepo.ErrPushRetriesExhausted.
func (s pushSeam) pushBranchWithRetry(dir, branch string) error {
	return gitrepo.PushWithRetry(func() error {
		_, err := s.runPush([]string{"push", "-u", "origin", branch}, dir)
		return err
	}, branch, s.sleep)
}
