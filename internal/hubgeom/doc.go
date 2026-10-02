// Package hubgeom is the hub-mode adapter that tells engines their geometry: it converts a resolved
// *lyxcwd.Location into the geometry struct each engine holds, so no engine derives its own
// coordinates from a Location itself.
// The engines it serves never import it — reedengine.Geometry knows nothing of hubgeom, and neither
// will burlerengine's or websterengine's own geometry types — so the told direction
// stays one-way: hubgeom depends on the engines, never the reverse.
//
// hubgeom's contract today is ReedGeometry, BurlerGeometry, and WebsterGeometry,
// converting a Location into a reedengine.Geometry, a burlerengine.Geometry, and a
// websterengine.Geometry respectively.
// Later waves add their own siblings here rather than spawning per-engine packages or re-deriving the
// construction inline at each call site.
//
// ReedGeometry is the one teller that does I/O: it reads the hub's recorded code and a task worktree's
// default-run seed, because the name prefix and parent are weft and run state that no Location carries.
// It is still the only reader — reedengine resolves neither, per the Told-Geometry Invariant.
//
// Standalone CLIs do not call hubgeom — they have no Location to convert, resolving their own
// geometry from CLI flags and local paths instead.
package hubgeom
