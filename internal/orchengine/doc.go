// Package orchengine is the engine behind `lyx orch`: the hub orchestrator hosted as an interactive shuttle run in the prime worktree's reed session, cycled before its context fills.
//
// The orchestrator is one long-lived Claude session that outlives any single context window.
// A detached watcher reads each turn end's context usage from the provider transcript.
// When usage crosses the configured threshold and the session is idle, the watcher has the session write a handoff, clears it, and resumes it from that handoff.
// A session is never cleared without a written handoff.
//
// # Verbs, and where they run
//
// `lyx orch` runs from the hub's prime worktree only; every verb refuses elsewhere.
//
//   - start: the idempotent bootstrap.
//     It leaves one live orchestrator strand and one watcher bound to it, then hands the terminal over to reed's attach.
//   - status: reports the strand, the watcher and the persisted cycle state.
//   - cycle: writes the cycle request, which makes the watcher cycle at its next idle moment regardless of the token count.
//   - stop: removes the orchestrator strand; the watcher notices and exits on its own.
//   - watch: the hidden daemon verb `start` spawns detached; it is not an operator verb.
//
// The verbs, the reed and shuttle wiring, and the strand-identity rules live in internal/orchcli.
// This package holds what sits behind the seam: the config, the stencil renders, the persisted state, `start`'s two decisions, and the watcher.
// It is told every path through Paths and derives none (Told-Geometry Invariant).
//
// # Start prompt order
//
// ChooseStartPrompt picks the launch prompt in a fixed order:
//
//  1. The resume stencil pointed at the `--handoff` file, when the flag is given.
//     A flag naming a missing file is an error rather than a fallback.
//  2. The resume stencil pointed at State.LastHandoff, when that file still exists.
//     A PendingHandoff is never chosen, since an aborted cycle may have left it partial.
//  3. The start stencil, for a first launch.
//
// DecideStart maps the strand and watcher liveness pair onto the branch `start` takes:
// attach only, spawn a watcher, or relaunch.
// A dead or absent strand always relaunches.
//
// # The watcher
//
// Watcher.Run polls at the configured interval, calling Tick once per poll.
// It holds watch.lock for its whole life, so at most one watcher runs per prime;
// a second one exits with ErrWatcherRunning.
// The consecutive tick-error count is capped, and the watcher exits with its reason recorded in State.WatcherExit once the cap is reached.
// Every provider and reed interaction goes through the Session seam, so the state machine runs against a fake in unit tests.
//
// # Idle rules
//
// In phase idle the watcher acts only when the context reading is at or over the threshold, or a cycle was requested, and only when all of these hold:
//
//   - The newest event it has read is a turn end.
//   - That event was first read at least the idle grace ago.
//     The arrival time is held in memory, so a watcher restart restarts the grace.
//   - Session.SessionIdle reports an empty input box with no turn running.
//
// A context reading that cannot be taken is unknown and never triggers a cycle by itself.
// The template threshold is 400000 tokens, sized for a session with a context window of about one million tokens.
//
// # The four-phase cycle
//
// The persisted phases are idle, handoff-requested, clearing and resuming.
// The watcher saves State before every side effect, so a restarted watcher resumes from it.
//
//   - handoff-requested: the handoff instruction is sent, naming a new timestamped file under handoffs/.
//     The phase ends once the file exists and is non-empty, a turn end has been read after the file was first seen written,
//     and the idle probe passes.
//   - clearing: the resume prompt is rendered first, so a stencil failure aborts before anything is cleared.
//     Then `/clear` is typed, and the phase waits for the pane to show an idle input box.
//   - resuming: the resume prompt is sent verbatim, and the phase ends at the resumed session's first turn end, whose context reading becomes the new one.
//   - idle: every return, completed or aborted, persists the events position read through.
//
// Nothing is ever typed into the pane, an instruction, a prompt or `/clear`, unless the idle probe passed on the same tick,
// so a running turn, a permission prompt or an operator's draft is never typed over.
// When in doubt the watcher waits.
//
// Restart rules: a non-idle phase's injection is unconfirmed until a turn end proves it landed or a passing idle probe shows it did not, and it is then sent again.
// The turn ends a restarted watcher re-reads from a handoff-requested phase's start never open the clear gate, since their order against the file write is unknown.
// A phase belonging to another strand is reset to idle.
// A handoff-requested or resuming phase that exceeds the handoff timeout, or a session that asks a question during the handoff, aborts back to idle with LastAbortReason set.
// A clearing phase that exceeds it keeps waiting for the idle probe and records the wait in State.Stuck, which `status` reports.
// Once the session is idle it moves on to resuming, and an unconfirmed `/clear` is not retried then, since the handoff is already written and the resume prompt lands in an idle, uncleared session.
// The guarantee holds through every restart: nothing before the handoff file is written and seen can reach `/clear`, and LastHandoff moves only when a cycle passes that gate.
//
// A tick saves State only while the record still names the strand it loaded, checked under the state lock,
// so a watcher never overwrites the binding a concurrent `start` just recorded.
//
// # The .lyx/orch/ layout
//
// Everything orch writes lives under the prime's `<anchor>/.lyx/orch/`, declared once in internal/orchcli/paths.go:
//
//   - state.json and state.json.lock: the persisted State and its lock.
//   - watch.lock: held for the watcher's life.
//   - start.lock: serializes `start`.
//   - cycle-request: the marker `cycle` writes and the watcher consumes.
//   - watch.log: the detached watcher's stdout and stderr.
//   - handoffs/: one timestamped handoff file per cycle, all kept.
//   - session-*.never: one sentinel per launch, an output file nothing ever writes.
//     A shuttle Spec needs an output file to validate, and the watcher, not shuttle's Wait, owns the session's lifetime.
//
// # Migrating from an operator's own terminal session
//
// In the running terminal session, run `/scribe:handoff` and note the file it writes, then exit that session.
// Then run `lyx orch start --handoff <that file>` from the prime.
// The new orchestrator strand starts from that handoff instead of the start stencil.
//
// # Residuals
//
//   - Shuttle switches Claude's prompt suggestion off in every settings file it writes, and the idle probe still fails closed on any non-empty box, so a session with a draft is never cycled until it is cleared.
//   - A SendMessage landing between the handoff turn's end and `/clear` is lost from context.
//   - The transcript and Stop-payload shapes are Claude Code internals, so usage degrades to unknown rather than failing.
//   - A threshold above the auto-compaction point lets Claude Code compact first.
//   - reed's resume path replays the launch's `--resume <session-id>`, which names the pre-clear session once a cycle has run, so a reed server rebirth resumes the older context.
//     `lyx orch stop` plus `lyx orch start` relaunches from the last completed handoff instead.
//
// # Open risks
//
// Two questions are settled only by a real session.
// `TestSmokeOrch_OneFullCycle` in internal/orchcli logs an observation for each, but has not been run against a live Claude Code install, so both remain unverified, pending a smoke run:
//
//   - Whether a background task survives `/clear`: unverified, pending a smoke run.
//   - Whether a `SendMessage` address stays stable across `/clear`: unverified, pending a smoke run.
package orchengine
