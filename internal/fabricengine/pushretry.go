// pushretry.go — the bounded retry Add wraps around its two branch pushes.
// It repeats a plain, never-forced `git push -u origin <branch>` only when the server refuses the pushed ref with a bare "(failed)".

package fabricengine

import (
	"errors"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/logger"
)

// pushMaxAttempts is the total number of pushes a transiently refused push gets.
const pushMaxAttempts = 4

// pushBaseBackoff is the delay before the second attempt; each later delay doubles it.
const pushBaseBackoff = time.Second

// pushSeam holds the runner and the sleep behind Add's branch pushes.
// A nil field means production: gitexec.Run and a real sleep of the delay plus up to half of it as jitter.
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

func (s pushSeam) sleepFor(delay time.Duration) {
	if s.sleep == nil {
		time.Sleep(delay + rand.N(delay/2+1))
		return
	}
	s.sleep(delay)
}

// pushBranchWithRetry runs `git push -u origin <branch>` in dir and returns the last error.
// It retries up to pushMaxAttempts in total, with doubling backoff, only while the push is a transient refusal of branch;
// any other failure returns on the first attempt, unwrapped.
func (s pushSeam) pushBranchWithRetry(dir, branch string) error {
	delay := pushBaseBackoff
	for attempt := 1; ; attempt++ {
		_, err := s.runPush([]string{"push", "-u", "origin", branch}, dir)
		if err == nil {
			return nil
		}
		line, transient := transientPushRefusal(err, branch)
		if !transient || attempt == pushMaxAttempts {
			return err
		}
		logger.Warn("fabricengine: branch push refused transiently, retrying", "branch", branch, "attempt", attempt, "stderr", line)
		s.sleepFor(delay)
		delay *= 2
	}
}

// transientPushRefusal reports whether err is a push whose stderr holds a "! [remote rejected] <ref> -> <ref> (failed)" line for branch, and returns that line.
// Only the reason on the pushed ref's own line decides; every other stderr line is ignored.
func transientPushRefusal(err error, branch string) (string, bool) {
	var gitErr *gitexec.GitError
	if !errors.As(err, &gitErr) {
		return "", false
	}
	ref := "refs/heads/" + branch
	for _, line := range strings.Split(gitErr.Stderr, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "! [remote rejected]") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, "! [remote rejected]"))
		if len(fields) != 4 || fields[1] != "->" {
			continue
		}
		if (fields[0] == branch || fields[0] == ref) && (fields[2] == branch || fields[2] == ref) && fields[3] == "(failed)" {
			return line, true
		}
	}
	return "", false
}
