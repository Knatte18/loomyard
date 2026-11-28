// Package pawlengine owns the producers of a pawl run that no other recipe shares:
// Beam-Up, Beam-Await and Beam-Down, the land request Beam-Await consumes, and the beam's ready record.
// A pawl run is the one batten run over a beam's project entry;
// these rows bring the beam up, hold it until its landing is requested and passing, and bring it down.
//
// The package is told its geometry and every seam by its caller through Deps;
// it imports no cwd resolver and no CLI package.
// A producer refuses a nil seam it uses when built, naming the field.
// Its records go through internal/state, so a key an older or newer binary wrote survives a rewrite.
// A request naming another run is read, never refused.
//
// Beam-Up marks the pair a beam, spawns its push and regenerates the hub workspace.
// It removes a land request and the await marker a run before left, and runs the beam steps, each idempotent.
// Once the beam's dobby and orch are up, Beam-Up hands the beam's Sketch and proposal to the orch, once.
// It hands off through the told HandOff and Notify seams.
// The hand-off marker records that it is settled, and Beam-Up never removes it.
// A failing step, a failing hand-off or a hand-off post that reached no orch is a halting Stuck.
// The Stuck names its cause and `lym loom resume`.
//
// Beam-Await spawns no agent.
// It holds the run until a land request naming this run passes the re-check.
// It wakes on a change in the request's directory and on a timer.
// The timer starts at one second and doubles to one minute while idle.
// An accepted request writes the status naming Finalize, so the landing hold stands.
// It then writes the dobby-stop marker and stops the dobby.
// It re-checks the beam through the told Recheck seam: its members and pairs, its owners and its ready record.
// A passing re-check ends the row.
// A failing one lifts the hold again and restarts the dobby the marker says was stopped, with its orch.
// It removes the request and posts the blockers to the beam's orch.
// An await marker tells a re-entry from the first entry.
// Two re-entries post one notice naming `lym beam start` and `lym beam land`.
// One is a re-entry whose dobby is down, once until a re-entry sees it live, recorded in the marker.
// The other is a re-entry that finds the hold standing after a crash.
// The hold stands when the entry's status names Finalize or a row of Deps.RetiredRows, the rows the recipe retired,
// and no land request stands.
// The entry settles an accepted landing it finds with no land request before it writes any status.
// These states mark such a landing: the hold standing;
// the ready record standing with the await marker absent, as after a goto from a halted Finalize;
// and the dobby-stop marker standing, as after a resume of a run halted inside the re-check.
// When the ready record stands, read through Deps.ReadyStands, the entry runs the accept's re-check itself.
// It ends the row or refuses as the accept does.
// Without the ready record, the entry lifts the hold and removes the dobby-stop marker.
// It restarts the dobby and posts a notice naming `lym beam ready`.
// A failed read of the ready record decides nothing and is retried at the next wake.
// Each completed wake that does not end the row is stamped through Deps.StampWake, which a failure leaves unchanged.
// A beam another machine owns removes any request and yields, deciding nothing.
//
// Beam-Down stops the beam's dobby and orch through the told Down seam.
// A failure is logged and the row is Done, since Worktree-Teardown ends what is left.
//
// The beam worktree's `.lym/beam/` holds these files:
// the land request, the dobby-stop marker, the await marker and the hand-off marker.
// The hub wiring composes their paths and tells them through Paths.
// Paths also names the file of a reopen request an older binary wrote, which no code reads.
//
// MoveOffRetired runs the side effects that an in-flight landing needs when its run is moved off a retired row.
// It works through RetiredMoveDeps.
// It ends the PR-Review session.
// It carries the run's scratch pull request into the landing record unless GitHub reports it closed or merged.
// It removes the scratch record, the ready record, the land request, the dobby-stop marker and the reopen request.
// It commits the landing directory.
// It then writes the status naming the target row, which lifts the landing hold.
// It posts one notice naming the commands that bring the beam back.
// A move repeated after the scratch record was removed names the pull request the landing record holds.
// It names none when GitHub reports that pull request closed or merged.
// Each step is idempotent, so a retried move repeats safely, and a failed step is returned naming it.
//
// The ready record, ReadyRecord, flags a beam ready to be merged into its parent.
// It names the beam it flags, since a pair forked from the beam inherits it.
// ReadyFor reads a record naming another slug as no flag, and RemoveReady removes only a record naming the beam.
// It is tracked, unlike the files above, and its path and lock path are told by the caller.
//
// The landing record, LandingRecord, sits beside the ready record and is tracked like it.
// The caller tells its path and lock path.
// It names the beam it was written for.
// It records when the description was last written and the beam's pull request, if one is open.
// ReadLanding reads a record naming another slug as not found.
// UpdateLanding replaces such a record with the beam's own, and DropLandingPullRequest leaves it as it is.
package pawlengine
