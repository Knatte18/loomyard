// Package pawlengine owns the producers of a pawl run that no other recipe shares: Beam-Up, Beam-Await and Beam-Down, the land request Beam-Await consumes, and the beam's ready record.
// A pawl run is the one batten run over a beam's project entry; these rows bring the beam up, hold it until its landing is requested and passing, and bring it down.
//
// The package is told its geometry and every seam by its caller through Deps; it imports no cwd resolver and no CLI package.
// A producer refuses a nil seam it uses when built, naming the field.
// Its records go through internal/state, so a key an older or newer binary wrote survives a rewrite and a request naming another run is read, never refused.
//
// Beam-Up marks the pair a beam, spawns its push, regenerates the hub workspace, removes a land request and the await marker a run before left, and runs the beam steps, each idempotent.
// A failing step is a halting Stuck naming its cause and `lym loom resume`.
//
// Beam-Await spawns no agent.
// It holds the run until a land request naming this run passes the re-check, waking on a change in the request's directory and on a timer that starts at one second and doubles to one minute while idle.
// An accepted request writes the status naming Finalize, so the landing hold stands, then writes the dobby-stop marker, stops the dobby and re-checks the beam through the told Recheck seam: its members and pairs, its owners and its ready record.
// A passing re-check ends the row; a failing one lifts the hold again, restarts the dobby the marker says was stopped, with its orch, removes the request and posts the blockers to the beam's orch.
// An await marker tells a re-entry from the first entry: a re-entry whose dobby is down (once until a re-entry sees it live, recorded in the marker), and a re-entry that finds the hold standing after a crash, post one notice naming `lym beam start` and `lym beam land`.
// The hold stands when the entry's status names Finalize or a row of Deps.RetiredRows, the rows the recipe retired, and no land request stands.
// The entry settles an accepted landing it finds with no land request before it writes any status: by the hold, by the ready record standing with the await marker absent, as after a goto from a halted Finalize, or by the dobby-stop marker standing, as after a resume of a run halted inside the re-check.
// With the ready record standing, read through Deps.ReadyStands, the entry runs the accept's re-check itself and ends the row or refuses as the accept does; with none it lifts the hold, removes the dobby-stop marker, restarts the dobby and posts a notice naming `lym beam ready`.
// A failed read of the ready record decides nothing and is retried at the next wake.
// Each completed wake that does not end the row is stamped through Deps.StampWake, which a failure leaves unchanged.
// A beam another machine owns removes any request and yields, deciding nothing.
//
// Beam-Down stops the beam's dobby and orch through the told Down seam; a failure is logged and the row is Done, since Worktree-Teardown ends what is left.
//
// The land request, the dobby-stop marker and the await marker are files under the beam worktree's `.lym/beam/`, whose paths the hub wiring composes and tells through Paths.
// Paths also names the file of a reopen request an older binary wrote, which no code reads.
//
// MoveOffRetired gives a run moved off a row its recipe retired the side effects an in-flight landing under the old rows needs, through RetiredMoveDeps.
// It ends the PR-Review session, carries the run's scratch pull request into the landing record unless GitHub reports it closed or merged, removes the scratch record, the ready record, the land request, the dobby-stop marker and the reopen request, commits the landing directory, writes the status naming the target row, which lifts the landing hold, and posts one notice naming the commands that bring the beam back.
// A move repeated after the scratch record was removed names the pull request the landing record holds, unless GitHub reports it closed or merged.
// Each step is idempotent, so a retried move repeats safely, and a failed step is returned naming it.
//
// The ready record, ReadyRecord, flags a beam ready to be merged into its parent and names the beam it flags, since a pair forked from the beam inherits it; ReadyFor reads a record naming another slug as no flag, and RemoveReady removes only a record naming the beam.
// It is tracked, unlike the files above, and its path and lock path are told by the caller.
//
// The landing record, LandingRecord, sits beside the ready record, tracked as it is, with its path and lock path told by the caller.
// It names the beam it was written for and records when the description was last written and the beam's pull request, if one is open.
// ReadLanding reads a record naming another slug as not found; UpdateLanding replaces such a record with the beam's own, and DropLandingPullRequest leaves it as it is.
package pawlengine
