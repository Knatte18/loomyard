// paths.go derives the orch paths for a prime: every file orch writes lives under the prime's `<anchor>/.lym/orch/`.
// internal/orchroute joins that path once, and orchengine receives the joined paths as told paths.

package orchcli

import (
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/orchroute"
)

// orchDirName is the module's own subdirectory name, beneath the anchor's .lym directory.
const orchDirName = orchroute.DirName

// PrimePaths returns the orch paths for the prime at location, so another module's CLI wiring reaches the orch state without re-deriving it.
func PrimePaths(location *lymcwd.Location) orchengine.Paths {
	return orchPaths(location)
}

// orchPaths returns every path orch operates on, rooted at location's anchor.
// It also sets the machine's presence record, which the rendered pointers read.
// A hub whose Slack geometry cannot be built leaves the presence path empty, which reads as no presence line.
func orchPaths(location *lymcwd.Location) orchengine.Paths {
	paths := orchroute.Paths(location)
	if geometry, err := hubgeom.SlackGeometry(location.HubPath); err == nil {
		paths.SlackPresencePath = geometry.PresencePath
	}
	return paths
}
