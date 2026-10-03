// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership: production
// code in internal/loomshed takes every absolute path it operates on from its caller and has no
// direct production import of internal/lyxcwd. Named seam_enforcement_test.go rather than
// leaf_enforcement_test.go on purpose -- that second filename pairs with a TestLeafInvariant_
// AllowlistOnly name on genuine zero-or-one-dependency leaves, and loomshed imports seven internal
// packages, so it is structurally a seam, not a leaf, exactly like internal/shedengine's own
// seam_enforcement_test.go this file is modelled on.
//
// The allowlist below is deliberately a membership list rather than a bare internal/lyxcwd denylist:
// it catches the excluded import and anything else that would drag geometry resolution in, with no
// list maintenance beyond a genuine new dependency. internal/loomengine appearing on the allowlist
// is legal despite loomengine itself importing internal/lyxcwd, because the invariant's membership
// predicate is about a direct production import and transitive is explicitly fine.

package loomshed

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// loomshedAllowedImports are the only non-stdlib import paths production code in this package may
// use.
var loomshedAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/shedengine",
	"github.com/Knatte18/loomyard/internal/shedadapters",
	"github.com/Knatte18/loomyard/internal/websterengine",
	"github.com/Knatte18/loomyard/internal/loomengine",
	"github.com/Knatte18/loomyard/internal/planparser",
	// internal/planglyph is a genuine new dependency, not a loosened rule: planvalidate.go's producer
	// now runs the gate through planglyph's resolve-backed entry points, and planglyph itself derives
	// no path of its own and never imports internal/lyxcwd, per the told-geometry-for-planglyph
	// Shared Decision -- so its transitive geometry footprint is exactly zero, same as
	// internal/planparser's own membership above.
	"github.com/Knatte18/loomyard/internal/planglyph",
	"github.com/Knatte18/loomyard/internal/discussionparser",
	"github.com/Knatte18/loomyard/internal/batcher",
	"github.com/Knatte18/loomyard/internal/state",
	// internal/logger is a genuine new dependency rather than a loosened rule: the gate producers
	// here surface the determined findings they used to discard, and it resolves no geometry of any
	// kind, so it cannot drag cwd resolution in. internal/shedadapters, already on this list,
	// imports it too.
	"github.com/Knatte18/loomyard/internal/logger",
	// internal/shuttleengine is imported for two type names (Gate, GateResult) and no behaviour --
	// gates.go's two closures build and return shuttleengine.Gate values but never drive a run
	// through the package. internal/shedadapters, already on this list, imports it transitively
	// anyway, so this does not widen the package's own geometry footprint.
	"github.com/Knatte18/loomyard/internal/shuttleengine",
	// internal/verifytree is the shared plan-verify function.
	// It is told its worktree and verify directory and resolves no geometry.
	"github.com/Knatte18/loomyard/internal/verifytree",
}

func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlistNoStale(t, "internal/loomshed", loomshedAllowedImports...)
}
