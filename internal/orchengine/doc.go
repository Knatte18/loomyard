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
//     `--adopt <session-id>` resumes an existing Claude session as the orchestrator strand instead of launching a fresh one.
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
// The launch prompt is picked in a fixed order:
//
//  1. The adopt stencil, when `--adopt` is given; `start` renders it directly, and it is exclusive with `--handoff`.
//  2. The resume stencil pointed at the `--handoff` file, when the flag is given.
//     A flag naming a missing file is an error rather than a fallback.
//  3. The resume stencil pointed at State.LastHandoff, when that file still exists.
//     A PendingHandoff is never chosen, since an aborted cycle may have left it partial.
//  4. The start stencil, for a first launch.
//
// Steps 2 to 4 are ChooseStartPrompt's own order.
//
// DecideStart maps the strand and watcher liveness pair onto the branch `start` takes:
// attach only, spawn a watcher, or relaunch.
// A dead or absent strand always relaunches.
//
// # Permission mode and subagents
//
// The orch run's spec carries `permission_mode: bypass`, the only accepted value.
// LoadConfig resolves an empty value to `bypass` and returns an error for any other, `prompt` included, naming the fix `set permission_mode: bypass or remove the key`; every verb loads the config first, so each refuses before any pane is touched.
// The spec also allows the Agent tool and forks, so the session can spawn typed subagents and forks while the fork-context `lyx webster` guard stays installed.
// Only the orch run sets the allowance, and it has no orch.yaml switch.
//
// Under `bypass`, the orch session and every subagent and fork it spawns run every tool with no permission prompt, because the orch never prompts per action.
//
// # Cycle mode
//
// `cycle_mode` in orch.yaml is `compact` or `clear`, and an absent or empty value is `compact`; any other value is a load error naming both.
// `compact` keeps the session id and the Remote Control link and writes no handoff file.
// `clear` runs the handoff, `/clear` and resume cycle.
// The trigger selection, gates and re-read under "Idle rules" are shared; the two machines follow under "The four-phase cycle (clear mode)" and "The compact cycle (compact mode)".
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
// In phase idle the watcher picks the trigger of a cycle in a fixed order:
//
//   - hard: a known context reading at or over `threshold_tokens`, the hard cap, whatever `idle_grace_s` and `soft_idle_s` are.
//   - requested: a pending cycle request.
//   - soft: a known reading at or over `soft_threshold_tokens` and below the hard cap.
//     A soft threshold at or above the hard cap never fires.
//
// The trigger is recorded in State.CycleTrigger before the cycle's first phase is entered.
// In clear mode the soft trigger sends its own handoff stencil, which offers the session a `DEFER` reply; compact mode has no `DEFER` exchange.
// A hard or requested cycle acts only when all of these hold:
//
//   - The newest event it has read is a turn end, EventStop or EventWaiting.
//   - That event was first read at least the idle grace ago.
//     The arrival time is held in memory, so a watcher restart restarts the grace.
//   - Session.SessionIdle reports an empty input box with no turn running.
//
// Every idle probe goes through one watcher helper.
// A probe that reports the pane too short to draw an input box records `orch pane too short for the idle probe; resize or use the larger client` in State.Stuck, saved and logged once, and holds cycles and notice delivery like any failing probe.
// The next probe that does not report it clears that reason and only that reason, so `lyx orch status` shows the hold in the idle phase too.
//
// A soft cycle holds the same gates with `soft_idle_s` in place of the idle grace, and adds one:
// State.LastDeferral is zero or at least `soft_idle_s` before now.
// In compact mode a hard trigger is held by the same rule, and a requested cycle never is.
//
// A hard or soft cycle also re-reads the context through the newest turn end once those gates pass, saves the new reading, and fires only if it still meets that trigger's threshold.
// The transcript can change without a turn end, so the reading saved at the last turn end can be stale.
// A re-read below the hard cap never turns a hard trigger into a soft firing on the same tick; the next tick re-evaluates from the saved reading.
// A requested cycle is not re-read, so it can cycle a small session on the operator's say.
//
// A context reading that cannot be taken is unknown and never triggers a cycle by itself.
// A fresh launch or adopt resets the reading to unknown until the new session's first turn end.
// The template hard cap is 400000 tokens, sized for a session with a context window of about one million tokens, and the template soft threshold is 300000.
//
// # The four-phase cycle (clear mode)
//
// The persisted phases are idle, handoff-requested, clearing and resuming.
// The watcher saves State before every side effect, so a restarted watcher resumes from it.
//
//   - handoff-requested: the handoff instruction is sent, naming a new timestamped file under handoffs/.
//     The phase ends once the file exists and is non-empty, a turn end has been read after the file was first seen written,
//     and the idle probe passes.
//     In a soft cycle a turn end whose message, trimmed of whitespace, is exactly `DEFER` declines the cycle:
//     the watcher re-stats the handoff file, and when it is still not written records the read time in State.LastDeferral and returns to idle with the abort reason `deferred`.
//     A written file wins over `DEFER`, and the cycle proceeds through the clear gate above.
//     In a hard or requested cycle `DEFER` is never recorded, and the phase waits for the file until the handoff timeout.
//     A restarted watcher re-sends the stencil matching State.CycleTrigger.
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
// # The compact cycle (compact mode)
//
// The persisted phases are idle and compacting.
// When a trigger fires, the watcher renders the focus stencil first, so a stencil failure changes nothing,
// then persists compacting with the trigger, clears the cycle request, types `/compact` followed by the focus, and records the injection as confirmed.
// The focus tells the summary to keep runs in flight, open parent-review forks, operator requests and decisions pending, and work half done.
// Nothing is typed unless the idle probe passed on the same tick.
// No handoff file is written, and State.LastHandoff is untouched.
//
// A compaction ends without a turn end, so the compacting phase re-reads the context every tick through State.ReadingTurnEnd, the turn end the current reading was taken through, which every stored reading records.
// The phase completes when the reading is a compaction boundary stamped at or after the phase was entered and the idle probe passes on that tick:
// the boundary's tokens become the reading, `cycle_count` increments and the watcher returns to idle.
// An earlier boundary never completes it.
// Past the handoff timeout the phase returns to idle with the abort reason `compaction timed out`, re-reads the reading, and records the time in State.LastDeferral,
// which holds the next hard or soft trigger for `soft_idle_s`; a requested cycle is not held.
// That hold delays a hard trigger by at most `soft_idle_s` per failed compaction.
//
// Restart: an unconfirmed `/compact` is typed again only when the idle probe passes and no qualifying boundary has been read.
// A qualifying boundary read after a restart completes the phase without typing anything.
//
// There is no `DEFER` handshake, so a background task's in-flight completion has no handshake protecting it if it does not survive `/compact`;
// `cycle_mode: clear` restores the handshake.
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
// # Adopting a running session
//
// To keep a session's full context instead of handing it off, read its id from `/status` in that session and exit it.
// Then run `lyx orch start --adopt <id>` from the prime.
// The session is resumed as the orchestrator strand with a watcher bound to it, and `LastHandoff` is kept.
// It resumes only a session recorded under the prime's own directory, and is refused while the orchestrator strand is live.
// It never kills or removes a strand or process orch did not launch.
// An absent or unreadable Claude session registry lets an unconfirmed holder through, so two processes can drive one session;
// the explicit id and the resume warning on the log and envelope bound that.
//
// # Residuals
//
//   - Shuttle switches Claude's prompt suggestion off in every settings file it writes, and the idle probe still fails closed on any non-empty box, so a session with a draft is never cycled until it is cleared.
//   - A SendMessage landing between the handoff turn's end and `/clear` is lost from context.
//   - The transcript and Stop-payload shapes are Claude Code internals, so usage degrades to unknown rather than failing.
//   - A threshold above the auto-compaction point lets Claude Code compact first, and the boundary it writes is a reading, so the watcher reads the shrunken context correctly.
//   - In clear mode, reed's resume path replays the launch's `--resume <session-id>`, which names the pre-clear session once a cycle has run, so a reed server rebirth resumes the older context.
//     `lyx orch stop` plus `lyx orch start` relaunches from the last completed handoff instead.
//     Compact mode keeps the session id, so it does not apply there.
//
// # Open risks
//
// The smoke suite in internal/orchcli (`go test -tags smoke -run TestSmokeOrch ./internal/orchcli/`) was run against Claude Code 2.1.287 on 2026-10-02, and all four tests passed.
// What the run showed:
//
//   - A background task survives `/clear`: its completion notification reached the resumed session, in the transcript and the pane.
//     A resumed session may therefore find its background shell still running instead of starting another.
//   - A `SendMessage` address stays stable across `/clear`: `<shortname>:orch` before and after.
//   - `--resume` with a positional prompt submitted the prompt: an adopted session answered the adopt stencil's turn and its turn end reached the events file.
//     No startup dialog stopped the resume launch; shuttle's startup probe cleared it without operator input.
//   - The idle probe passes on a live orch pane.
//     Claude draws the session name into the input box's top rule (`──── tst:orch ─`), which the probe's rule match rejected until it accepted a labelled top rule, so the probe had never passed on a named session.
//   - A visible plain run in the prime shares the orch pane's window and can squeeze it too short to draw an input box, which makes the idle probe fail and holds every cycle until the pane is tall again.
package orchengine
