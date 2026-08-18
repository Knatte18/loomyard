// Package hubgeom is the hub-mode adapter that tells engines their geometry: it converts a resolved
// *lyxcwd.Location into the geometry struct each engine holds, so no engine derives its own
// coordinates from a Location itself.
// The engines it serves never import it — reedengine.Geometry knows nothing of hubgeom, and neither
// will burlerengine's or perchengine's or websterengine's own geometry types — so the told direction
// stays one-way: hubgeom depends on the engines, never the reverse.
//
// hubgeom's contract today is ReedGeometry, BurlerGeometry, and PerchGeometry, converting a Location
// into a reedengine.Geometry, a burlerengine.Geometry, and a perchengine.Geometry respectively.
// Later waves add their own siblings here rather than spawning per-engine packages or re-deriving the
// construction inline at each call site: T7 adds WebsterGeometry.
//
// Standalone CLIs do not call hubgeom — they have no Location to convert, resolving their own
// geometry from CLI flags and local paths instead.
package hubgeom
