// pushretry.go — the bounded retry the plain, never-forced push primitives wrap around a push.
// It repeats a push only when the server refuses the pushed ref with a bare "(failed)" or a remote "cannot lock ref".

package gitrepo

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/logger"
)

// ErrPushRetriesExhausted is matched with errors.Is against the error a push returns after every attempt was refused with a retryable remote refusal.
// The error also wraps the last push error, and never ErrPushRejected.
var ErrPushRetriesExhausted = errors.New("gitrepo: push retries exhausted")

// pushMaxAttempts is the total number of pushes a retryably refused push gets.
const pushMaxAttempts = 4

// pushBaseBackoff is the delay before the second attempt; each later delay doubles it.
const pushBaseBackoff = time.Second

const remoteRejectedPrefix = "! [remote rejected]"

// retryablePushRefusal reports whether err is a push whose stderr holds a "! [remote rejected] <src> -> <dst> (<reason>)" line for ref, and returns that line.
// The line belongs to ref when src and dst both equal ref in its short form or its full refs/heads/ or refs/tags/ form, and it is retryable when the reason is exactly "(failed)" or contains "cannot lock ref".
// Every other stderr line is ignored, and an empty ref matches no line.
func retryablePushRefusal(err error, ref string) (line string, retryable bool) {
	var gitErr *gitexec.GitError
	if ref == "" || !errors.As(err, &gitErr) {
		return "", false
	}
	for _, candidate := range strings.Split(gitErr.Stderr, "\n") {
		candidate = strings.TrimSpace(candidate)
		rest, found := strings.CutPrefix(candidate, remoteRejectedPrefix)
		if !found {
			continue
		}
		source, afterArrow, found := strings.Cut(rest, " -> ")
		if !found {
			continue
		}
		destination, reason, found := strings.Cut(afterArrow, " (")
		if !found {
			continue
		}
		reason = "(" + reason
		if !namesRef(strings.TrimSpace(source), ref) || !namesRef(strings.TrimSpace(destination), ref) {
			continue
		}
		if reason == "(failed)" || strings.Contains(reason, "cannot lock ref") {
			return candidate, true
		}
	}
	return "", false
}

// namesRef reports whether name is ref in its short form or its full refs/heads/ or refs/tags/ form.
func namesRef(name, ref string) bool {
	return name == ref || name == "refs/heads/"+ref || name == "refs/tags/"+ref
}

// PushWithRetry calls push up to pushMaxAttempts times in total, sleeping a doubling backoff between attempts, only while push fails with a retryable remote refusal of ref.
// A non-retryable error returns unchanged on its attempt; an exhausted retry returns an error wrapping ErrPushRetriesExhausted and the last push error.
// A nil sleep sleeps the delay plus up to half of it as jitter.
//
// On the weft the refusal `! [remote rejected] main -> main (failed)` is a ref-update race:
// the remote refuses a ref update that raced another update of the weft's `main` (board writes and other runs' record pushes), and the local ref is not stale.
// The retry is the intended path for it, and the Warn each retry logs is the signal that surfaced the race.
func PushWithRetry(push func() error, ref string, sleep func(time.Duration)) error {
	if sleep == nil {
		sleep = func(delay time.Duration) { time.Sleep(delay + rand.N(delay/2+1)) }
	}
	delay := pushBaseBackoff
	for attempt := 1; ; attempt++ {
		err := push()
		if err == nil {
			return nil
		}
		line, retryable := retryablePushRefusal(err, ref)
		if !retryable {
			return err
		}
		if attempt == pushMaxAttempts {
			return fmt.Errorf("%w after %d attempts: %w", ErrPushRetriesExhausted, attempt, err)
		}
		logger.Warn("gitrepo: push refused, retrying", "ref", ref, "attempt", attempt, "stderr", line)
		sleep(delay)
		delay *= 2
	}
}
