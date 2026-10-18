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
//     It first ensures the prime's `runs` strand, the session's overview window running `lyx batten overview`, which shows the orch's context line above the runs in flight, so on a fresh session the overview is the first window and the orchestrator has a window of its own; a failure there only warns.
//     It leaves one live orchestrator strand and one watcher bound to it, then reports on the envelope.
//     It never attaches or switches a tmux client;
//     `lyx reed attach` is the only way into the session.
//     `--adopt <session-id>` resumes an existing Claude session as the orchestrator strand instead of launching a fresh one.
//     `--continue` resumes the session orch recorded for its strand.
//   - status: reports the strand, the watcher and the persisted cycle state.
//   - refresh: writes a clear-cycle request, which makes the watcher write a note and clear the session at its next idle moment regardless of the token count and of `cycle_mode`.
//   - distill: the same for a compact cycle: a note, then `/compact`, whatever `cycle_mode` says.
//     Both verbs report `requested` and `watcher_live`, and with no watcher live they withdraw the marker again, so no later watcher acts on a request made while none ran.
//   - stop: removes the orchestrator strand; the watcher notices and exits on its own.
//     The `runs` strand stays as the overview window.
//   - watch: the hidden daemon verb `start` spawns detached; it is not an operator verb.
//   - resume-context: the hidden verb the orch run's session-start hook runs after every compaction; it prints the resume pointer as the provider's additional-context JSON and writes the delivery mark.
//
// The verbs, the reed and shuttle wiring, and the strand-identity rules live in internal/orchcli.
// This package holds what sits behind the seam: the config, the stencil renders, the persisted state, `start`'s two decisions, and the watcher.
// It is told every path through Paths and derives none (Told-Geometry Invariant).
// MigrateConfig raises the module's below-floor poll_interval_ms to its floor for lyx config reconcile.
//
// # Start prompt order
//
// The launch prompt is picked in a fixed order:
//
//  1. The adopt stencil, when `--adopt` or `--continue` is given; `start` renders it directly, and both are exclusive with `--handoff`.
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
// `start` renders the role file before it launches, the watcher renders the note template before it posts a note request,
// and renders the role file before it injects `/clear`.
// A render failure is handled where a stencil render failure is: `start` refuses, a note request changes nothing, and a clear that cannot render aborts the cycle.
//
// The role file's `## Commands` section carries the operator command index, which every role-file render is handed as a string;
// a role stencil without the `command_index` marker fails the render naming the stencil.
// The role file's last section, `## This repository`, carries the `ORCHESTRATOR.md` at `Paths.RepoRulesPath`, read at every role-file render;
// an absent or whitespace-only file renders a placeholder line, any other read failure fails the render naming the file,
// and a stencil without the `repository_rules` marker fails naming the stencil.
// A role stencil without the `decision_note_title` marker fails the render naming the stencil, as for the other two.
// The watcher is told an index source through NewWatcher, calls it before each of its three role-file renders, and returns to idle with the reason when the source or the render fails.
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
//
// The orch is colored like every other strand, and the watcher queues the color through creel, never into the pane itself:
// the launch spec names the coordinator segment and leaves the color to the caller,
// and the watcher injects the color command with the strand's palette color, read from reed's strand status through Session.StrandColor, on the first tick after binding, before anything else that tick queues.
// A strand with no color queues nothing and counts as done.
// The bind color's inject id is watcher memory only, and each later tick reads its outcome:
// an injected or unconfirmed color records in State the strand and State.LaunchedAt it was queued for, as does a reload's color step.
// Binding sets the color pending only when those differ from the bound session's, so a watcher that re-execs onto a new binary queues no color for a session already showing it.
// A color creel refused, expired or withdrew is logged and dropped, records nothing, and is queued again by the next watcher that binds, since the color is display only.
//
// Each tick reads the strand's session id from shuttle's files-only reading and records it in State.SessionID when it changes, through the strand-checked save.
// The same reading's session-start time is logged as `orch: session start signal read` at Info once per new time.
// An unreadable reading, or one naming no id, leaves the recorded id.
// Every provider and reed interaction goes through the Session seam and every delivery into the orch through the Creel seam, so the state machine runs against fakes in unit tests.
// Nothing the watcher does types into the orch pane any more except the hub pause's guarded interrupt, which keeps Session.SessionIdle and Session.Interrupt.
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
// In both modes the soft trigger posts its own handoff stencil, which offers the session a `defer` reply.
//
// The request marker is JSON holding the mode and the request time.
// A pending request whose age is at least the handoff timeout is removed with a log entry naming its mode and age, and nothing is acted on;
// a marker that does not parse counts as that old.
// The time is on disk, so this covers a request whose cycle could not start, such as one creel kept refusing the hold for, and a marker a dead watcher left, across watcher restarts.
// State records the cycle's mode as CycleMode and the request time as CycleRequestedAt, zero for an automatic trigger.
//
// A cycle start clears the request pending at that moment, so a request found later was made after the cycle started.
// A same-mode request made during a cycle is satisfied by it:
// the compact cycle removes a pending compact request made no later than the boundary when the compaction completes,
// and the clear cycle removes a pending clear request when its `/clear` is confirmed typed; each logs the mode and request time.
// A request of the other mode, and a compact request made after the boundary, stay pending for the idle phase.
// Bound: it removes only a request made after the cycle started and before its action took effect, for the same thing the action did;
// the request time is CLI-written and the boundary time is the transcript's, both wall clock on one host.
//
// A hard or requested cycle acts only when all of these hold:
//
//   - The newest event it has read is a turn end, EventStop or EventWaiting.
//   - That event was first read at least the idle grace ago.
//     The arrival time is held in memory, so a watcher restart restarts the grace.
//     A requested cycle waits no grace: the operator chose the moment, and creel's own idle check still guards the pane.
//   - Creel grants the watcher's hold on the orch strand, taken through renewHold just before the phase is entered.
//     A refusal, or a failed call, starts no cycle on that tick: it is recorded in State.Stuck and logged once per change, and the next tick tries again.
//
// There is no idle probe before typing, because the watcher types nothing: creel types each step at the orch's next idle, and its own idle check, the readiness reading of the session's hooks and pane, gates every delivery.
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
//   - handoff-requested: the note request is posted as creel mail to the orch strand, naming a new timestamped file under handoffs/ and the template the note follows.
//     Creel assigns the mail's id when it is journaled, so the body tells the orch to reply to the id `lyx creel read` prints beside it, and the watcher saves the id the post answers in State.RequestID.
//     The orch answers with `lyx creel post --reply <id> "written <path>"` once the note is written, or, to a soft request only, `lyx creel post --reply <id> defer`.
//     Each tick reads the watcher's own mailbox without waiting, through readReplies, which runs before the client acknowledges the read, so a failed save leaves the reply unread for the next watcher.
//     Only a reply from the orch strand whose reply_to is State.RequestID counts; every other message, a reply to an earlier request included, is logged and dropped.
//     A `written` body naming the requested note, resolved against the prime, with the file existing and non-empty, ends the phase, and the move to clearing, or to compacting in compact mode, is persisted inside the handler.
//     A reply naming another path, carrying other text, or arriving with a missing or empty file is logged and leaves the phase waiting until the handoff timeout.
//     A `defer` body declines a soft cycle whose note is still unwritten: the watcher records the read time in State.LastDeferral and returns to idle with the abort reason `deferred`.
//     In a hard or requested cycle, or once the note is written, `defer` is logged and ignored.
//     A restarted watcher whose post was persisted with no id looks it up in creel's journal, and posts the stencil matching State.CycleTrigger again only when creel journaled none.
//     A requested cycle's phase times out at the earlier of the phase entry and the request time plus the handoff timeout, and a session that asks a question during the note aborts it.
//   - clearing: the resume prompt is rendered first, so a stencil failure aborts before anything is cleared.
//     Then `/clear` is injected as a command, and the phase waits for creel to type it, however long the orch stays busy, with the stall creel reports shown in State.Stuck.
//     An injected or unconfirmed `/clear` moves on to resuming, a withdrawn one returns to idle.
//   - resuming: the reload sequence, described under "The reload sequence" below; its pointer is the resume prompt, and the phase ends at the resumed session's first turn end, whose context reading becomes the new one.
//   - idle: every return, completed or aborted, persists the events position read through and releases the creel hold.
//
// Compact mode runs the same note phase and, once the gate passes, starts compacting instead of clearing.
// The guarantee holds through every restart: nothing before the note is written and named in a reply to the pending request can reach `/clear`, and LastHandoff moves only when a cycle passes that gate.
//
// Restart rules: a non-idle phase's pending step is read from State.Pending.
// A step with an id has its outcome read from creel, and a restarted watcher posts or injects nothing again.
// A step with no id is looked up in creel's journal and adopted when found, and sent once more only when creel journaled none.
// A phase belonging to another strand is reset to idle (see "Delivery through creel").
// A handoff-requested or resuming phase that exceeds the handoff timeout aborts back to idle with LastAbortReason set.
//
// # The compact cycle (compact mode)
//
// The persisted phases are idle, handoff-requested and compacting.
// A trigger first enters handoff-requested exactly as in clear mode: the session writes a note under handoffs/, and the same gate applies.
// When the gate passes, the watcher renders the focus stencil first, so a stencil failure changes nothing and leaves the reply unread.
// It then moves LastHandoff to the note, persists compacting, and injects `/compact` with the focus as its argument.
// A later fresh `start` therefore resumes from the newest note in either mode.
// The focus tells the summary to keep runs in flight, open parent-review forks, operator requests and decisions pending, and work half done.
//
// A compaction ends without a turn end, so the compacting phase searches the transcript of State.ReadingTurnEnd, the turn end the current reading was taken through, which every stored reading records, for a compaction boundary every tick.
// The boundary is searched for rather than read off the newest entry, because a message answered right after the boundary hides it there.
// The phase completes when a boundary after the phase was entered is found, whatever turn ends follow it:
// the transcript's newest reading becomes the reading, `cycle_count` increments, State.CompactionBaseline moves to the boundary,
// and the watcher renders the role file and the resume pointer naming the cycle's note and enters the reload sequence.
// A stencil failure there returns to idle with the reason.
// A boundary at or before the phase entry never completes it.
// Past the handoff timeout the phase returns to idle with the abort reason `compaction timed out`, re-reads the reading, and records the time in State.LastDeferral,
// which holds the next hard or soft trigger for `soft_idle_s`; a requested cycle is not held.
// That hold delays a hard trigger by at most `soft_idle_s` per failed compaction.
//
// The `/compact` is injected once.
// An expired inject is queued again, and an unconfirmed one keeps the search going for the boundary until the timeout.
// Restart: a saved `/compact` has its outcome read and is never sent again, and one with no saved id is looked up in creel's journal first.
// A qualifying boundary found after a restart completes the phase without sending anything.
//
// A soft compact cycle keeps the `defer` handshake, so a session with a background task in flight can decline before `/compact`.
//
// A tick saves State only while the record still names the strand it loaded, checked under the state lock,
// so a watcher never overwrites the binding a concurrent `start` just recorded.
//
// # The reload sequence
//
// A compaction keeps the skills the session invoked, which Claude Code re-injects,
// and `/clear` loses them;
// both lose the role.
// Every entry point therefore starts the sequence with a color step, which restores the orch's color a `/clear` or compaction reload drops,
// and a plugins step after it,
// so a skill deployed after the session started loads at the next reload.
// After `/clear` a skills step then loads the whole orch skill list in one turn,
// an optional retry step loads what that turn left missing,
// and the one-line pointer follows;
// after a compaction the pointer follows the plugins step directly.
// `start` and `--adopt` load the same skills through the launch spec.
// The step, the skills the retry step loads, whether the sequence skips the skills, the step's first queueing time and its events offset are persisted in State (`reload_step`, `reload_retry`, `reload_skips_skills`, `reload_typed_at`, `phase_events_offset`), the offset and time at the first queueing, before the step is sent.
// `reload_step` is -3 for the color step, -2 for the plugins step, 0 for the skills step and -1 for the retry step.
// Every other value, including a per-skill index persisted before the one-turn load, is read as the pointer step, as is a retry step with an empty `reload_retry` and a skills step when `reload_skips_skills` is set.
// A state persisted mid-reload by a watcher that had no color step resumes where it was.
//
// Every step is one creel inject, queued with the pending step persisted first.
// The color, plugins and skills steps are command injects (`color` with the strand's palette color, `reload-plugins`, `load-skills` with the space-joined skill list), and the pointer is a text inject.
// Every step times out from its first queueing, `reload_typed_at`, which is kept across every requeue and restart:
// past the handoff timeout the color and plugins steps are logged as skipped and the sequence moves on, as a skills step does.
//
//   - The color step injects the strand's color, and ends no turn.
//     A strand with no color, or a color reed cannot read, queues nothing: the step moves on to the plugins step at once and the bind color counts as done.
//     An injected or unconfirmed color moves the sequence to the plugins step, recording the strand's color as typed for the session.
//   - The plugins step injects `/reload-plugins`, and ends no turn.
//     An injected or unconfirmed one moves the sequence to the next step, the skills step or, when `reload_skips_skills` is set, the pointer.
//   - The skills step, only after `/clear`, injects one provider-built message asking the model to load every skill.
//     Its first turn end after the inject's newest sending record is classified against the transcript.
//     A skill the provider does not know is skipped, with a log entry naming the skill and the cause `unknown`.
//     A turn whose transcript cannot be read is confirmed unverified, with one `skill load unverified` entry listing the skills,
//     and the pointer follows.
//     A skill the provider knows but the model did not load moves the sequence to the retry step.
//     When the handoff timeout, measured from the first queueing, passes with no such turn end, every skill is skipped with the cause `timeout` and no retry follows.
//     An empty skill list starts the sequence at the pointer step.
//   - The retry step injects the same message for the missing skills only, and classifies its turn end the same way:
//     a skill still missing is skipped with the cause `not loaded`,
//     and there is no second retry.
//     The move into it persists the step and `reload_retry` in one save,
//     so a restart after it never retries twice.
//   - The pointer step injects State.PendingResume and ends the phase at its first turn end after the inject's newest sending record.
//     Its timeout runs from its first queueing and returns to idle with `resume timed out`; an unconfirmed pointer keeps waiting for its turn end until then.
//   - A restart in any step reads the step's own pending inject, adopting it from creel's journal when its id was never saved, and never an earlier step's.
//     A step creel expired is queued again without a fresh timeout.
//     Loading a skill twice costs one turn and changes nothing.
//
// A turn end counts for a skills, retry or pointer step only when its hook-side time, whole seconds, is after the second of the step's inject's newest sending record, and a turn end with no time never counts.
// The watcher keeps the turn ends it read since the step began, so a turn end read in an earlier tick than the outcome still counts.
// Bound: the stamp has whole-second precision, so a step whose own turn ends within the second it was typed is missed and times out,
// and another turn ending in the second after the inject's newest sending record still counts, a window creel's idle check before sending keeps narrow.
//
// Each step creel typed logs `orch: injection typed` at Info with the phase, the step name and the time since the phase began.
// A boundary found is logged as `orch: compaction boundary found` with its time, when a fresh one is read at a turn end and again when a qualifying one completes a requested compaction,
// so the delay from boundary to reload is readable from `watch.log` alone.
//
// The sequence has three entry points:
//
//   - After `/clear`: the pointer names the note, as before.
//   - After a compaction the watcher ran: the pointer names the cycle's note.
//   - After an auto-compaction: in idle, a turn end read makes the watcher ask for a main-chain compaction boundary after State.CompactionBaseline, with the turn ends that follow it in the transcript.
//     Exactly one turn end after the boundary, and that turn end the newest one read on the tick, enters the sequence, with the `orch-template-reload` pointer: read the role file and continue the work, no note.
//     More than one turn end after the boundary means turns ran without a reload,
//     or a restarted watcher on an old cursor found an old boundary:
//     the baseline moves to the boundary, the watcher logs it at Info and queues nothing.
//     One turn end after the boundary that the watcher has not read yet, or none, queues nothing and leaves the baseline,
//     so a later turn end evaluates the boundary again.
//     A confirmed boundary is acted on at once by queueing the reload's first inject once the creel hold is granted, and is held in memory only until then;
//     a restarted watcher finds it again at its next turn end, since the baseline has not moved.
//     A tick that read a turn end replaces the held boundary from its own evaluation alone,
//     so a boundary found stale never outlives that tick;
//     a tick that read none keeps it,
//     and binding to another strand clears it.
//     Bound: a compaction mid-turn whose turn end is followed by another before the watcher reads gets no reload,
//     and the role pointer reaches the session at its next cycle.
//
// The provider's session-start hook can deliver the pointer itself after a compaction:
// the hidden `lyx orch resume-context` verb renders it from the persisted state through ResumeContext, which renders the role file first in every phase but a resuming one with a pending pointer, and then writes the delivery mark, `resume-mark`.
// The pointer step reads the mark first.
// A mark dated at or after State.CompactionBaseline whose text equals the pending pointer means the hook delivered it, so the step injects nothing, logs `orch: pointer delivered by the session-start hook` at Info, and ends the phase at once with no abort reason.
// It does not wait for a turn end, since the one that revealed the boundary was read before the step, and a skip waiting for another could end in `resume timed out`.
// No mark, an older one or one with other text injects the pointer as before, so a hook that never fired costs nothing; a pointer delivered by both ways costs the session one redundant read.
// Every return to idle clears the mark, so it never carries into the next compaction.
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
//   - state.json and state.json.lock: the persisted State and its lock; the state carries the orch session's id and outlives `lyx orch stop`.
//   - watch.lock: held for the watcher's life.
//   - start.lock: serializes `start`.
//   - cycle-request: the JSON marker `refresh` and `distill` write and the watcher consumes.
//   - resume-mark: the JSON delivery mark `resume-context` writes after a successful render and the pointer step reads; removed when the reload ends.
//   - watch.log: the detached watcher's stdout and stderr.
//   - handoffs/: one timestamped handoff file per cycle.
//     The one State.LastHandoff names is kept, and `lyx orch cleanup --apply` removes the rest.
//   - notices/: the retired notice queue, which nothing writes now; `lyx orch cleanup` lists it beside the prime's retired `.lyx/notices.log` under `retired` and `--apply` removes both.
//   - session-*.never: one sentinel per launch, an output file nothing ever writes.
//     A shuttle Spec needs an output file to validate, and the watcher, not shuttle's Wait, owns the session's lifetime.
//
// # The creel seam
//
// The watcher reaches creel, the hub's message and inject daemon, through the `Creel` interface beside its `Session` seam, and `orchcli` implements it over the creelcli Go client signing as the `ORCHWATCH` program.
// The engine imports no creel package: `CreelMessage`, `CreelStep`, `CreelOutcome`, `CreelSent` and `CreelWithdrawn` carry the values it needs, and every call addresses a receiver by its full strand name.
// The watcher reads that name from reed through `Session.StrandName` at each call, so a strand renamed or rebound never leaves it addressing a stale name.
//
//   - Another module tells the orchestrator about a change by posting creel mail to its strand, never by typing into its pane (PATTERN-orch-pane-single-writer); there is no notice queue.
//   - `Watcher.Run` reads the watcher's mailbox once before its first tick, so creel knows the program as a receiver before a reply can arrive.
//     It uses the handler every tick's read uses, which declines until the watcher has set its events cursor for the recorded strand, so every message stays unread for the first tick; a failed read is logged and is not a tick error.
//   - The retired notice queue is only cleaned up: `CleanupRetiredNotices` lists and removes `Paths.NoticesDir` and `Paths.RetiredNoticeLogPath`.
//
// # Delivery through creel
//
// Every step of a cycle reaches the orch as a creel post or inject, and the watcher holds the orch strand in creel while a cycle runs.
//
//   - Before each post or inject, `sendStep` persists the step in State.Pending (whether it is mail, its command and argument or text, and the time it was persisted) with no id, then saves the id creel answers.
//     A step is queued only after the tick re-read the state file and found its phase still non-idle on its strand; a re-read that finds the phase idle or bound to another strand queues nothing on that tick.
//     Every step inject carries `stepInjectTTL`, ten minutes, creel's default inject time to live.
//   - `stepOutcome` reads the pending inject's outcome from creel without blocking the tick.
//     `injected` advances the step.
//     `expired` queues the same step again and records the stall reason creel reported in State.Stuck.
//     `unconfirmed` advances `/clear`, the color and plugins steps and the bind color, and leaves a step that waits on its own signal, a turn end or a compaction boundary, waiting until its timeout.
//     `withdrawn` queues nothing again and returns to idle, releasing the hold.
//   - A pending step with no id, left by a restart or by a failed post or inject call of a live watcher, is looked up by `adoptPending` before anything is sent:
//     creel may have journaled a request whose call then failed, so the lookup adopts only a post or inject sent since the step was persisted whose command and argument, or text, match the step, and the step is sent only when none exists.
//   - The hold is taken on entering any non-idle phase, a cycle start and the auto-compaction reload's resuming phase alike, for `cycleHoldDuration`, and renewed on every tick outside idle before the pause check, so a tick held by a hub pause renews it too.
//     It is released at every return to idle.
//     A restarted watcher in a non-idle phase takes the hold again before anything else.
//     Bound: a watcher that dies mid-cycle leaves a hold that lapses within `cycleHoldDuration`;
//     a live cycle keeps the orch's other mail unreminded until it returns to idle, which the phase timeouts bound everywhere but the clearing phase, where `lyx orch status` shows the stall.
//   - A failed creel call, an unreachable daemon, a refusal or the sender-resolution refusal, goes through `creelFailed`:
//     it is logged once per change of error and recorded in State.Stuck with State.StuckByCreel set, and the tick returns no error, so a creel outage holds the cycle and never ends the watcher.
//     A failed release at a return to idle does not keep the phase: the watcher returns to idle, and the hold it could not release lapses.
//     A tick in which every creel call succeeded clears State.Stuck and State.StuckByCreel when the flag is set, in idle as in any other phase.
//     A stall reason `stepOutcome` wrote sets no StuckByCreel and clears when the step advances; a restarted watcher's first tick clears a Stuck that neither a pending step, the flag nor State.FreshLaunchFailure owns.
//   - A phase belonging to another strand is reset by `resetForOtherStrand`:
//     it withdraws the watcher's queued injects and unread mail to the orch strand, releases its hold and drops its pending ids,
//     and `ClearResumeNoticed` clears, under the pause record's lock, the resume-noticed mark of each pause entry owning the strand whose resume line the withdraw removed, so a continued session gets the line posted again.
//     A failed withdraw or release saves no reset and leaves the watcher unbound, so the next tick runs the reset again.
//     A `ClearResumeNoticed` failure after the withdraw removed the resume line keeps the line in State.ResumeLineWithdrawn, so the next tick's reset still clears the mark.
//   - `lyx orch start` withdraws around a fresh launch, as the orchcli package documentation describes, and records a failed release or second round in State.FreshLaunchFailure.
//     While it is set, `retryFreshLaunch` runs first on every tick, after the events read and before the hold, the mailbox and the bind color:
//     it withdraws and releases on the orch strand, signing as the watcher's own program, and posts and queues nothing, starts no cycle and renews no hold until both succeed.
//     A failing retry rewrites State.Stuck with the failure and leaves State.StuckByCreel unset, so no successful creel call clears it and a restarted watcher keeps it while the field is set.
//     The tick in which both succeed clears State.FreshLaunchFailure and State.Stuck in one save and hands a withdrawn resume line to `ClearResumeNoticed`.
//     A `ClearResumeNoticed` failure counts as a retry failure; the removed resume line is kept in State.ResumeLineWithdrawn until the clear succeeds, because a later withdraw finds nothing.
//     Bound: while the second withdraw has failed and no retry has succeeded, an old watcher's inject queued between the two rounds, a `/clear` or `/compact` included, is typed on the relaunched session's first idle probe within its ten-minute time to live, and old mail it posted is reminded;
//     a release that failed leaves the old watcher's hold on the relaunched orch until a retry succeeds or the hold lapses.
//
// # Hub pause
//
// The watcher alone covers the orch under a hub pause entry, and its guarded interrupt is the one keystroke it still sends here.
// A watcher told a PauseWatch (the pause record path and a process liveness check) reads the record after each tick's events read and the renewal of its creel hold, and before the phase switch.
// A slug entry never covers the orch.
//
//   - While a live hub entry stands the tick returns there: no context cycle starts or advances, however long the pause lasts, and a cycle already running keeps renewing its hold.
//   - Each live hub entry is acted on once, remembered in memory by its PausedAt.
//     Nothing is interrupted or recorded for an entry the orch itself ran, one that already owns the orch's session, one that began at or before State.LaunchedAt (a zero LaunchedAt reads as launched before every entry), or one whose grace end lay before the watcher's first tick.
//   - Before the grace end, an orch the idle probe reads idle with no network operation marker naming the orch is left alone; any other waits.
//   - At the grace end, a busy orch is recorded in the entry under the record lock and interrupted once through shuttle's guarded interrupt.
//     A later tick at least the provider's settle time after it re-probes and, when the orch still reads busy, sends the guarded second and last key.
//     A guarded refusal reads as an idle orch, with one Info line; an open rewind picker gets one Warn line and no further key.
//     An entry resumed before that point or during the settle, one a live entry already owns the orch in, and a watcher restarted during the settle send no key.
//   - After the entry is resumed, an orch it owns gets the resume line once, posted as creel mail to the orch strand's name, which creel reminds once the pause is lifted, then marked noticed in the record.
//     Mail has no TTL, so the line cannot expire while the orch stays busy, and the mark follows the post, so a failed post is posted again on the next tick.
//     An orch the entry does not own, such as an idle one or the caller, gets none.
//   - An unreadable record holds nothing, with one Warn per change of error.
//
// # Binary re-exec
//
// A watcher told a check through `WatchBinary`, which `lyx orch watch` wires to a `selfexec` watcher, re-execs onto a replaced binary.
// `Watcher.Run` calls the check after a tick that succeeded and returned not done, before the poll sleep, so it adds no wake of its own and runs at most once per poll.
// It runs only at a restart-safe tick: the state, re-read from disk after the tick, is in phase idle, so no cycle step is in flight and the persisted cycle state carries over,
// and the told pause record reads with no live hub entry, handled or not, so no in-memory pause handling is lost.
// A watcher never told a pause record reads no entry, and a state or record read error waits for the next tick.
//
// The exec keeps the argv and environment, and the watch lock does not survive it, since the lock file is opened close-on-exec.
// The new image takes the lock again through the acquire retry.
// In that gap `lyx orch start` may spawn a second watcher; both contend for the one lock and the loser exits as already running, so exactly one watcher remains.
// A refresh, distill, hub pause or revive in the gap reports no live watcher, as it does with none.
// The new image holds the cycle machine under a live hub entry from its first tick, and interrupts a busy orch only for an entry whose grace end is still ahead.
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
// # Continuing after a reboot
//
// `lyx orch start --continue` resumes the session State.SessionID records, through the adopt branch, after a reboot or a dead strand.
// `start` records the id of every fresh launch and the watcher keeps it current across `/clear`, so it names the context the orchestrator last ran.
// A plain `start` still relaunches from the last completed handoff, and a fresh launch that replaces a recorded session names it in the envelope hint.
// `--continue` is refused alongside `--adopt` or `--handoff`, while the orchestrator strand is live, and with no session recorded, each before anything is removed or launched.
// `lyx orch status` shows the recorded id as `session_id`.
//
// # Residuals
//
//   - Shuttle switches Claude's prompt suggestion off in every settings file it writes, and the idle check creel delivers behind still fails closed on any non-empty box other than the provider's placeholder, so no cycle step is typed over a draft until it is cleared.
//   - A SendMessage landing between the handoff turn's end and `/clear` is lost from context.
//   - The transcript and Stop-payload shapes are Claude Code internals, so usage degrades to unknown rather than failing.
//   - A threshold above the auto-compaction point lets Claude Code compact first, and the boundary it writes is a reading, so the watcher reads the shrunken context correctly.
//   - In clear mode, reed's resume path replays the launch's `--resume <session-id>`, which names the pre-clear session once a cycle has run, so a reed server rebirth resumes the older context.
//     `lyx orch stop` plus `lyx orch start` relaunches from the last completed handoff instead.
//     Compact mode keeps the session id, so it does not apply there.
//     The watcher bound to a reborn strand records the replayed pre-clear id, so `--continue` then resumes the older context too, and the post-clear id is recoverable only from the provider's session list for `--adopt`.
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
//   - A visible plain run in the prime shares the orch pane's window and can squeeze it too short to draw an input box, which makes the idle check fail and holds every cycle step until the pane is tall again.
//   - The recorded session id is right only while the provider names the parent session id in a subagent's or fork's hook lines.
//     The orch strand runs with `ForkSubagents` and the claude engine takes each hook line's `session_id` as given,
//     so a provider that names the subagent's own id there records the subagent's session.
package orchengine
