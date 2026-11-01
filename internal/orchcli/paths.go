// paths.go declares the orch module's one scratch accessor: every file orch writes lives under the prime's `<anchor>/.lym/orch/`, joined once in internal/orchroute and handed to orchengine as told paths (Durable-vs-Ephemeral State, Lymdirs Single-Declarer and Told-Geometry Invariants).
// PrimeRoot is the worktree root a relative note path resolves against.

package orchcli

import (
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
func orchPaths(location *lymcwd.Location) orchengine.Paths {
	return orchroute.Paths(location)
}
