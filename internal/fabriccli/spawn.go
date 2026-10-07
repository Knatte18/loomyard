// spawn.go — the "lyx fabric sync" verb's async-push call site.
// spawnPush delegates to fabricengine.SpawnDetachedPush with both worktree paths, so the detached child pushes the warp side and the weft side.
// The detach/process-group mechanics themselves live in the engine helper — see internal/fabricengine/spawn.go.

package fabriccli

import "github.com/Knatte18/loomyard/internal/fabricengine"

// spawnPush launches a detached push of both warpPath and weftPath via fabricengine.SpawnDetachedPush.
func spawnPush(warpPath, weftPath string) error {
	return fabricengine.SpawnDetachedPush(warpPath, weftPath)
}
