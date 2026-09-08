// geometry.go declares Geometry, the eight-field struct webster is told its coordinates through.
// It declares the type only — no constructor, no validator, and no default; populating every field
// with a usable absolute path is entirely the caller's obligation, exactly as
// internal/reedengine/geometry.go does for reed.
// hubgeom.WebsterGeometry is the hub-mode teller that builds a Geometry from a resolved
// *lyxcwd.Location; internal/standalonegeom is the told-mode sibling that builds one from a
// standalone state tree. The dependency direction is one-way: those packages import websterengine,
// never the reverse.

package websterengine

// Geometry is the set of paths webster is told, once, at construction, and never derives itself.
// No method here validates or recomputes any field — populating every field with a usable absolute
// path is entirely the caller's obligation.
type Geometry struct {
	// AnchorRoot is the base every _lyx/.lyx join and every module config read hangs off.
	AnchorRoot string
	// WorktreeRoot is the repo checkout the head-SHA capture and the dirty-worktree check read.
	// It is also the fork-audit workdir, which must equal the pane's actual cwd
	// (reedengine.Geometry.PaneCwd), because the audit resolves transcript-relative write paths
	// against it.
	// It is NOT the same notion as reedengine.Geometry.WorktreeRoot in HUB mode specifically:
	// hubgeom.WebsterGeometry sets this to the anchor-anchored value (l.AnchorPath(), never
	// l.WorktreePath()), while reed's is the worktree path, so the two coincide only at an
	// unanchored anchor there. Standalone mode does NOT share that collision:
	// standalonegeom.WebsterGeometry sets this to the standalone TARGET, deliberately disjoint
	// from AnchorRoot (the derived state directory) — see that constructor's own doc comment. A
	// caller must not assume WorktreeRoot == AnchorRoot holds across both modes; only hub mode's
	// own construction makes it so today, and that collision is deliberate continuity, not to be
	// "fixed" by converging either field on the other.
	WorktreeRoot string
	// WebsterDir is the told path to webster's durable run state directory (state.json,
	// outcome.yaml).
	WebsterDir string
	// ReportsDir is the told path to the directory holding webster's per-batch report files.
	ReportsDir string
	// PromptsDir is the told path to the directory holding webster's rendered fork prompts.
	PromptsDir string
	// ScratchDir is the told path to the base directory for webster's never-tracked artifacts (the
	// pause flag, rendered fork prompts, and every *.lock).
	ScratchDir string
	// StencilsDir is the told absolute directory the prompt stencils are read from at call time.
	StencilsDir string
	// PlanDir is the told directory planparser parses.
	PlanDir string
}
