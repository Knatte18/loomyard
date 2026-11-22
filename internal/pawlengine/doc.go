// Package pawlengine owns the producers of a pawl run that no other recipe shares:
// Beam-Up, Beam-Await and Beam-Down, the land request Beam-Await consumes, and the beam's ready record.
// A pawl run is the one batten run over a beam's project entry;
// these rows bring the beam up, hold it until its landing is requested and passing, and bring it down.
//
// The package is told its geometry and every seam by its caller through Deps;
// it imports no cwd resolver and no CLI package.
// A producer refuses a nil seam it uses when built, naming the field.
// Its records go through internal/state, so a key a newer binary wrote survives a rewrite.
// A request naming another run is read, never refused.
//
// Beam-Up marks the pair a beam, spawns its push and regenerates the hub workspace.
// It removes a land request and the await marker a run before left, and runs the beam steps, each idempotent.
// Once the beam's dobby and orch are up, Beam-Up hands the beam's Sketch and proposal to the orch, once.
// The hand-off marker records that it is settled, and Beam-Up never removes it.
// A failing step, a failing hand-off or a hand-off post that reached no orch is a halting Stuck.
// The Stuck names its cause and `lym loom resume`.
//
// Beam-Await spawns no agent.
// It holds the run until a land request naming this run passes the re-check of the beam through the told Recheck seam:
// its members and pairs, its owners and its ready record.
// An accepted request writes the status naming Finalize, so the landing hold stands, and stops the dobby.
// A passing re-check ends the row.
// A failing one lifts the hold again, restarts the dobby and its orch and removes the request.
// It also posts the blockers to the beam's orch.
// A re-entry after a crash settles an accepted landing it finds with no land request before it writes any status.
// A failed read of the ready record decides nothing and is retried at the next wake.
// A beam another machine owns removes any request and yields, deciding nothing.
// While the beam's orch is autonomous, a wake that finds no land request posts one `[beam drained]` line to the orch.
// It does so when the beam holds commits beyond its parent, no member or child pair remains and no ready record stands.
// The drain marker records the posted line, so a re-entry posts no second one;
// work that opens again removes the marker, so the next drain posts again.
// The line only informs: it opens, merges or readies nothing.
//
// Beam-Down stops the beam's dobby and orch through the told Down seam.
// A failure is logged and the row is Done, since Worktree-Teardown ends what is left.
//
// The beam worktree's `.lym/beam/` holds these files:
// the land request, the dobby-stop marker, the await marker, the hand-off marker and the drain marker.
// The hub wiring composes their paths and tells them through Paths.
//
// The ready record, ReadyRecord, flags a beam ready to be merged into its parent.
// It names the beam it flags, since a pair forked from the beam inherits it.
// ReadyFor reads a record naming another slug as no flag.
// It is tracked, unlike the files above, and its path and lock path are told by the caller.
//
// The landing record, LandingRecord, sits beside the ready record and is tracked like it.
// The caller tells its path and lock path.
// It names the beam it was written for.
// It records when the description was last written and the beam's pull request, if one is open.
// ReadLanding reads a record naming another slug as not found.
// UpdateLanding replaces such a record with the beam's own, and DropLandingPullRequest leaves it as it is.
package pawlengine
