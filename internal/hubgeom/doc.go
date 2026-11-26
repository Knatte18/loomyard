// Package hubgeom is the hub-mode adapter that tells engines their geometry:
// it converts a resolved *lymcwd.Location into the geometry struct each engine holds.
// No engine therefore derives its own coordinates from a Location itself.
// The engines it serves never import it, so the told direction stays one-way:
// hubgeom depends on the engines, never the reverse.
// New teller functions belong here, not in per-engine packages or inline at each call site.
//
// Each teller converts a Location, or a hub path or worktree name, into one engine's geometry:
// reed, burler, webster, weaver, hub reconcile, pause, dobby and slack.
// ReconcileGeometry reports false for a repository with no hub-level board lym dir, which never reconciles.
// GateSlots is the one constructor of a hub's gate-slot pool, over the Bolt checkout's slot directory.
// Its limits are read from the Bolt checkout's gate.yaml and this machine's override at each acquire.
//
// ReedGeometry also tells reed the hub's spawn order (spawnorder.go), a lazy closure over the Location.
// The closure skips prunable and unresolvable worktrees.
// It orders the prime first, then each pair by its run's start time, then pairs without one by name.
// A pair's start time is the `started_at` stamp of its own run seed, which shedrun.WriteSeed writes once.
// The file's modification time is never read, so a checkout, rebase or re-materialized worktree cannot move it.
// A pair whose seed is absent, has no stamp or has one that does not parse is undated.
//
// The tellers do I/O only for Board and run state that no Location carries, and spawn nothing.
// ReconcileBeforeLoad is no teller: it serves a verb about to load a reconciled config.
// It runs the hub reconcile over ReconcileGeometry, and that reconcile writes, commits and spawns.
// BurlerGeometry tells the shortname and slug as ReedGeometry does, from the same prime test.
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
// A worktree with no origin record ends the walk at the prime.
// So does an empty parent worktree, a parent worktree that is gone, and a cycle.
// An error from the predicate is returned.
//
// Autonomy is the reader of beam.yaml's `autonomy` for runs and orchs.
// It answers for the worktree it is named, through TaskSettingChain and the per-task lookup, at the moment of the call.
// A failed read answers false and warns once per episode.
//
// BeamModes is the board's beam mode reader.
// It reads autonomy and pull_request for the root and each beam the way their runs read them.
// The read goes through TaskSettingChain and the per-task lookup, and each value carries the worktree that set it.
// It adds a beam's parent branch from its origin record and its pull request from its landing record.
// A beam whose worktree is not on this machine is left out,
// so the board files never show its parent's values as its own runs' values.
// It finds the prime from the board directory alone:
// the Bolt checkout is a worktree of the records repository, whose main worktree is the prime's records sibling.
//
// A pair reads as a beam when its origin record carries the beam mark or its board entry is a project with the beam flag.
// So a beam reads as one before its pawl run marks the pair.
// A failed prime read is taken as the prime and a failed beam read as a task, each logged at Warn.
//
// Standalone CLIs do not call hubgeom:
// they have no Location to convert, and resolve their own geometry from CLI flags and local paths instead.
package hubgeom
