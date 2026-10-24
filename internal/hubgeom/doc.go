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
// ReconcileGeometry converts a Location into the hubreconcile.Geometry the start verbs and the Preflight row hand to hubreconcile.EnsureOrWarn, and reports false for a repository with no hub-level board lyx dir, which never reconciles.
// GateSlots is the one constructor of a hub's gate-slot pool, over the board dir's slot directory, with its limits read from the board dir's gate.yaml and this machine's override at each acquire;
// WebsterGeometry hands that pool to webster.
// Later waves add their own siblings here rather than spawning per-engine packages or re-deriving the
// construction inline at each call site.
// PauseGeometry converts a Location into the pauserecord.Geometry every reader of a pause is told: the record under the board-level `.lyx` and the pair's slug, empty for the prime.
//
// ReedGeometry tells reed the hub-level discover signal file in DiscoverSignalPath, beside LogsDir: reedengine.DiscoverSignalFileName under fabricengine.HubScratchDir.
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
// BurlerGeometry also tells the shortname and slug as ReedGeometry does, from the same prime test, so the agent names burler forms match the names reed gives the strands.
// ResolveParent reads the pair's origin record alone; an origin that names no parent worktree, or no origin record, yields no parent.
// An origin record whose sibling worktree lacks its lock directory is an error naming `lyx fabric reconcile`, never a silent "no parent".
// A missing or unreadable .git entry is the one error of ReedGeometry and of BurlerGeometry, and an unresolvable parent leaves ParentName empty with a warning in all three.
// hubgeom is still the only reader — no engine resolves a parent, per the Told-Geometry Invariant.
//
// ParentOf is the one parent resolver every owner reader is handed: it returns the slug of the pair an entry was forked from, "" for the prime.
// It reads the slug's origin record over the prime's location, and only when no record is found reads the nearest open beam above the entry on the local board.
// A found record with no parent worktree reads as the prime and never falls through to the board.
// A found record's parent is its ParentWorktree, read as the prime when the record names the prime's checked-out branch or the prime's worktree, and never derived by matching the recorded branch against the pairs present here.
// So a member whose beam pair is absent on this machine still reads the beam as its parent, and the beam's owner as its own.
// ResolveParent keeps answering the parent pair's orch name for a reader that needs the pair itself:
// the parent names the driver's prompts carry, and the Discussion-Write parent review, whose reviewer is the recorded parent's own orch and is absent when that orch is not live.
// The `lyx loom decision add --by parent` guard admits the destination of notices instead, so the orch that receives them can record the call.
//
// NearestLiveOrch is the destination of a notice to an orch.
// It tries the worktree it is told, then walks upward through the origin records' parent worktrees.
// It answers the nearest worktree whose orch a told predicate reports live, or the prime whatever its state when none is.
// The predicate is told because the orch's state paths and reed belong to a package that imports hubgeom.
// The answer's Name is that worktree's orch agent name, formed as ResolveParent forms it, and its Worktree names the worktree, empty for the prime.
// A worktree with no origin record, an empty parent worktree or a parent worktree that is gone ends the walk at the prime, and so does a cycle; an error from the predicate is returned.
//
// Standalone CLIs do not call hubgeom — they have no Location to convert, resolving their own
// geometry from CLI flags and local paths instead.
package hubgeom
