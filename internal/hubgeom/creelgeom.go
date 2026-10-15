// creelgeom.go builds the geometry creel's daemon and clients are told for one hub.

package hubgeom

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// CreelGeometry builds the creelengine.Geometry of the hub at hubPath, with the socket under tempRoot.
// It takes the hub path rather than a location, since the daemon runs from the hub directory and that is no worktree.
// It takes the temp root path rather than opening the root, so its caller owns the lyxtemp root.
// A hub with no recorded shortname is an error naming the shortname record and the hub.
func CreelGeometry(hubPath, tempRoot string) (creelengine.Geometry, error) {
	boardDir := fabricengine.BoardDir(hubPath)
	shortname, found := fabricengine.ReadShortname(boardDir)
	if !found {
		return creelengine.Geometry{}, fmt.Errorf("hub %q has no usable shortname record %s", hubPath, filepath.Join(boardDir, fabricengine.ShortnameFileName))
	}
	stateDir := filepath.Join(fabricengine.HubScratchDir(hubPath), lyxdirs.CreelDirName)
	return creelengine.Geometry{
		HubPath:    hubPath,
		Shortname:  shortname,
		StateDir:   stateDir,
		JournalDir: filepath.Join(stateDir, "journal"),
		LockPath:   filepath.Join(stateDir, "daemon.lock"),
		SocketPath: filepath.Join(tempRoot, creelengine.SocketName(shortname, hubPath)),
	}, nil
}
