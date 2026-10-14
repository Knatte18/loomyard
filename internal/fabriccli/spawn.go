// spawn.go — the async-push call sites of "lyx fabric sync" and "lyx fabric add".
// spawnPush delegates to fabricengine.SpawnDetachedPush with the worktree paths it is given,
// so the detached child pushes whichever sides are supplied: the code side is empty in the prime.
// spawnPairPush is add's push of a new pair, replaceable by tests, since a detached push forks the running binary.
// The detach/process-group mechanics themselves live in the engine helper — see internal/fabricengine/spawn.go.

package fabriccli

import "github.com/Knatte18/loomyard/internal/fabricengine"

// spawnPush launches a detached push of warpPath and weftPath via fabricengine.SpawnDetachedPush, each side only when its path is non-empty.
func spawnPush(warpPath, weftPath string) error {
	return fabricengine.SpawnDetachedPush(warpPath, weftPath)
}

// spawnPairPush launches the detached push of the new pair at the given location: its code and records worktrees.
var spawnPairPush = fabricengine.SpawnPairPush
