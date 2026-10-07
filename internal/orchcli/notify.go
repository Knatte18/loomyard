// notify.go declares NotifyPrime, the one way a task-worktree module queues a notice for the hub's prime orch.

package orchcli

import (
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/orchengine"
)

// NotifyPrime queues line as an orch notice for the prime of the hub that location's worktree belongs to, resolving the prime at call time.
// A prime with no orch strand recorded is not an error: the notice is logged and not queued.
func NotifyPrime(location *lyxcwd.Location, line string) error {
	primeName, err := fabricengine.PrimeName(location)
	if err != nil {
		return err
	}
	prime, err := lyxcwd.ResolveWorktree(fabricengine.WorktreePath(location, primeName))
	if err != nil {
		return err
	}
	_, err = orchengine.QueueNotice(PrimePaths(prime), line, time.Now())
	return err
}
