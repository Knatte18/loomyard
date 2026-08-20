// webstergeom.go implements WebsterGeometry, the hub-mode teller that converts a resolved
// *lyxcwd.Location into a websterengine.Geometry.

package hubgeom

import (
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// WebsterGeometry builds a websterengine.Geometry for l: the resolved Location's paths, read off
// its accessors and passed through untouched.
// It performs no cwd resolution of its own — internal/lyxcwd stays the sole owner of cwd resolution
// (the Cwd Resolution Invariant), and WebsterGeometry only reads what l's caller already resolved.
// WorktreeRoot is l.AnchorPath(), NOT l.WorktreePath(): every one of webster's CLI call sites passes
// the anchor path today, and this is the opposite of the neighbouring ReedGeometry's own
// WorktreeRoot, which is l.WorktreePath(). Converging the two would silently change behaviour in a
// subpath-anchored hub, where the anchor path and the worktree path diverge.
func WebsterGeometry(l *lyxcwd.Location) websterengine.Geometry {
	anchorPath := l.AnchorPath()
	return websterengine.Geometry{
		AnchorRoot:   anchorPath,
		WorktreeRoot: anchorPath,
		WebsterDir:   websterengine.Dir(anchorPath),
		ReportsDir:   websterengine.ReportsDir(anchorPath),
		ScratchDir:   websterengine.ScratchDir(anchorPath),
		PromptsDir:   websterengine.PromptsDir(anchorPath),
		StencilsDir:  fabricengine.StencilsDir(l.HubPath),
		PlanDir:      planparser.PlanDir(anchorPath),
	}
}
