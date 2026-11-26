// pendingteardown_test.go covers the list of pending-teardown records against a hub directory with no git.

package fabricengine

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestPendingTeardowns covers the list of remote teardowns whose child ended before its job did.
// A record whose teardown lock no process holds is listed with the re-run that finishes it.
// A removal's record that carries its caller's command names that command, not `lym fabric remove`, as the re-run.
// A record that carries the task tip a discard tagged names the hand push of that tip's archive tag first.
// A record whose lock a process holds exclusive is a teardown still running and is not listed.
func TestPendingTeardowns(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("AHubWithNoRecordListsNone", func(t *testing.T) {
		t.Parallel()
		got, err := PendingTeardowns(t.TempDir())
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("PendingTeardowns() = (%v, %v); want an empty, non-nil list", got, err)
		}
	})

	t.Run("ARecordWithNoLockHolderIsListedWithItsRerun", func(t *testing.T) {
		t.Parallel()
		hub := t.TempDir()
		handedAt := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
		taskTip := strings.Repeat("b", 40)
		taskTag := ArchiveTagName("tipped", taskTip)
		for _, job := range []remoteTeardownJob{
			{Kind: remoteTeardownRemove, Slug: "ended"},
			{Kind: remoteTeardownRemove, Slug: "cancelled", Rerun: "lym batten cancel cancelled --remove"},
			{Kind: remoteTeardownDiscard, Slug: "discarded"},
			{Kind: remoteTeardownDiscard, Slug: "tipped", Worktree: "/code", TaskTip: taskTip},
			{Kind: remoteTeardownRemove, Slug: "running"},
		} {
			if err := writePendingTeardown(hub, job, handedAt); err != nil {
				t.Fatalf("writePendingTeardown(%s) error = %v; want nil", job.Slug, err)
			}
		}
		held, acquired, err := lock.TryAcquireWriteLock(TeardownLockPath(hub, "running"))
		if err != nil || !acquired {
			t.Fatalf("TryAcquireWriteLock() = (%v, %v); want (true, nil)", acquired, err)
		}
		t.Cleanup(func() { _ = held.Release() })

		got, err := PendingTeardowns(hub)

		if err != nil {
			t.Fatalf("PendingTeardowns() error = %v; want nil", err)
		}
		want := []PendingTeardown{
			{Slug: "cancelled", Kind: remoteTeardownRemove, HandedAt: handedAt, WayForward: "run `lym batten cancel cancelled --remove` again"},
			{Slug: "discarded", Kind: remoteTeardownDiscard, HandedAt: handedAt, WayForward: "run the command that discarded the task branch again"},
			{Slug: "ended", Kind: remoteTeardownRemove, HandedAt: handedAt, WayForward: "run `lym fabric remove ended --remote` again"},
			{Slug: "tipped", Kind: remoteTeardownDiscard, HandedAt: handedAt, WayForward: "push the archive tag with `git -C /code push origin refs/tags/" + taskTag + ":refs/tags/" + taskTag +
				"`, then run the command that discarded the task branch again"},
		}
		same := func(a, b PendingTeardown) bool {
			return a.Slug == b.Slug && a.Kind == b.Kind && a.HandedAt.Equal(b.HandedAt) && a.WayForward == b.WayForward
		}
		if !slices.EqualFunc(got, want, same) {
			t.Errorf("PendingTeardowns() = %+v; want %+v", got, want)
		}
	})
}
