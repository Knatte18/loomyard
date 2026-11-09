// pushnotifier_helper_test.go holds the failed-push notifier fake that the tier0 and tier1 push tests share.
// It carries no build tag, so it compiles into every tag set.

package fabricengine_test

import (
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// fakePushNotifier counts the calls carrying its own hub path and answers any other hub as posted.
type fakePushNotifier struct {
	hub    string
	posted bool
	err    error
	lines  []string
}

func (n *fakePushNotifier) notify(hubPath, line string) (bool, error) {
	if hubPath != n.hub {
		return true, nil
	}
	n.lines = append(n.lines, line)
	return n.posted, n.err
}

// options returns push options that mail a failed push through n and sleep each backoff delay through backoff.
// A nil backoff sleeps for real.
func (n *fakePushNotifier) options(backoff func(time.Duration)) fabricengine.SyncOptions {
	return fabricengine.SyncOptions{Notifier: n.notify, BackoffSleep: backoff}
}
