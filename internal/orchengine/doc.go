// Package orchengine is the engine behind `lym orch`.
// It hosts the hub orchestrator as an interactive shuttle run in the prime worktree's reed session.
// The session is cycled before its context fills.
//
// The orchestrator is one long-lived Claude session that outlives any single context window.
// A detached watcher reads each turn end's context usage from the provider transcript.
// When usage crosses the configured threshold and the session is idle, the watcher has the session write a handoff;
// it then clears the session and resumes it from that handoff.
// A session is never cleared without a written handoff.
//
// # Verbs, and where they run
//
// `lym orch` runs from the hub's prime worktree or a beam only; every verb refuses elsewhere.
// `lym beam start` brings the orch up through `orchcli.BeamUp`, and `lym beam stop` stops it through `orchcli.BeamDown`.
// No orch verb starts it.
// A bring-up is idempotent and leaves one live orchestrator strand and one watcher bound to it.
// It never attaches a tmux client; `lym reed attach` is the only way into the session.
//
//   - status: reports the strand, the watcher and the persisted cycle state.
//   - refresh: writes a clear-cycle request, whatever the token count and `orch.cycle_mode`.
//   - distill: writes a compact-cycle request, whatever `orch.cycle_mode` says.
//     With no watcher live, both request verbs withdraw the marker again.
//   - stop: removes the orchestrator strand; the watcher notices and exits on its own.
//   - watch and resume-context: hidden verbs, not operator verbs.
//
// The verbs, the reed and shuttle wiring, and the strand-identity rules live in internal/orchcli.
// This package holds what sits behind the seam:
// the config, the stencil renders, the persisted state, the start's two decisions, and the watcher.
// It is told every path through Paths and derives none (Told-Geometry Invariant).
//
// # Start prompt order
//
// A start resumes the session State.SessionID records, with the reload pointer.
// Only a launch with no session recorded, or whose resume failed to start, picks a fresh prompt (ChooseStartPrompt):
// the resume stencil pointed at State.LastHandoff when that file exists, else the start stencil.
// A PendingHandoff is never chosen, since an aborted cycle may have left it partial.
//
// Every launch prompt is a one-line pointer to the orch role file, which RenderRoleFile renders to Paths.RolePath.
// The role is one fixed text built into the binary (`contracts/orchrole`); no board copy exists, so no hub can edit it.
// The role file carries no command list and no repository rules:
// it sends the session to `lym help index`.
// A repository's orch rules reach the session through heddle's `orch-prompt` target.
// Every delivery re-renders its file first, so a stencil edit or a new binary applies from the next delivery.
// A render failure makes a start refuse, a note request change nothing, and a clear abort the cycle.
//
// DecideStart maps the strand and watcher liveness pair onto the branch `start` takes:
// already running, spawn a watcher, or relaunch.
// A dead or absent strand always relaunches.
//
// # Permission mode and subagents
//
// The orch run's spec carries `permission_mode: bypass`, the only accepted value.
// LoadConfig resolves an empty value to `bypass` and refuses any other, so each verb refuses before any pane is touched.
// Under `bypass`, the orch session and every subagent and fork it spawns run every tool with no permission prompt.
// The spec also allows the Agent tool and forks, with the fork-context `lym webster` guard still installed.
//
// # Cycle mode
//
// `orch.cycle_mode` is `compact` or `clear`, and an absent or empty value is `compact`.
// `compact` keeps the session id and the Remote Control link; `clear` runs the handoff, `/clear` and resume cycle.
// `orch.cycle_mode` picks the mode of an automatic cycle;
// an operator request carries its own mode, recorded in State.CycleMode.
// Both modes write the note first, and neither clears or compacts before the note gate has passed.
//
// # Remote control
//
// While `orch.remote_control` is on, every orch session starts with the provider's remote control on.
// This covers every launch and resume lym makes.
// It is off unless the operator sets it, so lym never turns remote control on by itself;
// the RC button of a pane turns it on for one agent.
// The reinterpretation FlagRemoteControlDefault flags a hub file that set no value before the default turned off.
// The hub's orch hears of it once.
// Config.RemoteControlSessionName fills the `orch.remote_control_name` template;
// `{machine}::{name}` is the cross-machine address creel uses.
// orchengine records no remote-control state.
//
// # The watcher
//
// Watcher.Run holds watch.lock for its whole life, so at most one watcher runs per prime;
// a second one exits with ErrWatcherRunning.
// On SIGINT or SIGTERM the watcher records State.WatcherStopping,
// and `start` waits for a stopping watcher through WaitWatcherGone instead of reading it as live.
// Too many consecutive tick errors end the watcher, with the reason in State.WatcherExit.
// A tick saves State only while the record still names the strand it loaded,
// so a watcher never overwrites the binding a concurrent `start` recorded.
// Provider and reed interactions go through the Session seam, and deliveries into the orch go through the Creel seam.
// The watcher types into the orch pane only for the hub pause's guarded interrupt.
//
// A watcher told an OwnerWatch through WatchOwner ends every process this machine runs for the beam, the dobby excepted.
// It does so once the beam's owner is another machine.
// It then stops the orch strand and ends as done, even when the end failed.
// A foreign owner stops nothing while OwnerWatch.UnderAdopt reports an adopt hold this machine wrote in Bolt.
//
// The watcher queues the strand's palette color through creel, never into the pane.
// A color creel refused, expired or withdrew is dropped, since the color is display only.
//
// # Idle rules
//
// In phase idle the watcher picks the trigger of a cycle in a fixed order:
//
//   - hard: a known context reading at or over `orch.threshold_tokens`, the hard cap.
//   - requested: a pending cycle request.
//   - soft: a known reading at or over `orch.soft_threshold_tokens` and below the hard cap.
//     The soft trigger posts its own handoff stencil, which offers the session a `defer` reply.
//
// The trigger is recorded in State.CycleTrigger before the cycle's first phase is entered.
// A context reading that cannot be taken is unknown and never triggers a cycle by itself.
//
// The request marker is JSON holding the mode and the request time.
// A pending request at least the handoff timeout old is removed with a log entry, and nothing is acted on.
// A marker that does not parse counts as that old.
// The age excludes the time a hub pause held the watcher's cycle machine.
//
// Every writer and remover of the marker holds the orch state lock,
// and the watcher removes only the request it read.
// A marker that changed since the tick's read leaves a requested start idle.
// A cycle satisfies a same-mode request made during it, and leaves a request of the other mode pending.
//
// A hard or requested cycle acts only when all of these hold:
//
//   - The newest event read is a turn end, EventStop or EventWaiting.
//   - That event was first read at least the idle grace ago.
//     A requested cycle waits no grace, and creel's own idle check still guards the pane.
//   - Creel grants the watcher's hold on the orch strand.
//     A refusal starts no cycle on that tick and is recorded in State.Stuck.
//
// A soft cycle holds the same gates with `orch.soft_idle_s` in place of the idle grace;
// State.LastDeferral must be zero or at least `orch.soft_idle_s` before now.
// A hard or soft cycle first re-reads the context through the newest turn end.
// It fires only if the reading still meets the threshold.
// A requested cycle is not re-read.
//
// # The cycle
//
// The watcher saves State before every side effect, so a restarted watcher resumes from it.
// The persisted phases in clear mode are idle, handoff-requested, clearing and resuming;
// compact mode uses idle, handoff-requested and compacting.
//
//   - handoff-requested: the note request is posted as creel mail naming a new timestamped file under handoffs/.
//     The gate passes at a turn end, read since the phase began, with the requested note file existing and non-empty.
//     A note the orch extends later is cycled on the version present at the first qualifying turn end.
//     A `defer` reply declines a soft cycle whose note is still unwritten and sets State.LastDeferral.
//     The phase times out at the handoff timeout, counted from the orch's read of the note request.
//   - clearing: `/clear` is injected once the resume prompt has rendered; creel types it at the orch's next idle.
//   - compacting: LastHandoff moves to the note, and `/compact` is injected with a focus stencil as its argument.
//     A compaction ends without a turn end, so the phase searches the transcript for a compaction boundary.
//     A boundary after the phase was entered completes it and enters the reload sequence.
//     A boundary found while the `/compact` inject is still queued means the session compacted itself;
//     the watcher withdraws its queued items so a second compaction never follows.
//     Past the handoff timeout the phase returns to idle with `compaction timed out`.
//   - resuming: the reload sequence, ending at the resumed session's first turn end.
//   - idle: every return, completed or aborted, releases the creel hold.
//
// The guarantee holds through every restart:
// nothing can reach `/clear` or `/compact` before the named note exists, non-empty, at a turn end of the note phase.
// LastHandoff moves only when a cycle passes that gate.
// A step with a saved id has its outcome read from creel and is never sent again;
// a step with none is looked up in creel's journal and sent only when creel journaled none.
//
// # The reload sequence
//
// A compaction keeps the skills the session invoked, and `/clear` loses them;
// both lose the role.
// The sequence runs these steps, each one creel inject with the pending step persisted first:
// color, plugins, then after `/clear` a skills step with an optional retry step, then the one-line pointer.
// After a compaction the pointer follows the plugins step directly.
// Every step times out from its first queueing; a color, plugins or skills step is then skipped,
// and the pointer step returns to idle with `resume timed out`.
// A skill the provider does not know, or the model did not load, is skipped with a logged cause.
// A turn end counts for a step only if its hook-side time is after the inject's sending, in whole seconds.
//
// The sequence has three entry points:
// after `/clear`, after a compaction the watcher ran, and after an auto-compaction.
// An auto-compaction is found in idle as a main-chain compaction boundary after State.CompactionBaseline.
// It has exactly one turn end after it; the pointer then names no note.
// A fresh launch sets the baseline to the launch time, so no boundary reloads twice.
//
// The provider's session-start hook can deliver the pointer itself after a compaction:
// the hidden `lym orch resume-context` verb renders it through ResumeContext and writes the delivery mark, `resume-mark`.
// The pointer step injects nothing for a mark that carries the pending pointer's text.
// That mark must be dated at or after State.PointerMarkSince.
//
// # The .lym/orch/ layout
//
// Everything orch writes lives under the prime's `<anchor>/.lym/orch/`, declared once in internal/orchcli/paths.go.
// state.json carries the orch session's id and outlives `lym orch stop`.
// handoffs/ holds one file per cycle.
// The one State.LastHandoff names is kept, and `lym cleanup --apply` removes the rest.
//
// # The creel seam
//
// The watcher reaches creel, the hub's message and inject daemon, through the `Creel` interface.
// That interface sits beside its `Session` seam.
// `orchcli` implements it over the creelcli Go client signing as the `ORCHWATCH` program.
// The engine imports no creel package.
// Another module tells the orchestrator about a change by posting creel mail to its strand (PATTERN-reed-panes);
// it never types into the orchestrator's pane.
//
// # Delivery through creel
//
// Every step of a cycle reaches the orch as a creel post or inject,
// and the watcher holds the orch strand in creel while a cycle runs.
// The step is persisted in State.Pending before it is sent,
// so a restart adopts a journaled step instead of sending it twice.
// A creel failure, an unreachable daemon or a refusal is recorded in State.Stuck and holds the cycle;
// it never ends the watcher.
// A phase belonging to another strand is reset to idle, withdrawing the watcher's queued items to the orch strand.
// A start withdraws around a fresh launch, as the orchcli package documentation describes;
// State.FreshLaunchFailure keeps a failed release or second round, and the watcher retries it before anything else.
// A watcher that dies mid-cycle leaves a hold that lapses within `cycleHoldDuration`.
//
// # Hub pause
//
// A watcher told a PauseWatch reads the pause record on each tick; a slug entry never covers the orch.
// While a live hub entry stands, no context cycle starts or advances.
// At the entry's grace end, a busy orch is interrupted through shuttle's guarded interrupt.
// It is interrupted once more after the settle time.
// This is the one keystroke the watcher types.
// After the entry is resumed, an orch it owns gets the resume line once, posted as creel mail.
//
// # Usage limit
//
// The orch shares the account's usage limit with every session it watches.
// A usage-limited turn end records State.UsageLimitAt, State.UsageLimitResetsAt and State.UsageLimitWakeAt.
// While the limit stands, the watcher starts no cycle.
// It renews its creel hold so no other sender's mail reaches the orch.
// After the wake time it injects a line telling the orch to run `lym tender revive`, repeating until the orch answers.
// The first turn end that is not a usage limit clears the record and releases the hold.
//
// A watcher told a `WatchRestart` asks Session.SwitchAccountOnLimit first and calls OrchRestart.Resume on a switch.
// A refused resume calls OrchRestart.Fresh; a failed restart has removed the old strand and ends the watcher.
//
// # Binary re-exec
//
// A watcher told a `WatchBinary` check re-execs onto a replaced binary, only at a restart-safe tick:
// the state is in phase idle and no live hub entry stands.
// The watch lock does not survive the exec, so a start may spawn a second watcher in the gap;
// the loser exits as already running, and exactly one watcher remains.
// `lym orch watch` reaps inherited children through `proc.ReapInheritedChildren` before it starts anything.
//
// # Continuing after a reboot
//
// After a reboot, a `lym orch stop` or a dead strand, a start resumes State.SessionID's session with the reload pointer.
// A start records the id of every launch and the watcher keeps it current across `/clear`.
// State lives under the worktree's `.lym/orch/`, so the id stays on the machine that ran the session.
// A resume refused because a live process holds the session ends that process when it is this worktree's own orch.
// A holder the orch's start did not register in `internal/runproc` refuses the start.
// A resume refused because this machine holds no transcript of the session launches fresh.
// It launches from the last completed handoff, else the start stencil, and the report names the session it gave up.
// Every other failed resume fails the start, which launches nothing fresh.
// With no session recorded the start launches fresh the same way.
// A fresh orch otherwise comes only from `lym orch refresh` on a live orch.
//
// # Residuals
//
//   - Creel mail handed over between the handoff turn's end and `/clear` is lost from context like any text of that turn.
//   - The transcript and Stop-payload shapes are Claude Code internals, so usage degrades to unknown rather than failing.
//   - In clear mode, reed's resume path replays the launch's `--resume <session-id>`,
//     so after a cycle a reed server rebirth resumes the pre-clear context, and a start then resumes it too.
//
// # Open risks
//
//   - A background task survives `/clear`: its completion notification reaches the resumed session.
//   - A visible plain run in the prime shares the orch pane's window and can squeeze it too short to draw an input box.
//     The idle check then fails and holds every cycle step until the pane is tall again.
//   - The recorded session id is right only while the provider names the parent session id in a subagent's or fork's hook lines.
package orchengine
