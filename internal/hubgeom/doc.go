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
// ReconcileGeometry converts a Location into the hubreconcile.Geometry the start verbs and the Preflight row hand to hubreconcile.Ensure,
// and reports false for a repository with no hub-level board lyx dir, which never reconciles.
// Later waves add their own siblings here rather than spawning per-engine packages or re-deriving the
// construction inline at each call site.
//
// ReedGeometry also tells reed the hub's spawn order (spawnorder.go), as a lazy closure over the Location that does nothing until a revival is due:
// it lists the hub's worktrees with fabricengine.List, skips prunable and unresolvable ones, and orders the prime first, then each pair by its run's start time, then pairs without one by name.
// A pair's start time is the `started_at` stamp of its own run seed, which shedrun.WriteSeed writes once when the run is seeded; the file's modification time is never read, so a checkout, rebase or re-materialized worktree cannot move it.
// A pair whose seed is absent, has no stamp or has one that does not parse is undated.
// Runs seeded before the stamp existed are undated once, and sort by name after the dated pairs.
// Each entry's Revive builds that worktree's own reed engine from its reed.yaml and its own ReedGeometry.
//
// The tellers do I/O only for Board and run state that no Location carries, and spawn nothing.
// ReedGeometry reads the worktree's .git entry to tell the prime from a task worktree, the hub's recorded shortname, and a task worktree's parent through ResolveParent.
// BurlerGeometry and WebsterGeometry read the parent through ResolveParent too, as the name their spawned roles' parent directive renders.
// ResolveParent reads the pair's origin record alone; an origin that names no parent worktree, or no origin record, yields no parent.
// An origin record whose sibling worktree lacks its lock directory is an error naming `lyx fabric reconcile`, never a silent "no parent".
// A missing or unreadable .git entry is ReedGeometry's one error, and an unresolvable parent leaves ParentName empty with a warning in all three.
// hubgeom is still the only reader — no engine resolves a parent, per the Told-Geometry Invariant.
//
// Standalone CLIs do not call hubgeom — they have no Location to convert, resolving their own
// geometry from CLI flags and local paths instead.
package hubgeom
