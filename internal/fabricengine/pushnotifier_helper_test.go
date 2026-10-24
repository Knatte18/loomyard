// pushnotifier_helper_test.go holds fakePushNotifier and installFakePushNotifier, the failed-push notifier fake the tier0 and tier1 push tests share.
// It carries no build tag, so it compiles into every tag set.

package fabricengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// fakePushNotifier counts the calls carrying its own hub path, so a parallel test recording a failure in another hub never reaches its counts.
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

func installFakePushNotifier(t *testing.T, n *fakePushNotifier) {
	t.Helper()
	t.Cleanup(fabricengine.InstallPushNotifier(n.notify))
}
