// geometry.go declares Geometry, the two-field struct perch is told its coordinates through.
// It declares the type only — this file adds no constructor, no validator, and no default.

package perchengine

// Geometry is the set of paths perch is told, once, at construction, and never derives itself.
// perchengine.New validates no field of a Geometry.
// hubgeom.PerchGeometry is the hub-mode answer that builds a Geometry from a resolved
// *lyxcwd.Location, but is deliberately not imported here.
type Geometry struct {
	// GateDir is the gate command's working directory, the value Engine.Run stamps onto
	// treadleengine.Profile.GateDir.
	// The field is named GateDir rather than WorktreeRoot because perch's only use of that root is
	// treadleengine.Profile.GateDir.
	GateDir string
	// AnchorPath is the base RunsDir and ScratchDir join onto, carried alongside GateDir so a perch
	// call site's two roots stay visible together, and is deliberately not read by Engine itself
	// today.
	AnchorPath string
}
