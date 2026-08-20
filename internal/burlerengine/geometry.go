// geometry.go declares Geometry, the two-field struct burler is told its coordinates through.
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
	WorktreeRoot string
	// AnchorPath is the base the per-round .lyx/burler instruction directory joins onto.
	AnchorPath string
}
