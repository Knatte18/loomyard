// spawn.go holds the pusher-start call sites of `lym fabric sync` and `lym fabric add`.
// Both start the background pusher of the worktree they are given, which brings its commits to origin.

package fabriccli

import (
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// spawnPush starts the pusher of the worktree at l.
func spawnPush(l *lymcwd.Location) (fabriclayout.SpawnResult, error) {
	return fabricengine.SpawnStatusPush(l, "")
}

// spawnPairPush starts the pusher of the new pair at the given location.
// It is a variable so tests can replace it, since a detached push forks the running binary.
var spawnPairPush = fabricengine.SpawnPairPush
