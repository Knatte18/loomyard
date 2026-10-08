// spawn.go — the "lyx fabric sync" verb's async-push call site.
// spawnPush delegates to fabricengine.SpawnDetachedPush with the worktree paths it is given,
// so the detached child pushes whichever sides are supplied: the code side is empty in the prime.
// The detach/process-group mechanics themselves live in the engine helper — see internal/fabricengine/spawn.go.

package fabriccli

import "github.com/Knatte18/loomyard/internal/fabricengine"

// spawnPush launches a detached push of warpPath and weftPath via fabricengine.SpawnDetachedPush, each side only when its path is non-empty.
func spawnPush(warpPath, weftPath string) error {
	return fabricengine.SpawnDetachedPush(warpPath, weftPath)
}
