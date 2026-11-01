//go:build tier0

// seam_enforcement_test.go enforces this package's Told-Geometry Invariant membership: production
// code in internal/loomshed takes every absolute path it operates on from its caller and has no
// direct production import of internal/lymcwd. Named seam_enforcement_test.go rather than
// leaf_enforcement_test.go on purpose -- that second filename pairs with a TestLeafInvariant_
// AllowlistOnly name on genuine zero-or-one-dependency leaves, and loomshed imports seven internal
// packages, so it is structurally a seam, not a leaf, exactly like internal/shedengine's own
// seam_enforcement_test.go this file is modelled on.
//
// The allowlist below is deliberately a membership list rather than a bare internal/lymcwd denylist:
// it catches the excluded import and anything else that would drag geometry resolution in, with no
// list maintenance beyond a genuine new dependency. internal/loomengine appearing on the allowlist
// is legal despite loomengine itself importing internal/lymcwd, because the invariant's membership
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
	"github.com/Knatte18/loomyard/internal/gateslot",
	"github.com/Knatte18/loomyard/internal/shedadapters",
	"github.com/Knatte18/loomyard/internal/websterengine",
	"github.com/Knatte18/loomyard/internal/loomengine",
	"github.com/Knatte18/loomyard/internal/planparser",
	// internal/planindex is the cgo-free seam the plan gates resolve through.
	// The resolve-backed implementation, internal/planglyph, links tree-sitter and stays out of this package for good.
	"github.com/Knatte18/loomyard/internal/planindex",
	"github.com/Knatte18/loomyard/internal/discussionparser",
	// internal/impactset parses the `//lym:seam` markers of the package directories it is told and resolves no geometry.
	"github.com/Knatte18/loomyard/internal/impactset",
	"github.com/Knatte18/loomyard/internal/batcher",
	// internal/configengine carries the told Dirs the batcher-backed rows load through.
	// It is handed the dirs and resolves no geometry.
	"github.com/Knatte18/loomyard/internal/configengine",
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
	// internal/commentlint is the round gate's comment lint.
	// It is told the worktree and resolves no geometry.
	"github.com/Knatte18/loomyard/internal/commentlint",
	// internal/patternlint and internal/pattern are the round gate's headroom lint and the finding type it returns.
	// Both are told the worktree and resolve no geometry.
	"github.com/Knatte18/loomyard/internal/patternlint",
	"github.com/Knatte18/loomyard/internal/pattern",
	// internal/gitrepo is the go-git read that checks a Publish failure record's merge commit and a recorded pass's commit.
	// It is told the worktree and resolves no geometry.
	"github.com/Knatte18/loomyard/internal/gitrepo",
	// internal/shedverbs is imported for the two interrupt-policy words and no behaviour.
	// It imports no lymcwd, so the told-geometry membership holds.
	"github.com/Knatte18/loomyard/internal/shedverbs",
	// github.com/Knatte18/quarry/glyph is the cgo-free glyph grammar internal/planparser already imports.
	"github.com/Knatte18/quarry/glyph",
	// internal/lymdirs names the two root directory names and resolves no geometry.
	"github.com/Knatte18/loomyard/internal/lymdirs",
	// internal/fsx writes the Record-Write row's files atomically.
	// It is told absolute paths and resolves no geometry.
	"github.com/Knatte18/loomyard/internal/fsx",
	// internal/samplerengine holds the Sampler-Session row's record and reads and writes it at the paths it is told.
	// It resolves no geometry.
	"github.com/Knatte18/loomyard/internal/samplerengine",
	// internal/pawlengine supplies the Beam-Await row-name constant for the operator-wait list.
	// It resolves no geometry here.
	"github.com/Knatte18/loomyard/internal/pawlengine",
	// internal/shedtransient classifies a session start failure as transient or not.
	// It resolves no geometry.
	"github.com/Knatte18/loomyard/internal/shedtransient",
	// gopkg.in/yaml.v3 decodes and encodes the amendment record, which is YAML.
	// The package still resolves no geometry: the record's path is told.
	"gopkg.in/yaml.v3",
}

func TestToldGeometryInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlistNoStale(t, "internal/loomshed", loomshedAllowedImports...)
}
