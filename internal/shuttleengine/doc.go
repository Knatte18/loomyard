// Package shuttleengine runs one LLM agent as an interactive session and returns its result.
// Callers invoke it once per spawn to run a producer, handler or progress-judge and get its output files back.
// shuttle owns which provider (via an engine), the prompt envelope, and what "done" means.
// It does not own panes, layout, or tmux mechanics: it asks reed for a strand and drives the LLM in it.
//
// Every agent runs as an interactive tmux session, never headless `claude -p`;
// the constraint is economic (subscription coverage), not technical.
// This is why the whole proc -> reed -> shuttle stack exists instead of a plain headless exec.
//
// The only channel in and out of a run is files:
// the prompt is handed to the provider as the launch argument (never typed into a live pane),
// and the agent writes its structured result to a file the caller reads.
//
// # Shuttle Provider-Seam Invariant
//
// The verdict/output contract is provider-invariant, which is what makes engines swappable.
// shuttleengine defines the Engine interface and its value types, and never imports a concrete engine implementation.
// Concrete engines import shuttleengine, not the reverse.
// Only internal/shuttleengines and an engine's own directory import a concrete engine.
// Nothing here names a tmux command or a Claude specific:
// panes are reed's vocabulary, reached only through ReedOps, and provider grammar is the Engine's.
// seam_enforcement_test.go enforces the import rule module-wide, in TestProviderSeamImportRule.
// UserSetupSource is the seam through which one engine hands another the user's Claude setup;
// only internal/shuttleengines wires a concrete source to it.
//
// # Engines
//
// An engine is a per-LLM adapter that launches and drives its provider as a tmux session.
// A Runner takes an EngineSet, which internal/shuttleengines builds from shuttle.yaml's `engine:` key.
// Start and Open resolve the choice's Engine, an empty one meaning the set's default, and record the name in run.json.
// Every other engine-consulting operation resolves the engine its run's record names;
// an unregistered name is refused, except in stop and teardown, which resolve no engine.
// Every engine's Prepare runs CheckPromptEngine, which refuses a spec whose prompt engine is another engine's or unset.
// A spec marked HandWrittenPrompt, which only `lym shuttle run` sets, passes.
// Optional engine capabilities extend the Engine interface, and Spec.Requires lists what a run requires of its engine.
// A refusal wraps ErrRequirementsUnmet; ModelKey.Wrap makes the message name the config key the model came from.
//
// # Geometry
//
// shuttle is told its anchor path and worktree root as plain strings at Runner construction, and derives neither.
// NewRunner serves hub geometry, where the anchor is the worktree root or a subdirectory of it.
// NewDetachedRunner serves standalone geometry, where the anchor is outside the worktree root it names.
// Construction validates the pair, and every public method refuses on an unusable one,
// so a swapped or relative pair fails loudly instead of succeeding against the wrong tree.
//
// # Structure
//
// The pure, hermetic half derives nothing and spawns nothing:
// the config module (shuttle.yaml), the run Spec and its validation;
// the run directory and run.json state, and the age-guarded orphan sweep.
// shuttle.yaml carries no provider key but the neutral `deny_python`; each engine's own config module holds the rest.
// The run-loop half drives a live agent through the ReedOps seam.
// It is Runner/Run in run.go, Wait in wait.go, and Attach in attach.go.
//
// # Run outcomes
//
// A turn end without every output file never ends a run: Wait holds it and keeps polling the same agent.
// Runner.SetReplyProbe tells a Runner how to ask whether a strand awaits the reply to a question it asked.
// While the run's strand awaits one, a turn end that would end the run or start its gate's evaluation is held,
// so the reply starts a turn in a live session; the hold sends no notice.
// The hold ends once no reply is awaited on two probes in a row and the agent's pane reads idle.
// A run ends only as done, died, timeout, api-error or a mechanism failure.
// A hold never extends the run's deadline, and each Attach starts a fresh deadline.
// RunState.Outcome records whether a run ever ended, so "has this run already ended?" is a fact on disk.
// The run deadline counts awake, unpaused time, and no time a skill-load turn is held on an API error.
// A timed-out run keeps its strand, since its agent may still be live.
// A spec naming the hidden anchor is refused before any strand exists, since reed starts no command for it.
//
// # Notices
//
// Runner.SetNotifier gives a Runner an optional one-line notifier; nil, the default, holds silently.
// An autonomous run (Spec.Interactive false) sends the parent a notice per held turn end;
// a second for a permission or input dialog lym cannot answer, and a third for an edit loop.
// A start whose prompt is still held busy after its last send notices the parent once, naming the strand.
// An interactive run, a runner with no notifier and Spec.QuietHold hold the turn end silently.
// Agent-written text in a notice is delimited, cut, and stripped of control characters.
// A notice is never typed into a pane.
// A notice changes no outcome, and a notifier error is logged and never ends the run.
// RunState.NotifiedOffset and the other persisted offsets keep a reattach from repeating a notice.
// A turn end is not notified when the parent has nothing to answer:
// a usage-limited one; one built from a waiting turn end with RequirementBackgroundWake;
// and one an operator instruction began.
// After the last of these the agent is typed one line per run naming its missing output files.
// That holds for every autonomous run, with or without a notifier.
// A turn end that is an API error across a detected sleep is first given one automatic continue line.
// A run with Spec.APIErrorRetries re-sends such a turn after a back-off, then ends OutcomeAPIError.
// A turn end the engine marks ModelRejected ends the run OutcomeAPIError at once.
//
// # Pause
//
// Runner.SetPause tells the runner a pauserecord.Geometry; a runner never told one reads every target uncovered.
// A start waits at entry while a pause covers the geometry's slug, and starts nothing while held.
// A line typed into a session a pause covers is refused with a *PausedError, which wraps ErrPaused, and nothing is typed.
// Keys are not lines: an interrupt, an inject and the pause's own Esc are delivered under a pause.
// While a pause covers a run, Wait skips the deadline check, sends no notice and evaluates no gate.
// The first uncovered tick evaluates a gated arrival as if it had just arrived.
//
// # Relaunch under another account
//
// An engine whose AccountSwitcher takes another account relaunches the run's session under it instead of holding.
// A relaunch keeps the run's run directory, strand and GUID, and changes only the process behind the strand.
// The record carries RunState.RelaunchingAt while it happens, and an attach reads such a record as live within its bound.
// A failure of any relaunch step ends the run as a strand that died.
//
// # Opened and unawaited sessions
//
// Runner.Open opens a named interactive session with no prompt in a window of its own.
// It needs the SessionOpener capability, writes no run directory or run.json, and is refused while a pause covers it.
// Runner.StartSession starts a session that carries the rendered prompt and is never awaited.
// The Spec sets Unawaited and names no output file, and the caller stops it through StopStrand.
// The run record reads running until a stop ends it.
// A Spec that also sets EndsWithSession needs no stop:
// as an opened session does, its strand removes itself when the provider exits, and a resume drops it.
// A Spec with ResumeSessionID resumes that session through the engine's resume capability.
//
// # Stop verb
//
// Run.Stop and Runner.StopStrand remove a run's strand after recording the stop in run.json.
// The record reads "stopped", a record-only terminal outcome that is never a Result.Outcome.
// A record already terminal keeps its outcome,
// and a record still reading running whose output files all exist keeps running, so a stop never hides finished work.
// The record is written first; a failed write leaves the strand untouched.
// Runner.StopStepRuns and Runner.StopOrphanedStepRuns apply the same record-then-remove order to every run of a step.
// They record each stop as interrupted, since the step ended before the run's work did.
// A run whose agent cannot be addressed is stopped only when its engine proves no process holds its session;
// otherwise it stays running and is listed under StepStopReport.Held.
//
// # Resuming an interrupted session
//
// A start resumes the session of its role's newest earlier run.
// It does so when that run declares the same output files and its session ended before its work did.
// A crash, a reboot or a killed pane ends a session that way.
// Such a run died, reads running with its waiter gone, or carries a stop recorded as interrupted.
// The resume needs the engine's SessionResumer capability.
// A resume check that refuses, as for a missing transcript, starts fresh.
// The resumed session takes no skill-load turn, and its prompt opens with a line telling it to continue its work.
// Once a start of those output files is up, the earlier run is set aside and never resumed again.
// Runner.RetireInterruptedRuns sets aside every interrupted run, for a deliberate restart such as a goto.
// A run that timed out, ended on API errors or was stopped on purpose is never resumed.
//
// # Attach
//
// Runner.Attach answers one question:
// is there a still-live-or-already-finished, never-terminated run for this exact output-file set?
// If so, it waits on that run instead of starting a second agent; a not-found answer is the caller's cue to start one.
// The answer rests on the persisted RunState.Outcome, then the run's file contract, then reed's live-agent evidence.
// Output-file existence alone is not evidence:
// a bounce that re-runs a producer over present files looks like a finished run.
// An absent or unreadable reed state file and a Status() failure are errors, never found == false.
// Attach reconstructs its *Run without calling Start, so no orphan sweep runs there;
// a caller must probe Attach before anything else that could sweep the directory it is looking for.
// Attach and AttachGated first remove the live strand of an earlier run with the same output-file set.
// Only a strand the probe judged respawn-eligible is removed; no other strand is touched.
// Runner.AttachIfLive is the same probe with the removal off.
//
// # Provider-neutral done-when list
//
// An ungated run's strand is registered with its output files as reed's done-when list,
// so reed's resume drops a finished agent strand instead of reviving it.
// A gated run's strand carries none, since its outputs existing does not mean its gate passed.
// An opened session and a run whose Spec sets EndsWithSession carry the exit marker their pane line writes.
//
// # Skill loading
//
// Spec.Skills names provider-neutral skills.
// A spec with a ConfigRole has that role's agents.yaml list put ahead of them.
// shuttle loads them into a fresh session in one turn before the prompt, which needs the SkillLoader capability.
// A skipped skill never fails the launch; only a load turn still open at the run's deadline does.
// A load turn that ends on an API error skips nothing: the launch holds with a back-off and types the load message again.
// The hold's loadHoldMaxAPIErrors-th API error ends the start as api-error, with Result.APIError naming the error.
// That typed turn is the only probe of the API a session has,
// so the prompt line is never typed into a session the API failed.
//
// # Pending prompt
//
// A launch that defers its prompt to a typed line (Launch.PromptLine) records the line in run.json as `promptLine`.
// Run.DeliverPendingPrompt types it through the guarded paste-submit.
// It does so when no turn start carries it past RunState.PromptOffset,
// so a step that died between its skill-load turns and its prompt delivery still gets its task.
// The run's events offset is the events file's size just before the prompt is sent;
// readers that replay the events file without Waiting, such as Attach, start there.
//
// # Sending
//
// Every send into a session is verified: it waits for an idle session and types nothing until then;
// it confirms the submission landed rather than only that the text appeared.
// A send that stays held fails with a SessionBusyError wrapping ErrSessionBusy.
// A send that does not land fails with ErrSubmissionNotLanded.
// A draft the send cannot clear fails with ErrSentTextInDraft.
// An operator active in the pane holds the write, so the operator's own typing is never overwritten.
// Run.Send and the gated wait loop's own send share the run's send lock, so two sends on one run never type at once.
// Runner.SendConditioned gives one line to a session only while it is in a named state, and refuses at once otherwise.
// Runner.ClearSession and Runner.CompactSession use the same guarded path.
// So do Runner.ReloadPlugins and Runner.TurnOnRemoteControl.
// Runner.TurnOnRemoteControl and Runner.AwaitRemoteControl report a connection only once the provider records one,
// so a connection the provider refuses, such as by an organization's policy, reads as none.
// A command taken as a prompt fails with ErrCommandTakenAsPrompt instead of confirming.
// Every interrupt goes through a pane guard that sends the key only to a running turn;
// a pane with no running turn gets no key and ErrNoRunningTurn, and an open rewind picker fails with ErrRewindPickerOpen.
//
// # Gates
//
// A gated run's GateSpec entries answer passed, failed or pending.
// A failed entry with budget remaining is re-prompted with one line.
// The line names the findings file, or is the entry's own Reprompt line.
// GateResult.Terminal finalizes the run at once with no re-prompt.
// A re-prompt held by a busy session spends no attempt.
// A strand that can take no keys resumes the writer's session in a fresh strand.
// The run directory and gate state are kept.
// A pending answer holds the run at a turn boundary without re-prompting or counting a failure.
// A deadline or liveness finalize evaluates each entry's optional Final closure in place of Gate.
//
// # Background work
//
// A turn end with background work outstanding (EventWaiting) is judged by the output files and what is outstanding.
// A gated run with an output file missing holds it at once.
// An ungated run with every output file present finishes done.
// A turn end with a fork or an awaited shell outstanding keeps waiting.
// The wait is bounded by the run's Timeout and the liveness check.
// Spec.AwaitedShellPrefixes names the shells the session awaits; Spec.validate does not inspect them.
// lym kills no shell, since the provider reports no pid: strand removal ends the shells it can.
// AwaitAny and Runner.AwaitForks are the shared capped poll over told paths and over a run's forks.
//
// # Display marks
//
// Wait shows each Go-side wait in a WaitMarker file (`wait.yaml`) and in a pane mark ReedOps.SetWaitMark sets.
// Both exist only for display and status.
// No shuttle decision reads either, and a failure to write one changes no verdict.
//
// # Session state
//
// Session state has two consumers: Runner.SessionIdle and the verified send's idle wait.
// The optional SessionSignalParser capability reads the events file's hook lines as provider-neutral signals.
// An engine without it reads unknown, and the idle gate falls back to the pane probe.
// The session state is a shadow reading the wait loop never acts on.
// Output files are consulted first only among the output-derived states,
// so a finished run can read dead.
// The first consumer that acts on dead applies PATTERN-shuttle-provider-seam's output check.
// ReadAgentActivity, ReadSessionStates and ReadAgentWaits read files only,
// so a process that runs no shuttle may call them.
//
// # Startup
//
// Start, StartGated, Run and RunGated run the startup probe before issuing a handle:
// readiness plus dismissal of any one-time startup gate, through the Engine seam.
// No caller outside this package probes readiness or plays gate keys.
// A not-ready provider is torn down and reported as ErrNotStarted from StartGated.
// A start that died with its strand removed is launched again in a fresh run directory.
// This repeats up to shuttle.yaml's start_retries times.
// A timed-out start and any failure before the strand registered are never launched again.
// Runner.AcceptStartupGate dismisses the gate of a strand already live, such as one added by hand.
// `TestProviderSeamImportRule` pins that no production file outside this package calls it.
// A production caller is a new decision.
//
// # Carried operator instructions
//
// Before its orphan sweep, a start finds the newest unfinished run with the same role key.
// It appends that run's carried and operator instructions to the new prompt as one block.
// A done or live predecessor and an empty role key carry nothing.
// A Spec with NoCarryOver set has no role key, so it neither takes instructions over nor leaves any to a later start.
package shuttleengine
