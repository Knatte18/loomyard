// creelgeom.go builds the geometry creel's daemon and clients are told for one hub.

package hubgeom

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// CreelGeometry builds the creelengine.Geometry of the hub at hubPath, with the socket under tempRoot.
// It takes the hub path, since the daemon runs from the hub directory, which is no worktree.
// It takes the temp root path without opening it, so its caller owns the lymtemp root.
// A hub with no recorded shortname is an error naming the shortname record and the hub.
func CreelGeometry(hubPath, tempRoot string) (creelengine.Geometry, error) {
	boltDir := fabricengine.BoltDir(hubPath)
	shortname, found := fabricengine.ReadShortname(boltDir)
	if !found {
		return creelengine.Geometry{}, fmt.Errorf("hub %q has no usable shortname record %s", hubPath, filepath.Join(boltDir, fabricengine.ShortnameFileName))
	}
	stateDir := filepath.Join(fabricengine.HubScratchDir(hubPath), lymdirs.CreelDirName)
	return creelengine.Geometry{
		HubPath:       hubPath,
		Shortname:     shortname,
		StateDir:      stateDir,
		JournalDir:    filepath.Join(stateDir, "journal"),
		LockPath:      filepath.Join(stateDir, "daemon.lock"),
		SocketPath:    filepath.Join(tempRoot, creelengine.SocketName(shortname, hubPath)),
		DaemonLogPath: filepath.Join(stateDir, "daemon.log"),
		FlagDir:       filepath.Join(stateDir, lymdirs.CreelFlagDirName),
	}, nil
}
