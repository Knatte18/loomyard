// spawn.go — the async-push call sites of "lym fabric sync" and "lym fabric add".
// spawnPush delegates to fabricengine.SpawnStatusPush with the location it is given,
// so the detached child pushes the sides the engine's prime rule and local-branch check leave in the request.
// spawnPairPush is add's push of a new pair, replaceable by tests, since a detached push forks the running binary.
// The detach/process-group mechanics themselves live in the engine helper — see internal/fabricengine/spawn.go.

package fabriccli

import (
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// spawnPush launches the detached status push of the worktree at l via fabricengine.SpawnStatusPush.
func spawnPush(l *lymcwd.Location) (fabricengine.SpawnResult, error) {
	return fabricengine.SpawnStatusPush(l, "")
}

// spawnPairPush launches the detached push of the new pair at the given location: its code and records worktrees.
var spawnPairPush = fabricengine.SpawnPairPush
