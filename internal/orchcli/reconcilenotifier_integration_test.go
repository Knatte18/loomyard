// reconcilenotifier_integration_test.go proves the reconcile notice route over a real hub, with creel's daemon run in process on a fake delivery.

package orchcli

import (
	"context"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestReconcileNotifierFor covers the notifier an install from the hub's board worktree stores: a notice for that hub's Bolt checkout and one for another.
//
// It runs without t.Parallel: the strand environment variable is process-global.
func TestReconcileNotifierFor(t *testing.T) {
	tiergate.Require(t, 1)

	const line = "[reconcile notice] lym build x changed what a value in the hub's config means: drift.yaml capture.dir"
	tests := []struct {
		name string
		// noticeBoardDir returns the Bolt checkout the notice is for.
		noticeBoardDir func(t *testing.T, hubPath string) string
		wantPosted     bool
	}{
		{"ThisHubsNoticeMailsThePrimeOrchAsReconcile", func(_ *testing.T, hubPath string) string { return fabricengine.BoltDir(hubPath) }, true},
		{"AnotherHubsNoticeIsNotPostedAndMailsNothing", func(t *testing.T, _ string) string { return t.TempDir() }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(agentname.StrandNameEnv, "")
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			receiver := startHubOrchDaemon(t, h)
			location, inHub := hubLocation(lymcwd.WithCwd(context.Background(), fabricengine.BoardDir(h.Path)))
			if !inHub {
				t.Fatalf("hubLocation(board worktree) found no hub")
			}

			posted, err := reconcileNotifierFor(location)(tt.noticeBoardDir(t, h.Path), line)
			if err != nil || posted != tt.wantPosted {
				t.Errorf("notifier() = %v, %v; want posted %v and no error", posted, err, tt.wantPosted)
			}

			mails := postedMails(t, h)
			if !tt.wantPosted {
				if len(mails) != 0 {
					t.Errorf("posted events = %+v; want none for another hub's notice", mails)
				}
				return
			}
			if len(mails) != 1 {
				t.Fatalf("posted events = %+v; want exactly one", mails)
			}
			if mails[0].To != receiver || !strings.HasSuffix(mails[0].From, ":RECONCILE") || mails[0].Subject != line {
				t.Errorf("posted mail = from %q to %q subject %q; want from <shortname>:RECONCILE to %q subject %q", mails[0].From, mails[0].To, mails[0].Subject, receiver, line)
			}
		})
	}
}
