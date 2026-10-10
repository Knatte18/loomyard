// Package loomcli is the `lyx loom` command: it bootstraps and drives one task's phase machine per worktree on the generic shed engine, and carries the verbs through which the operator and the run's parent answer its gates.
//
// The run, step, status, pause and goto bodies are the shared `internal/shedverbs` verbs, mounted here with loom's own text.
// Friction-Reflect, the last row, runs the Tier 2 friction reflection under run and under step alike, before the run records done.
// `lyx start` is the same start verb mounted a second time at the root, resolving the full engine stack exactly as `lyx loom start` does.
//
// # Start
//
// `lyx loom start` works in order.
// It resolves the recorded parent branch, seeds the status file when absent and commits the seed into the fabric before anything else touches it.
// It brings up the worktree's tmux session;
// a go-driven run ensures its status strand, an llm-driven run removes any status strand the session still holds, and the per-hub watchdog daemon is spawned best-effort.
// Unless a driver is already alive, it spawns the driver the seed records, which is never chosen by a flag;
// a second invocation while a driver runs ensures the substrate instead.
//
// A live loom driver parked at a hand-back is resumed by typing one line into its pane, and start refuses after a bounded wait when that pane is not ready, since the driver may be busy having resumed on its own.
// A live loom driver over a run halted at a hand-back (awaiting, blocked, paused or failed) that has not yet written its park marker is still writing its stop report, so start refuses with the kind `driver_not_parked`;
// a live driver over a running run is left working.
// Before spawning or resuming a driver, start refuses with the kind `merge_in_progress` over an unfinished merge, naming the conflicted paths and the remedy, the fabric verbs for a fabric merge and git for one fabric did not start.
// The exception is a run whose current producer is Publish or Finalize, which goes through over a parked fabric merge-in of its own parent branch, since that row aborts and redoes it.
// A loom driver strand reed marked retiring is never adopted: start removes it and spawns a fresh driver.
//
// start returns once the driver's readiness signal confirms it is up, and for a parked loom driver once delivery of the resume line is verified.
// For the Go driver the signal is the run lock being taken;
// for a loom driver it is the provider TUI coming up ready within shuttle's `startup_timeout_s`, with any one-time startup gate its provider requires dismissed along the way.
// A readiness refusal removes the driver strand, so the next start spawns a fresh one.
// The signal is checked only for a driver this invocation spawns, so a strand left live because shuttle got no liveness answer from reed, or could not remove the strand, is returned over by a later start without re-checking.
//
// The detached Go driver writes its stdout and stderr to the driver log in the ephemeral tree, never to start's own output;
// a loom driver strand writes no such log, since its pane holds its output.
// A loom driver strand launches from the driver stencil, read from the stencils directory at start time.
//
// A worktree opened through `lyx ide spawn`'s generated VS Code task runs `lyx reed up`, `lyx reed add --if-absent --cmd claude --name claude --focus` and `lyx reed attach`, so the operator's own session is the strand named `claude` and the run's panes are its siblings.
// To self-check, compare `$TMUX_PANE` against the tracked strands `lyx reed status` reports: tracked is fine;
// set but untracked means relaunching through that chain or proceeding without reed supervision;
// unset is unconfirmed rather than failed, since psmux on Windows may not export it.
// A worktree whose `.vscode/tasks.json` predates this convention is upgraded by deleting that file and re-running `lyx ide spawn`.
//
// # Resume
//
// `lyx loom resume` takes the bootstrap lock start takes, so the two never run at once, and decides by the run's state:
//
//   - no status file: refused, naming `lyx loom start`.
//   - done: a no-op success.
//   - awaiting at a review segment's Bouncer row with a circling decision recorded: handled as a halted run, so a live, parked driver is woken and its step acts on the decision.
//   - awaiting at a Bouncer row with no decision: refused, naming the circling verbs and then resume.
//   - awaiting at a Bouncer row with an unreadable decision: refused naming the file, to fix it or delete it and record the decision again.
//   - awaiting at any other row: refused, naming `lyx loom approve` or `lyx loom reject`, after which a batten run watching the child resumes it, or `lyx batten run <slug>` from the prime when none does.
//   - running: a no-op success when the driver is live or holds the run lock, and refused as a halted run otherwise.
//   - halted with the run lock held, or with a live driver and no park marker: refused with the kind `driver_not_parked`, to retry shortly.
//   - halted with a live driver and a park marker: resumed, refused with `merge_in_progress` over an unfinished merge under start's Publish and Finalize exception.
//   - halted with no live driver: refused over an unfinished merge with `merge_in_progress`, otherwise naming `lyx batten run <slug>` from the prime and then resume for a dead strand, and `lyx loom start` for a retiring strand or none.
//
// The driver strand is read from reed's directory, which still lists a session that died with the machine, never from the run's seed.
// A resume line that cannot be delivered is refused, naming `lyx loom status` and a re-run of resume.
package loomcli
