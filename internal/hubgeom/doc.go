// Package hubgeom is the hub-mode adapter that tells engines their geometry:
// it converts a resolved *lymcwd.Location into the geometry struct each engine holds.
// No engine therefore derives its own coordinates from a Location itself.
// The engines it serves never import it, so the told direction stays one-way:
// hubgeom depends on the engines, never the reverse.
//
// ReedGeometry converts a Location into a reedengine.Geometry.
// BurlerGeometry converts it into a burlerengine.Geometry, and WebsterGeometry into a websterengine.Geometry.
// ReconcileGeometry converts a Location into the hubreconcile.Geometry that start verbs hand to EnsureOrWarn.
// The Preflight row hands it over too.
// It reports false for a repository with no hub-level board lym dir, which never reconciles.
// GateSlots is the one constructor of a hub's gate-slot pool, over the Bolt checkout's slot directory.
// Its limits are read from the Bolt checkout's gate.yaml and this machine's override at each acquire;
// WebsterGeometry hands that pool to webster.
// New teller functions belong here, not in per-engine packages or inline at each call site.
// PauseGeometry converts a Location into the pauserecord.Geometry every reader of a pause is told:
// the record under the board-level `.lym` and the pair's slug, empty for the prime.
// DobbyGeometry converts a worktree's name into the dobbyengine.Geometry its dobby is told.
// That geometry holds its lock, state, log and output files.
// It holds its tidy directory under the worktree's own `.lym/beam/`.
// It also holds the batten runs directory under the prime and the anchor-move records that stop it.
//
// SlackGeometry converts a hub path into the slackengine.Geometry the Slack engine is told.
// That geometry holds the hub's Slack directory and its files under the board-level `.lym`.
// It holds the token file and the machine socket lock in the per-user credential directory.
// It also holds the machine name, the process-owner directory and the running `lym` binary.
//
// ReedGeometry tells reed the hub-level discover signal file in DiscoverSignalPath, beside LogsDir:
// reedengine.DiscoverSignalFileName under fabricengine.HubScratchDir.
//
// ReedGeometry also tells reed the hub's spawn order (spawnorder.go).
// The order is a lazy closure over the Location that does nothing until a revival is due.
// The closure lists the hub's worktrees with fabricengine.List and skips prunable and unresolvable ones.
// It orders the prime first, then each pair by its run's start time, then pairs without one by name.
// A pair's start time is the `started_at` stamp of its own run seed, which shedrun.WriteSeed writes once.
// The file's modification time is never read, so a checkout, rebase or re-materialized worktree cannot move it.
// A pair whose seed is absent, has no stamp or has one that does not parse is undated.
// Each entry's Revive builds that worktree's own reed engine from its reed.yaml and its own ReedGeometry.
//
// The tellers do I/O only for Board and run state that no Location carries, and spawn nothing.
// ReedGeometry reads the worktree's .git entry to tell the prime from a task worktree.
// It also tells the hub's recorded shortname, and a task worktree's parent through ResolveParent.
// BurlerGeometry and WebsterGeometry read the parent through ResolveParent too.
// They render it as the name their spawned roles' parent directive carries.
// BurlerGeometry also tells the shortname and slug as ReedGeometry does, from the same prime test.
// The agent names burler forms therefore match the names reed gives the strands.
// ResolveParent reads the pair's origin record alone.
// An origin that names no parent worktree, or no origin record, yields no parent.
// An origin record whose sibling worktree lacks its lock directory is an error naming `lym fabric reconcile`.
// It is never a silent "no parent".
// A missing or unreadable .git entry is the one error of ReedGeometry and of BurlerGeometry.
// An unresolvable parent leaves ParentName empty with a warning in all three.
// hubgeom is the only reader of the parent: no engine resolves one, per the Told-Geometry Invariant.
//
// ParentOf is the one parent resolver every owner reader is handed.
// It returns the slug of the pair an entry was forked from, "" for the prime.
// It reads the slug's origin record over the prime's location.
// Only when no record is found does it read the nearest open beam above the entry on the local board.
// A found record with no parent worktree reads as the prime and never falls through to the board.
// A found record's parent is its ParentWorktree.
// It reads as the prime when the record names the prime's checked-out branch or the prime's worktree.
// It is never derived by matching the recorded branch against the pairs present here.
// So a member whose beam pair is absent on this machine still reads the beam as its parent.
// The beam's owner reads as its own.
// ResolveParent answers the parent pair's orch name for a reader that needs the pair itself:
// the parent names the driver's prompts carry.
// The `lym loom decision add --by parent` guard admits the destination of notices instead,
// so the orch that receives them can record the call.
//
// NearestLiveOrch is the destination of a notice to an orch.
// It tries the worktree it is told, then walks upward through the origin records' parent worktrees.
// It answers the nearest worktree whose orch a told predicate reports live.
// When none is live it answers the prime, whatever its state.
// The predicate is told because the orch's state paths and reed belong to a package that imports hubgeom.
// The answer's Name is that worktree's orch agent name, formed as ResolveParent forms it.
// Its Worktree names the worktree, empty for the prime.
// A worktree with no origin record ends the walk at the prime.
// So does an empty parent worktree, a parent worktree that is gone, and a cycle.
// An error from the predicate is returned.
//
// ReedGeometry also tells reed a lazy button-context closure (buttons.go).
// It carries the place, the placeholder values and desktop's machine buttons.
// A pair's place is the beam's when its origin record carries the beam mark.
// It is also the beam's when its board entry is a project with the beam flag.
// So a beam reads as one before its pawl run marks the pair.
// It also carries a {beam} resolver that reads the parent through ParentOf over the prime's location only when called.
//
// Standalone CLIs do not call hubgeom:
// they have no Location to convert, and resolve their own geometry from CLI flags and local paths instead.
package hubgeom
