// Package tenderengine implements the pause, resume, revive and status sequences behind `lym tender`, over seams.
//
// A pause records an entry and waits a short grace for busy sessions and network operations to finish.
// It then interrupts every covered session still in a running turn with Esc, and reports what it saw.
// A resume lifts entries and gives each session it interrupted one line to carry on.
// A revive gives one carry-on line to each session stalled on an API error.
// Status reads the live entries and the holds a pause leaves, and changes nothing.
//
// Every path and every probe is told through Deps; the engine resolves no working directory and spawns nothing.
// Pair is one hub pair as the sequences see it, and the CLI fills it over shuttle and reed.
// The hub has one orch per worktree that runs one, and every reading of "the orch" covers all of them.
//
// # Pause
//
// A pause covers the whole hub for an empty slug and one pair for a slug.
// A live entry of the same scope adds no entry.
// An entry resumed during the grace ends the sequence with no Esc.
// A request's `Owner`, `Record` and `Added` callbacks are documented on the request type;
// an error from any of them ends the pause, and `Owner` refuses it with no entry written.
//
// Each covered session that reads a running turn is recorded in the entry before it is interrupted,
// so a pause killed before its Esc still leaves the session recorded.
// The caller's own session and a session another live entry owns are skipped.
// The engine sends Esc only, never text; it kills no pane, strand or process and never touches the orch.
// A session whose turn ended with background work is sent no key and is listed under `background`.
// A refusal, a failed capture and a session still running after the second settle are reported, not retried.
//
// # Resume
//
// An empty slug resumes every live entry; a slug resumes that pair's own entry.
// A resume lifts an entry and drives nothing beyond giving each session it interrupted `pauserecord.ResumeLine`.
// A request's `Verdict` and `Clear` are asked first, and either one refusing leaves nothing lifted.
// A slug that only a live hub entry covers is refused, naming `lym tender resume`.
// An adopt pause is an operator pause: a resume of its slug lifts it.
// A session another live entry still covers moves to that entry with no line.
// A send refused because a live entry added after the lift covers the session is claimed into that entry.
// Each orch's session is left to its own orch watcher.
//
// A lift that lifted at least one entry then spawns what a pause deferred.
// It spawns through `Deps.PushPair`, `Deps.PushPrimeRecords` and `Deps.SyncBoardIfOwed`;
// a nil seam spawns nothing.
// The report lists the spawns under `spawned` and a failure under `spawn_failed`; neither fails the lift.
// An owners reading that fails spawns no pair push, since an owner it cannot read may be another machine.
//
// # Bolt hold
//
// While an adopt hold stands in the board, each machine holds its runs through `bolt-hold` slug entries.
// `SyncBoltHold` settles one dobby's runs against the hold.
// It writes and lifts only `bolt-hold` entries, so an operator's hub entry or slug entry stands.
// While a hold stands, a resume leaves every `bolt-hold` entry standing
// and refuses `lym tender resume <slug>` of a slug whose own entry is a `bolt-hold` one.
// `Deps.BoltHold` reads the hold the local Bolt records, and a nil seam reads none.
//
// # Revive
//
// An empty slug covers the whole hub; a slug covers that pair.
// Revive never waits for a session.
// It sends `ReviveLine` only to a session that reads idle-stalled with cause `api-error` at the send's own reading.
// A session a live entry covers is reported `paused` and sent nothing, since resume gives it its line.
// No send is retried, so a second revive finds a revived session busy or done and sends nothing.
//
// For the hub scope only, Revive then revives each orch `Deps.Orchs` lists.
// It uses a checked creel inject, `Orch.InjectRevive`, and never types into the orch's pane.
// Two concurrent revives inject at most once.
// Revive sends no key, kills nothing and changes no live entry.
//
// The coverage read and the send run inside one `pauserecord.Update` that changes no entry.
// A pause, resume or orch watcher record write therefore waits at most one send's hold.
// The orch's condition is read before the inject.
// A session that leaves the stall within the inject's time to live can still get the line typed.
// That starts one extra turn.
//
// # Status
//
// Status reports the live entries, the network operations in flight, and the holds.
// It adds read-only parts: every live session with its state and coverage;
// the processes running a replaced binary; the processes registered in the owner record with their orphan reading;
// and the owners of local pairs.
// A failed or unsupported reading is reported in its part and leaves the rest of the report unaffected.
// Status reads no transcript and no pane content and writes nothing, so any caller may run it.
//
// # Reap
//
// Reap takes one fresh reading of the processes part and ends exactly the orphans it lists.
// It ends them through the `Deps.EndOwner` and `Deps.EndProcess` seams; a nil seam ends nothing.
// A pid reused since the reading is left alone.
// A reap never ends these, since they are never orphans:
// the hub owner, a pair that exists, a process in a registered worktree and a slug whose batten reads running.
// A survivor returns the report with an error naming each survivor's pid and role.
// Any other end error is returned after the remaining orphans are tried.
package tenderengine
