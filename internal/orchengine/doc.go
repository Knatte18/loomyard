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
//     It leaves one live orchestrator strand and one watcher bound to it, then reports on the envelope.
//     It never attaches or switches a tmux client;
//     `lyx reed attach` is the only way into the session.
//     `--adopt <session-id>` resumes an existing Claude session as the orchestrator strand instead of launching a fresh one.
//   - status: reports the strand, the watcher and the persisted cycle state.
//   - refresh: writes a clear-cycle request, which makes the watcher write a note and clear the session at its next idle moment regardless of the token count and of `cycle_mode`.
//   - distill: the same for a compact cycle: a note, then `/compact`, whatever `cycle_mode` says.
//     Both verbs report `requested` and `watcher_live`, and with no watcher live they withdraw the marker again, so no later watcher acts on a request made while none ran.
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
// Every launch prompt is a one-line pointer: the orch's whole procedure lives in the role stencil, which RenderRoleFile renders to Paths.RolePath, and the pointer names that file.
// The resume pointer also names the orch note to resume from.
// A note request names the note path and Paths.NoteTemplatePath, the file RenderNoteTemplateFile renders from the note stencil: what the orch is doing, the agreed next step and the questions waiting on the operator.
// Every delivery re-renders its file first, so a stencil edit applies from the next delivery:
// `start` renders the role file before it launches, the watcher renders the note template before it types a note request,
// and renders the role file before it types the resume pointer after `/clear`.
// A render failure is handled where a stencil render failure is: `start` refuses, a note request changes nothing, and a clear that cannot render aborts the cycle.
//
// DecideStart maps the strand and watcher liveness pair onto the branch `start` takes:
// already running, spawn a watcher, or relaunch.
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
// `compact` keeps the session id and the Remote Control link; `clear` runs the handoff, `/clear` and resume cycle.
// `cycle_mode` picks the mode of an automatic (hard or soft) cycle; an operator request carries its own mode, `refresh` for clear and `distill` for compact, recorded in State.CycleMode.
// Both modes write the note first, and neither clears or compacts before the note gate has passed.
// The trigger selection, gates and re-read under "Idle rules" are shared; the two machines follow under "The four-phase cycle (clear mode)" and "The compact cycle (compact mode)".
//
// # The watcher
//
// Watcher.Run polls at the configured interval, calling Tick once per poll.
// It holds watch.lock for its whole life, so at most one watcher runs per prime;
// a second one exits with ErrWatcherRunning.
// On SIGINT or SIGTERM it records State.WatcherStopping before its in-flight tick finishes and before it releases the lock,
// and the next Run clears the record.
// `start` waits for a stopping watcher through WaitWatcherGone, at most three poll intervals, instead of reading the dying watcher as live;
// past the bound it reports the watcher live with `watcher_stopping: true` and a hint to run `start` again.
// A healthy live watcher records no stopping,
// so `start` never waits for it.
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
// In both modes the soft trigger sends its own handoff stencil, which offers the session a `DEFER` reply.
//
// The request marker is JSON holding the mode and the request time.
// A pending request whose age is at least the handoff timeout is removed with a log entry naming its mode and age, and nothing is acted on;
// a marker that does not parse counts as that old.
// The time is on disk, so this covers a request whose idle probe never passes and a marker a dead watcher left, across watcher restarts.
// State records the cycle's mode as CycleMode and the request time as CycleRequestedAt, zero for an automatic trigger.
//
// A hard or requested cycle acts only when all of these hold:
//
//   - The newest event it has read is a turn end, EventStop or EventWaiting.
//   - That event was first read at least the idle grace ago.
//     The arrival time is held in memory, so a watcher restart restarts the grace.
//     A requested cycle waits no grace: the operator chose the moment, and the idle probe below still guards the pane.
//   - Session.SessionIdle reports an empty input box with no turn running.
//
// Every idle probe goes through one watcher helper.
// A probe that reports the pane too short to draw an input box records `orch pane too short for the idle probe; resize or use the larger client` in State.Stuck, saved and logged once, and holds cycles and notice delivery like any failing probe.
// The next probe that does not report it clears that reason and only that reason, so `lyx orch status` shows the hold in the idle phase too.
//
// A soft cycle holds the same gates with `soft_idle_s` in place of the idle grace, and adds one:
// State.LastDeferral is zero or at least `soft_idle_s` before now.
// In compact mode a hard trigger is held by the same rule after a failed compaction, and a requested cycle never is.
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
//     A requested cycle's phase times out at the earlier of the phase entry and the request time plus the handoff timeout.
//     Compact mode runs this same phase and, once the gate passes, starts compacting instead of clearing.
//   - clearing: the resume prompt is rendered first, so a stencil failure aborts before anything is cleared.
//     Then `/clear` is typed, and the phase waits for the pane to show an idle input box.
//   - resuming: the reload sequence, described under "The reload sequence" below; its pointer is the resume prompt, and the phase ends at the resumed session's first turn end, whose context reading becomes the new one.
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
// The persisted phases are idle, handoff-requested and compacting.
// A trigger first enters handoff-requested exactly as in clear mode: the session writes a note under handoffs/, and the same gate applies.
// When the gate passes, the watcher renders the focus stencil first, so a stencil failure changes nothing,
// then moves LastHandoff to the note, persists compacting, types `/compact` followed by the focus, and records the injection as confirmed.
// A later fresh `start` therefore resumes from the newest note in either mode.
// The focus tells the summary to keep runs in flight, open parent-review forks, operator requests and decisions pending, and work half done.
// Nothing is typed unless the idle probe passed on the same tick.
//
// A compaction ends without a turn end, so the compacting phase re-reads the context every tick through State.ReadingTurnEnd, the turn end the current reading was taken through, which every stored reading records.
// The phase completes when the reading is a compaction boundary stamped at or after the phase was entered and the idle probe passes on that tick:
// the boundary's tokens become the reading, `cycle_count` increments, State.CompactionBaseline moves to the boundary,
// and the watcher renders the role file and the resume pointer naming the cycle's note and enters the reload sequence.
// A stencil failure there returns to idle with the reason.
// An earlier boundary never completes it.
// Past the handoff timeout the phase returns to idle with the abort reason `compaction timed out`, re-reads the reading, and records the time in State.LastDeferral,
// which holds the next hard or soft trigger for `soft_idle_s`; a requested cycle is not held.
// That hold delays a hard trigger by at most `soft_idle_s` per failed compaction.
//
// Restart: an unconfirmed `/compact` is typed again only when the idle probe passes and no qualifying boundary has been read.
// A qualifying boundary read after a restart completes the phase without typing anything.
//
// A soft compact cycle keeps the `DEFER` handshake, so a session with a background task in flight can decline before `/compact`.
//
// A tick saves State only while the record still names the strand it loaded, checked under the state lock,
// so a watcher never overwrites the binding a concurrent `start` just recorded.
//
// # The reload sequence
//
// A compaction keeps the skills the session invoked, which Claude Code re-injects,
// and `/clear` loses them;
// both lose the role.
// Every entry point therefore starts the sequence with a plugins step,
// so a skill deployed after the session started loads at the next reload.
// After `/clear` a skills step then loads the whole orch skill list in one turn,
// an optional retry step loads what that turn left missing,
// and the one-line pointer follows;
// after a compaction the pointer follows the plugins step directly.
// `start` and `--adopt` load the same skills through the launch spec.
// The step, the skills the retry step loads, whether the sequence skips the skills, the step's first typing time and its events offset are persisted in State (`reload_step`, `reload_retry`, `reload_skips_skills`, `reload_typed_at`, `phase_events_offset`), the offset and time at the first typing, before the text is typed.
// `reload_step` is -2 for the plugins step, 0 for the skills step and -1 for the retry step.
// Every other value, including a per-skill index persisted before the one-turn load, is read as the pointer step, as is a retry step with an empty `reload_retry` and a skills step when `reload_skips_skills` is set.
//
//   - The plugins step types `/reload-plugins`, only when the idle probe passed on the same tick, and ends no turn.
//     It persists the move to the next step, the skills step or, when `reload_skips_skills` is set, the pointer, with a zero `reload_typed_at`, and types nothing else on that tick;
//     the next step is typed on a later tick whose idle probe passed.
//     It has no confirmation and no timeout: a restart before the move finds it persisted and types `/reload-plugins` again, which is idempotent.
//   - The skills step, only after `/clear`, types one provider-built message asking the model to load every skill, only when the idle probe passed on the same tick.
//     Its first turn end after the typing is classified against the transcript.
//     A skill the provider does not know is skipped, with a log entry naming the skill and the cause `unknown`.
//     A turn whose transcript cannot be read is confirmed unverified, with one `skill load unverified` entry listing the skills,
//     and the pointer follows.
//     A skill the provider knows but the model did not load moves the sequence to the retry step.
//     When the handoff timeout, measured from the first typing, passes with no turn end, every skill is skipped with the cause `timeout` and no retry follows.
//     An empty skill list starts the sequence at the pointer step.
//   - The retry step types the same message for the missing skills only, and classifies its turn end the same way:
//     a skill still missing is skipped with the cause `not loaded`,
//     and there is no second retry.
//     The move into it persists the step and `reload_retry` in one save,
//     so a restart before the move reads the same turn end again and a restart after it never retries twice.
//   - The pointer step types State.PendingResume under the same rule and ends the phase at its first turn end.
//     Its timeout runs from its first typing and returns to idle with `resume timed out`.
//   - A step typed before a restart and not confirmed is typed again once the idle probe passes, without a fresh timeout.
//     Loading a skill twice costs one turn and changes nothing.
//
// The sequence has three entry points, each entered only on a tick whose idle probe passed:
//
//   - After `/clear`: the pointer names the note, as before.
//   - After a compaction the watcher ran: the pointer names the cycle's note.
//   - After an auto-compaction: in idle, a turn end read makes the watcher ask for a main-chain compaction boundary after State.CompactionBaseline, with the turn ends that follow it in the transcript.
//     Exactly one turn end after the boundary, and that turn end the newest one read on the tick, enters the sequence, with the `orch-template-reload` pointer: read the role file and continue the work, no note.
//     More than one turn end after the boundary means turns ran without a reload,
//     or a restarted watcher on an old cursor found an old boundary:
//     the baseline moves to the boundary, the watcher logs it at Info and types nothing.
//     One turn end after the boundary that the watcher has not read yet, or none, types nothing and leaves the baseline,
//     so a later turn end evaluates the boundary again.
//     A confirmed boundary is held in memory only until the idle probe passes,
//     and a restarted watcher finds it again at its next turn end, since the baseline has not moved.
//     A tick that read a turn end replaces the held boundary from its own evaluation alone,
//     so a boundary found stale never outlives that tick;
//     a tick that read none keeps it,
//     and binding to another strand clears it.
//     Bound: a compaction mid-turn whose turn end is followed by another before the watcher reads gets no reload,
//     and the role pointer reaches the session at its next cycle.
//
// Bound: only the two compaction entries skip the skills step and `/clear` keeps it;
// the pointer step still has the session read its role file.
// A session that did lose a skill in a compaction misses it until its next `/clear` or restart.
//
// State.CompactionBaseline is set to the launch time by a fresh launch, so a boundary an adopted session already carried never reloads,
// and moves to each handled boundary, including a compaction the watcher ran, so no boundary reloads twice.
//
// # The .lyx/orch/ layout
//
// Everything orch writes lives under the prime's `<anchor>/.lyx/orch/`, declared once in internal/orchcli/paths.go:
//
//   - state.json and state.json.lock: the persisted State and its lock.
//   - watch.lock: held for the watcher's life.
//   - start.lock: serializes `start`.
//   - cycle-request: the JSON marker `refresh` and `distill` write and the watcher consumes.
//   - watch.log: the detached watcher's stdout and stderr.
//   - handoffs/: one timestamped handoff file per cycle, all kept.
//   - notices/: one file per queued notice, removed on delivery.
//   - session-*.never: one sentinel per launch, an output file nothing ever writes.
//     A shuttle Spec needs an output file to validate, and the watcher, not shuttle's Wait, owns the session's lifetime.
//
// # Notices
//
// Another module tells the orchestrator about a change through `QueueNotice`, never by typing into its pane:
// only the orch watcher types into the orch session (PATTERN-orch-pane-single-writer), so the orchestrator keeps no watcher of its own.
//
//   - A notice is one line, written as one file under `notices/` whose name sorts by arrival (UTC time to the nanosecond, then a random suffix), so concurrent writers never share a file.
//     A line containing a newline or beginning with `/` is refused, since it would be typed as several lines or read as a command.
//   - With no strand recorded in the orch state, the notice is logged and not queued.
//   - The queue holds at most 50 notices: after a write the oldest beyond the cap are removed and logged.
//     Concurrent appenders can overshoot the cap transiently by at most one notice each.
//   - In phase idle, when `tickIdle` has not started a cycle on the tick and the idle probe passes on that tick, the watcher types the oldest notice as a turn and then removes its file.
//     It delivers one notice per tick, and a non-idle probe, a running cycle or a watcher that is down leaves the queue untouched.
//   - Delivery is at least once: a watcher that dies between typing and removing types the notice again after a restart.
//   - A notice can be delayed by a busy session, a too-short pane or a cycle, but is never typed over a draft or a running turn.
//   - `Watcher.Run` drops the queue at start when the orch state records no strand.
//
// # Migrating from an operator's own terminal session
//
// In the running terminal session, ask for a note in the orch note's three sections and note the file it writes, then exit that session.
// Then run `lyx orch start --handoff <that file>` from the prime.
// The new orchestrator strand starts from that note instead of the start stencil.
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
// The smoke suite in internal/orchcli (`go test -tags llm -run TestSmokeOrch ./internal/orchcli/`) was run against Claude Code 2.1.287 on 2026-10-02, and all four tests passed.
// What the run showed:
//
//   - A background task survives `/clear`: its completion notification reached the resumed session, in the transcript and the pane.
//     A resumed session may therefore find its background shell still running instead of starting another.
//   - A `SendMessage` address stays stable across `/clear`: `<shortname>:orch` before and after.
//   - `--resume` with a positional prompt submitted the prompt: an adopted session answered the adopt stencil's turn and its turn end reached the events file.
//     No startup dialog stopped the resume launch; shuttle's startup probe cleared it without operator input.
//   - The idle probe passes on a live orch pane.
//     Claude draws the session name into the input box's top rule (`──── tst:orch ─`), which the probe's rule match rejected until it accepted a labelled top rule, so the probe had never passed on a named session.
//   - Adopting a large session starts within `startup_timeout_s` and leaves no dialog on the pane.
//     `TestSmokeOrch_AdoptLargeSession` (opt-in through `LYX_SMOKE_ADOPT_SESSION`) adopted a copy of a hub session's transcript of 12827281 bytes on 2026-10-05, Claude Code 2.1.289.
//     Startup took 43.8s against a `startup_timeout_s` of 90, so a transcript of this size uses about half the budget.
//   - A visible plain run in the prime shares the orch pane's window and can squeeze it too short to draw an input box, which makes the idle probe fail and holds every cycle until the pane is tall again.
package orchengine
