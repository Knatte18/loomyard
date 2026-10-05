// geometry.go declares Geometry, the struct burler is told its coordinates through.
// It declares the type only — this file adds no constructor, no validator, and no default.

package burlerengine

// Geometry is the set of paths burler is told, once, at construction, and never derives itself.
// burlerengine.New validates no field of a Geometry — populating every field with a usable absolute
// path is entirely the caller's obligation.
// hubgeom.BurlerGeometry is the hub-mode answer that builds a Geometry from a resolved
// *lyxcwd.Location, but is deliberately not imported here — this file states the contract, not the
// implementation.
type Geometry struct {
	// WorktreeRoot is the root Engine.Run resolves a Profile's relative paths against, via
	// (*Profile).validate.
	// What fills it differs by mode: hub mode now tells it the anchor path
	// (hubgeom.BurlerGeometry), while standalone tells it the reviewed target directory
	// (standalonegeom.BurlerGeometry) — this is why the field is not collapsible into AnchorPath
	// and is not renamed.
	WorktreeRoot string
	// AnchorPath is the base the per-round .lyx/burler instruction directory joins onto.
	AnchorPath string
	// RepoRoot is the directory holding PATTERN.md and go.mod, the repository's worktree root.
	// It is told separately because WorktreeRoot is the anchor path in hub mode,
	// which is not the repo root for a subpath-anchored hub.
	// An empty RepoRoot yields no PATTERN directive.
	RepoRoot string
	// ParentName is the told parent agent name the round prompt renders its parent directive from.
	// An empty name renders the directive's no-parent variant.
	ParentName string
}
