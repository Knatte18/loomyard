// spawn.go — the "lyx fabric sync" verb's async-push call site.
// spawnPush delegates to fabricengine.SpawnDetachedPush, weft-only (an empty warpPath), preserving
// the existing "lyx fabric sync" verb's behavior at its unchanged call site in weft_verbs.go.
// The detach/process-group mechanics themselves now live in the engine helper — see
// internal/fabricengine/spawn.go.

package fabriccli

import "github.com/Knatte18/loomyard/internal/fabricengine"

// spawnPush launches a detached, weft-only push of weftPath via
// fabricengine.SpawnDetachedPush.
func spawnPush(weftPath string) error {
	return fabricengine.SpawnDetachedPush("", weftPath)
}
