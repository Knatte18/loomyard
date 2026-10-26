// Package pawlengine owns the producers of a pawl run that no other recipe shares: Beam-Up, Beam-Await and Beam-Down, and the land request Beam-Await consumes.
// A pawl run is the one batten run over a beam's project entry; these rows bring the beam up, hold it until its landing is requested and passing, and bring it down.
//
// The package is told its geometry and every seam by its caller through Deps; it imports no cwd resolver and no CLI package.
// A producer refuses a nil seam it uses when built, naming the field.
// Its records go through internal/state, so a key an older or newer binary wrote survives a rewrite and a request naming another run is read, never refused.
//
// Beam-Up marks the pair a beam, spawns its push, regenerates the hub workspace, removes a land request and the await marker a run before left, and runs the beam steps, each idempotent.
// A failing step is a halting Stuck naming its cause and `lyx loom resume`.
//
// Beam-Await spawns no agent.
// It holds the run until a land request naming this run passes the re-check, waking on a change in the request's directory and on a timer that starts at one second and doubles to one minute while idle.
// An accepted request writes the status naming Parent-Align, so the landing hold stands, then writes the dobby-stop marker, stops the dobby and re-checks the beam's members and pairs.
// A passing re-check writes the snapshot and ends the row; a failing one lifts the hold again, restarts the dobby the marker says was stopped, removes the request and posts the blockers to the beam's orch.
// An await marker tells a re-entry from the first entry: a re-entry whose dobby is down (once until a re-entry sees it live, recorded in the marker), and a re-entry that finds the hold standing after a crash, post one notice naming `lyx beam start` and `lyx beam land`.
// A beam another machine owns removes any request and yields, deciding nothing.
//
// Beam-Down stops the beam's dobby and orch through the told Down seam; a failure is logged and the row is Done, since Worktree-Teardown ends what is left.
//
// The land request, the dobby-stop marker and the await marker are files under the beam worktree's `.lyx/beam/`, whose paths the hub wiring composes and tells through Paths.
package pawlengine
