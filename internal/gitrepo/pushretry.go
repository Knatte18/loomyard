// pushretry.go — the bounded retry the plain, never-forced push primitives wrap around a push.
// It repeats a push only when the server refuses the pushed ref with a bare "(failed)" or a remote "cannot lock ref".
// A caller may name further retryable reasons.

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

// pushMaxAttempts is the total number of pushes the plain push primitives give a retryably refused push.
const pushMaxAttempts = 4

// pushBaseBackoff is the plain push primitives' delay before the second attempt; each later delay doubles it.
const pushBaseBackoff = time.Second

// PushBackoffDelays returns the delays the plain push primitives sleep between their attempts, in order:
// pushBaseBackoff, then each doubled, one fewer than pushMaxAttempts.
// A caller that retries a push on the same schedule passes or sleeps these delays.
func PushBackoffDelays() []time.Duration {
	delays := make([]time.Duration, 0, pushMaxAttempts-1)
	delay := pushBaseBackoff
	for range pushMaxAttempts - 1 {
		delays = append(delays, delay)
		delay *= 2
	}
	return delays
}

const remoteRejectedPrefix = "! [remote rejected]"

// retryablePushRefusal finds the "! [remote rejected] <src> -> <dst> (<reason>)" line in err's stderr that refuses ref.
// It returns that line and reports whether the refusal is retryable.
// The line belongs to ref when source and destination both name it, as a short or full refs/heads/ or refs/tags/ name.
// The refusal is retryable when its reason is exactly "(failed)" or contains "cannot lock ref" or one of extraReasons.
// Every other stderr line is ignored, and an empty ref matches no line.
func retryablePushRefusal(err error, ref string, extraReasons []string) (line string, retryable bool) {
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
		if reason == "(failed)" || strings.Contains(reason, "cannot lock ref") || containsAny(reason, extraReasons) {
			return candidate, true
		}
	}
	return "", false
}

// namesRef reports whether name is ref in its short form or its full refs/heads/ or refs/tags/ form.
// A ref given in full form also matches its short form, which is how git prints a pushed ref.
func namesRef(name, ref string) bool {
	for _, namespace := range []string{"refs/heads/", "refs/tags/"} {
		if short, found := strings.CutPrefix(ref, namespace); found {
			return name == ref || name == short
		}
	}
	return name == ref || name == "refs/heads/"+ref || name == "refs/tags/"+ref
}

// PushWithRetry calls push once, then once more after each delay in delays.
// It sleeps each delay between attempts, and only while push fails with a retryable remote refusal of ref.
// A non-retryable error returns unchanged on its attempt;
// an exhausted retry returns an error wrapping ErrPushRetriesExhausted and the last push error.
// A nil sleep sleeps the delay plus up to half of it as jitter.
// Only a bare "(failed)" and a "cannot lock ref" reason are retryable.
//
// On the records repository the refusal `! [remote rejected] main -> main (failed)` is a ref-update race:
// the remote refuses a ref update that raced another writer of that repository's `main`,
// and the local ref is not stale.
// The retry is the intended path for it,
// and the Warn each retry logs is the signal that surfaced the race.
func PushWithRetry(push func() error, ref string, delays []time.Duration, sleep func(time.Duration)) error {
	return PushWithRetryReasons(push, ref, delays, sleep, nil)
}

// PushWithRetryReasons calls push and retries a remote refusal of ref whose reason contains one of extraReasons.
// A bare "(failed)" and a "cannot lock ref" refusal are retried whatever extraReasons holds.
// The check still reads only the "remote rejected" line naming ref,
// and the number of attempts stays one more than delays holds,
// so a refusal that never clears still ends in an error wrapping ErrPushRetriesExhausted.
func PushWithRetryReasons(push func() error, ref string, delays []time.Duration, sleep func(time.Duration), extraReasons []string) error {
	if sleep == nil {
		sleep = func(delay time.Duration) { time.Sleep(delay + rand.N(delay/2+1)) }
	}
	for attempt := 1; ; attempt++ {
		err := push()
		if err == nil {
			return nil
		}
		line, retryable := retryablePushRefusal(err, ref, extraReasons)
		if !retryable {
			return err
		}
		if attempt > len(delays) {
			return fmt.Errorf("%w after %d attempts: %w", ErrPushRetriesExhausted, attempt, err)
		}
		logger.Warn("gitrepo: push refused, retrying", "ref", ref, "attempt", attempt, "stderr", line)
		sleep(delays[attempt-1])
	}
}
