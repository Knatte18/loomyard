// Package shuttleengine runs one LLM agent as an interactive session and returns its result.
// Callers invoke it once per spawn to run a producer, handler or progress-judge and get its output files back.
// shuttle owns which provider (via an engine), the prompt envelope, and what "done" means.
// It does not own panes, layout, or tmux mechanics: it asks reed for a strand and drives the LLM in it.
//
// Every agent runs as an interactive tmux session, never headless `claude -p`;
// the constraint is economic (subscription coverage), not technical.
// This is why the whole proc -> reed -> shuttle stack exists instead of a plain headless exec.
//
// shuttle runs a provider through an engine, a per-LLM adapter that launches and drives its provider as a tmux session.
// The engine constructs the launch command, injects the prompt, recognizes the completion edge and locates the output.
// A Runner takes an EngineSet, which internal/shuttleengines builds from shuttle.yaml's `engine:` key.
// Spec and OpenSpec embed ModelChoice, which holds the engine, model, effort and version,
// so a construction sets `ModelChoice: choice` and a read such as spec.Engine works through promotion.
// Start and Open resolve the choice's Engine, an empty one meaning the set's default.
// They record the name in run.json's `engine` (in OpenResult for Open).
// Result.Engine is the resolved name of the engine the run was driven through.
// It is the set's default name for a record naming none.
// Every other engine-consulting operation resolves the engine its run's record names.
// An absent key reads as the default, and an unregistered name is refused with the lookup's error.
// Stop and teardown resolve no engine, so a record naming an unregistered engine never blocks them.
// The verdict/output contract is provider-invariant, which is what makes engines swappable:
// shuttleengine defines the Engine interface and its value types, and never imports a concrete engine implementation;
// concrete engines import shuttleengine, not the reverse.
// Only internal/shuttleengines and an engine's own directory import a concrete engine.
// seam_enforcement_test.go enforces that rule module-wide.
// StandingPassages returns the instruction passages heddle's layers hold for a session of an engine and a strand role.
// It reads them through the reader InstallPassageReader installs for the process.
// It returns none outside a hub, with no reader installed, or when the reader fails, which is logged at Warn.
// A launch therefore never waits on the store.
// An engine appends the passages to the system prompt it launches with, and shuttle itself parses no store.
// The Engine interface is extended by optional capabilities, which an engine can lack:
// requirement support, fork audit, the outstanding-fork reading, resume-context rendering and instruction files;
// the check after startup; and the idle wake of a provider no hook exit wakes.
// Creel hook delivery is one more: the hook-input parse, the hook output and the wake output.
// A ForkSubagents run whose engine cannot audit forks finishes with its classified result, a nil ForkAudit and an error.
// It keeps its run directory.
// Spec.Requires lists what a run requires of its engine.
// A ForkSubagents spec requires fork support without listing it.
// Start checks the requirements right after resolving the spec's engine.
// Attach checks them against the engine the run's record names, unless every output file is already on disk.
// A refusal wraps ErrRequirementsUnmet and names the way forward:
// at Start, a model alias of an engine that meets the requirements;
// at attach, ending the live strand with `lym reed remove` and re-stepping the row once the run record is old enough.
// The Start refusal carries a generic way forward, naming a model alias or shuttle.yaml's engine.
// A refusal no caller wraps therefore still names one.
// A caller that resolved the role's model from a config key passes the error through ModelKey.Wrap.
// Wrap replaces the generic way forward with one naming that key.
//
// shuttle is told its anchor path and worktree root as plain strings, at Runner construction, and derives neither.
// internal/lymcwd is consequently absent from the package's production imports.
// The two are adjacent parameters of one type whose consumers are semantically distinct.
// Construction therefore validates the pair, and every public method refuses on an unusable one.
// A swapped or relative pair fails loudly instead of succeeding against the wrong tree.
// There are two constructors, one rule each.
// NewRunner validates its pair (absolute, non-empty, anchor inside-or-equal worktree root) for hub geometry.
// There the anchor is always the worktree root or a subdirectory of it.
// NewDetachedRunner validates its own triple (absolute, non-empty, anchor and worktree root strictly disjoint).
// It serves standalone geometry, where the anchor is outside the worktree root it names.
//
// The only channel in and out of a shuttle run is files:
// the prompt is handed to the provider as the launch argument (never typed into a live pane).
// The agent writes its structured result to a file the caller reads.
// The package is two halves.
// The pure, hermetic half derives nothing and spawns nothing:
// the config module (shuttle.yaml), the run Spec and its validation, the run directory / run.json state;
// its age-guarded orphan sweep; and the Windows-to-POSIX path helper the engine layer needs for hook commands.
// shuttle.yaml carries no provider key but the neutral python guard switch `deny_python`.
// The keys a provider owns live in that engine's own config module.
// shuttle.yaml's `keep_warm_max_reruns` is the gate's break-even:
// the number of keep-warm re-runs one long gate wait is worth.
// Retired Claude keys still load from an unreconciled layer and are dropped.
// MigrateConfig raises the module's below-floor poll_interval_ms to its floor.
// It also renames a legacy `claude_deny_python` to `deny_python` for lym config reconcile.
// Start adds each run's engine name to `engines-run.json` at the run directory root before it registers the strand.
// The orphan sweep never removes that file.
// Runner.EnginesRun returns the union of that ledger and the engines of the run records still under the root.
// A caller thus learns which engines a worktree's sessions ran under after their directories are swept.
// The cross-run readers ReadAgentActivity, ReadSessionStates and ReadAgentWaits read each run.
// They use the engine its run record names, the engine set's default for a record naming none.
// They skip a run whose engine the set does not hold.
// The run-loop half drives a live agent through the ReedOps seam.
// It is Runner/Run in run.go, Wait in wait.go, and Attach in attach.go.
// It registers and removes strands and polls a real pane's capture through the engine's Startup classifier.
// It plays key choreography into that pane for Interrupt/Send/Inject.
// It reads reed's own persisted state during the orphan sweep and during Attach's own liveness gate.
// Both halves keep the provider boundary: nothing here names a tmux command or a Claude specific.
// Panes are reed's vocabulary, reached only through ReedOps.
// Provider grammar is the concrete Engine's, reached only through the Engine interface.
//
// A turn end without every output file never ends a run:
// Wait holds it, logs it at Info and keeps polling the same agent, whether the turn end is a Stop or a live ask.
// A run ends only as done, died, timeout, api-error or a mechanism failure.
// Done means every output file is present at a turn end, or at the deadline or a pane's death.
// Api-error means a spec with APIErrorRetries, or a turn end the engine marks ModelRejected.
// A died or api-error run whose strand is own-window keeps its last pane capture in `died-capture.txt`.
// The capture sits in the run directory; the run loses its strand, and an anchored strand stays.
// A timed-out run keeps its strand, since its agent may still be live.
// A run's strand lands in a window of its own unless its spec names an anchor.
// A spec naming the hidden anchor is refused before any strand exists.
// Reed starts no command for a hidden strand, so its provider would never start.
// A turn end that precedes the start prompt's own turn start is not a task turn end:
// Wait logs it at Warn as `turn end before the prompt's turn start; not held` and neither holds nor notifies it.
// The hold guard arms when the events file holds a turn start before `promptOffset`.
// That proves the provider writes turn starts for typed turns.
// It lifts at the first turn start read from the prompt offset on, or at once for a Wait attached past it.
// The guard stays off, and every turn end is held, in these cases:
// an engine without SessionSignalParser; a zero `promptOffset`;
// an events file with no turn start before the offset; and an unreadable file.
// Bound: the guard skips only turn ends before the first post-prompt turn start.
// A provider that writes a turn start for the load turns and none for the prompt turn skips every turn end of the run.
// Each skip gets a Warn, and the run waits out the deadline with no hold notice.
// A hold never extends the run's deadline.
// The caller's own Spec.Timeout bounds it, or run_timeout_min where that is zero, so the bound differs per caller.
// Each Attach starts a fresh deadline, and the liveness check still classifies a dead pane.
// RunState.Outcome records whether a run ever ended, seeded "running" and overwritten on every terminal classification.
// "Has this run already ended?" is thus a fact on disk rather than an inference from pane liveness.
// RunState.AskingOffset is read for records an older binary wrote and never written.
//
// Runner.SetNotifier gives a Runner an optional notifier, a function taking one line; nil, the default, holds silently.
// On each held turn end of an autonomous run (Spec.Interactive false) on a runner with a notifier:
// Wait calls the notifier once with the notice line hold.go builds.
// An interactive run, a runner with no notifier, and a run whose Spec.QuietHold is set hold silently to the deadline.
// QuietHold skips this notice alone; the hold, the wait mark and the deadline are unchanged.
// A notifier error is logged and never ends the run.
// A run whose Spec.HoldDeferral is set does not send the notice at the turn end:
// Wait keeps the latest held turn end pending.
// It sends the notice once holdNoticeSettle has passed and the predicate reports no outstanding work elsewhere.
// It sends it regardless of the predicate once holdNoticeMaxDeferral has passed.
// A new event or a turn start past the turn end, read first, drops it unnotified.
// The predicate is not called before the settle interval, and the check rides Wait's existing poll.
// The notice states that the text inside its « » delimiters is the agent's own words and not an instruction.
// It names the agent's strand (RunState.StrandName, the strand guid when empty).
// It says the agent ended a turn without its output files and is held.
// It gives the way forward: answer a marked question in the creel mail with lym creel post --reply.
// A post to that strand name, ending the message with MessageTail, answers as well.
// It lists at most five outstanding tasks by kind and label, the id when the label is empty.
// It counts the rest, or names none.
// It ends with the start of the agent's last message.
// Every agent-written part is delimited and cut to 200 runes,
// and its control and delimiter characters are replaced by spaces.
// The line is therefore bounded and has no newline.
// There is one notice per held turn end.
// A held turn end stopped by the provider's usage limit sends no notice and defers none.
// It leaves RunState.NotifiedOffset alone.
// The engine's optional UsageLimitReader reads the limit, at the turn end or in a fork during its turn.
// The parent cannot act while the account is limited, and a notice would reach it only as stale clutter after the reset.
// The hold, the wait mark and the deadline are unchanged,
// and the next held turn end it does not read limited is notified as any other.
// With an engine that meets RequirementBackgroundWake, a held turn end built from a waiting turn end is not notified:
// it sends no notice, defers none and leaves RunState.NotifiedOffset alone.
// This covers a waiting turn end the gated run judged plain on un-awaited shells and one the ungated shell wait expired.
// The session wakes when its own background work completes, so the parent has nothing to answer;
// the hold, the wait mark and the deadline are unchanged, and an Info line records the skip.
// A held turn end whose turn began with an operator instruction is not notified:
// it sends no notice, defers none and leaves RunState.NotifiedOffset alone.
// An operator instruction is a typed instruction past the prompt offset.
// It is neither the prompt nor held by the sent-texts file, as the carry-over below reads one.
// The operator is talking to the agent in its pane, so the parent has nothing to answer;
// the turn start is the newest one before the turn end, and an Info line records the skip.
// Creel mail, re-prompts and continue lines go through the verified send,
// so a held end of a turn they started is notified as any other.
// A held turn end that is an API error across a detected sleep is first given one automatic continue line.
// The sleep must last at least pauserecord.SleepTolerance.
// The line is typed through the guarded paste-submit, and the turn end sends no notice and defers none.
// It goes out only when no pause covers the run, the session state names the newest turn end an API error,
// and the sleep lies after the latest turn start Wait read.
// RunState.APIContinued is persisted before typing and cleared by the next held turn end that is not an API error,
// so a turn gets at most one continue.
// A send that fails, the guard's ErrSessionBusy included, types nothing more.
// The stall is then held and notified as any other.
// A turn end on an API error that is not also an event, such as Claude's StopFailure, holds nothing and sends no notice.
// A run whose Spec.APIErrorRetries is set re-sends such a turn with one line after a back-off.
// The back-off doubles with each API-error turn end of the run.
// The turn end past the last re-send ends the run OutcomeAPIError, with Result.APIError naming the error.
// When every output file exists, it ends the run done instead.
// A run with no retries waits such a turn end out to its deadline.
// A turn end the engine marks SessionSignal.ModelRejected ends the run OutcomeAPIError at once, retries or not.
// No re-send changes the model, but every output file existing ends the run done.
// RunState.NotifiedOffset is the events-file offset past the last notified one.
// It is persisted before the notifier is called.
// A held turn end at or below it is never notified again, including when an Attach replays it.
// A batch of several new events is classified by its last event, so only its last held turn end is notified.
// The notice logic stays provider-invariant, per the Shuttle Provider-Seam Invariant.
//
// A second notice covers a permission or input dialog lym cannot answer.
// It goes out in the same runs and through the same notifier.
// An interactive run and a runner with no notifier skip it as they skip the hold notice.
// An engine with SessionSignalParser and InputBoxReader reports each ask that is not also an event.
// Wait folds those asks into a latch that sets on the first ask and keeps its stamp for further asks.
// While the latch is set, each tick reads the pane.
// An input box on screen clears the latch at any age,
// and a failed capture or a pane too short to draw the box changes nothing.
// A pane with no input box, once the latch is permissionNoticeBound old, gets one notice line.
// The line names the strand.
// It says the operator answers the dialog in the pane, reached with `lym reed attach` in the run's worktree.
// A later turn end or turn start never clears the latch.
// Spec.QuietHold and Spec.HoldDeferral neither suppress nor defer the notice.
// RunState.DialogNotifiedOffset is the events-file offset past the latest ask folded into a notified episode.
// It is persisted before the notifier is called and advanced by later asks of that episode.
// A Wait reads asks from the larger of the prompt offset and DialogNotifiedOffset.
// It never rebuilds a latch from an ask at or below that offset, including after an Attach.
// The offset is kept apart from RunState.NotifiedOffset, so a hold notice and a dialog notice never suppress each other.
// The notice is never typed into the pane and changes no outcome.
//
// AwaitAny is the shared wait.
// It stats the told paths once a second until one exists or the cap elapses, and watches no directory.
// It is the poll every capped keep-cache-warm wait verb runs on.
// DefaultAwaitCap is the fallback of `shuttleengines.KeepWarm`, the resolver of the cap those verbs default to.
// Runner.AwaitForks is the same capped poll over the forks a run's session launched.
// It reads them through the engine's optional OutstandingForkReader capability.
// It answers ready once none is still running.
// A session that spawns forks and waits inside that call takes every fork's result in the turn after the call returns.
// It does not wake once per fork.
//
// # Pending prompt
//
// A launch that defers its prompt to a typed line (Launch.PromptLine) records the line in run.json as `promptLine`.
// Run.DeliverPendingPrompt reads that record and types the line through the guarded paste-submit.
// It types only when no turn start whose instruction is the line lies at or past RunState.PromptOffset.
// Thus a step that died between its skill-load turns and its prompt delivery still gets its task.
// A turn start with another instruction, one the operator typed included, does not count as the prompt's.
// It first persists the events file's size as the prompt offset when none was recorded.
// It clears any pending-send record so the send types the line, and it reports whether it sent.
// It types the line at most once per handle and changes nothing when it fails.
// Wait calls it on two occasions.
// The first is once, at the first tick no pause covers the run.
// It applies to a run Attach reconstructed whose output files are missing.
// The second is a held turn end with no turn start read past the prompt offset and no pause covering the run.
// A held turn end it types the prompt at sends no hold notice,
// and a failed delivery is logged at Warn and tried again at the next held turn end.
// Bound: a prompt the provider queued while a turn ran starts its turn only when that turn ends,
// so a held turn end read before it can type the line a second time.
//
// # Pause
//
// Runner.SetPause tells the runner a pauserecord.Geometry; a runner never told one reads no record.
// Every agent spawn starts through the runner, so it is the hold point.
// A start waits at entry while a pause covers the geometry's slug.
// While held it starts nothing, registers no strand and has no deadline of its own.
// The run's deadline is taken after that wait, so a started run's deadline counts from its start.
// Each Wait tick reads the runner's coverage first.
// While a pause covers the run, the deadline check is skipped.
// A held turn end sends no notice and leaves RunState.NotifiedOffset alone, and a latched dialog sends no notice.
// A held turn end read while covered is not re-notified after the resume, since the resume line starts the next turn.
// An unreadable record reads uncovered, with one Warn per change of error.
//
// A line typed into a session a pause covers is refused with a *PausedError, which wraps ErrPaused.
// Nothing is typed.
// The send judges the target by its own slug, the one in the strand name reed reports.
// The name is the one on the run's record.
// For a strand with no record it is the live strand's name, and reed's status supplies it only when neither is held.
// A prime name has the empty slug, which only a hub entry covers.
// A name that does not parse, an unlisted strand and a failed status read fall back to the runner's geometry slug.
// The coverage is read after the idle wait.
// It is read again before each replay of a typed choreography, the conditioned send's included.
// Keys are not lines: an interrupt, an inject and the pause's own Esc are delivered under a pause.
// A runner told no pause geometry, as the orch runner is, reads every target uncovered.
// The start's segment-color line is a send too, so a refusal drops it with a Warn as any color failure.
// The start's prompt line, its typed skill-load line and the deferred task prompt wait out a refusing pause.
// They wait on the slug the refusal judged.
// Each then sends again, looping if a second pause lands first.
//
// Each Wait tick reads the coverage of the run's own session by that same slug, not the runner's.
// A gated Done arrival that reaches the boundary while covered is recorded as reached and held.
// No gate closure runs and no pending or re-prompt send goes out.
// The first uncovered tick evaluates it as if it had just arrived.
// A re-prompt refused with ErrPaused is held like a busy one, spending no attempt.
// The off-boundary expiry probes keep running under cover, since they send nothing,
// so an entry's own wait bound can still end a run.
// The hold only delays an evaluation and changes no verdict or attempt count.
//
// # Awake deadline
//
// The run deadline counts awake, unpaused time, and no time a start's skill-load turn is held on an API error.
// A Clock that is also an AwakeClock feeds the run's sleep watch each tick.
// The deadline moves later by the union of the detected sleeps and the coverage's paused intervals.
// The union spans the time since the run was created.
// A clock without Awake detects no sleep.
//
// # Opened sessions
//
// Runner.Open opens a named interactive session with no prompt in a window of its own.
// It serves an extra session the orch or the operator wants beside the runs.
// It needs an engine with the optional SessionOpener capability,
// and a covered pause refuses it before anything is created.
// A non-empty OpenSpec.Skills needs the SkillLoader capability too.
// The engine's fresh launch line carries their load message as the session's only turn.
// After it the session waits for the operator.
// After the provider line the strand's command chains an exit marker, reed's detached self-removal and the shell's exit.
// The marker lies under the anchor's ephemeral `shuttle-open` directory.
// Ending the session therefore closes its window.
// The marker is the strand's done-when path, so a resume drops a finished session instead of relaunching it.
// OpenLaunch is told a directory of the session's own for the files its launch reads.
// It lies beside the marker and is named by the same token.
// The engine creates it only when it writes one.
// The strand carries the provider's session id, which the watchdog's name repair reads.
// The strand also carries the launch's CreelHooks mark,
// so a session whose launch carries creel's hook delivery gets its mail through its hooks as a run session does.
// An open writes no run directory, no run.json and no output file,
// so the state verb and the hold notices never cover an opened session.
// OpenSpec.PermissionMode is engine vocabulary carried blind to OpenLaunch,
// and the engine realizes it or refuses it before anything is created.
//
// # Unawaited sessions
//
// Runner.StartSession starts a session that carries the rendered prompt and is never awaited.
// It serves a caller that talks to the agent itself and stops it when it is done.
// The Spec sets Unawaited and names no output file.
// StartSession refuses a spec without Unawaited.
// Spec.validate refuses an unawaited spec that names an output file.
// It gives the session its run directory, run record, settings and shell guards, its skills and its startup gates.
// It returns once the provider is past startup and never calls Wait.
// The run record reads running until a stop ends it.
// A resume treats the strand like any other non-live one, since an empty done-when list never drops it as finished.
// A Spec with ResumeSessionID resumes that session through the engine's resume capability.
// Bound: one session per call in the caller's worktree, with no timeout classification, since nothing waits on it;
// the caller stops it through StopStrand.
//
// # Stop verb
//
// Run.Stop and Runner.StopStrand remove a run's strand after recording the stop in run.json.
// Runner.StopStrand is the guid form, for a caller holding no handle.
// They remove a live strand.
// They also remove an own-window strand whose pane is present but not live, a dead pane or one showing only its shell.
// A dead window 0 strand is left to reed's reconcile.
// The record reads "stopped", a record-only terminal outcome that is never a Result.Outcome.
// A record already terminal keeps its outcome.
// A record still reading running whose output files all exist keeps running,
// so a stop never turns finished work into a stopped record.
// An empty output list, an unawaited run's, has no finished work and reads stopped.
// The record is written first.
// A failed write returns its error with the strand untouched.
// A failed removal leaves the terminal record over a live strand.
// Attach reads such a strand as respawn-eligible and stops it as superseded.
// A guid with no run record is stopped by removing its strand alone.
// Runner.SetStepTie tells a runner the shed step its process runs.
// A StepTie holds the steps directory and the step's trace id.
// Every run the runner starts records that step in run.json as `step`, and a runner never told one writes no step.
// Runner.StopStepRuns stops, through the same record-then-remove order, every run under the runner's run-directory root.
// A run qualifies when it still reads running, has a live or not-live own-window strand and records the given step.
// A running run with such a strand and no step is listed under StepStopReport.Untied and left alone.
// A run of another step is left alone.
// A stop that fails is listed under Failed while the stop goes on.
// Attach's removal of a superseded run's strand goes through the same record-then-remove order.
// A Wait on a stopped handle finalizes with the stop mark and records "stopped" unless the files show the run done.
// Its liveness check reads the strand the stop removed as the run's end, OutcomeDied.
// It never reads it as reed's bookkeeping going away.
// A Wait in another process may overwrite "stopped" with its own terminal outcome, and either answer is terminal.
//
// A Run's record lock guards the stop mark, RunState.Outcome and every save of run.state,
// so a stop from another goroutine never interleaves with Wait's own record writes.
// Shuttle's teardowns inside the run lifecycle (finalize's cleanup, abandonStartup) write the record themselves.
// They do not go through the verb.
//
// Spec.PermissionMode and Spec.ResumeSessionID are caller-owned engine vocabulary.
// Spec.validate never inspects them, and the engine is the sole validator and realizer.
// Empty PermissionMode selects the engine's bypass mode in both run modes.
// Every session lym launches into reed needs it.
// An empty ResumeSessionID mints a new session.
// A non-empty ResumeSessionID launches that existing session instead.
// OpenSpec.PermissionMode is the same vocabulary, carried blind by Runner.Open, with empty selecting bypass there too.
// Each engine maps the default onto its own bypass flag and names its own per-call opt-out.
//
// An ungated run's strand is registered with its output files as reed's done-when list,
// so reed's resume drops a finished agent strand instead of reviving it.
// A gated run's strand carries none, since its outputs existing does not mean its gate passed.
// The gate re-prompts the live agent instead.
// The list is provider-neutral, set by the runner and never by a provider engine.
//
// Runner.Attach answers one question:
// is there a still-live-or-already-finished, never-terminated run for this exact output-file set?
// If so, it waits on that run instead of starting a second agent.
// The answer rests on the persisted RunState.Outcome.
// It then applies, in a fixed precedence, the run's own file contract and reed's live-agent evidence.
// It does not rest on output-file existence at the caller's level:
// a bounce that re-runs a producer over already-present files looks identical to a finished run from there.
// Inside Attach the same file-existence question is safe.
// It is asked only of a matched run.json still declaring itself "running".
// Such a run is an agent to attribute the files to, and a bounce never leaves one behind.
// Those signals are defence in depth, not redundancy: they fail in different directions.
// The persisted record answers "did this run end"; the file contract answers "did its agent finish";
// reed answers "is the process there".
// A crash mid-run leaves the record at "running" with reed still tracking a live pane.
// A crash after the agent wrote every output file leaves the record at "running" with reed's answer irrelevant.
// That is why the file contract outranks reed.
// It is also why an unreadable, absent, or unanswerable strand table still harvests such a run rather than refusing.
// A corrupted or torn-down reed session leaves the record honest but the process unreachable.
// Attach reconstructs its *Run without calling Start, so sweepOrphansOpportunistic never runs there.
// A caller must therefore probe Attach before anything else that could sweep the directory it is looking for.
// An absent or unreadable reed state file and a Status() failure are errors rather than found == false.
// Neither is evidence a run has ended.
// Both name the run directory and the strand guid in a logged warning.
// The operator's only escape from either is out of band, via "lym reed status".
// A run held at a turn end keeps outcome "running", so the same probe finds it.
// A record left at the legacy outcome "asking" is attached too when reed tracks its strand as live,
// with or without an AskingOffset and whether or not its events grew.
// With a dead or untracked strand the usual classification applies.
// The attach resets such a record to "running" and replays from its AskingOffset (the prompt offset when it has none).
// It counts the old ask as notified, so only a held turn end after it notifies the parent.
// Only a live, tracked strand is attached, and the attach starts a fresh deadline.
// An attached record with no StrandName takes the name reed's status reports for its guid.
// A not-found answer is the caller's cue to start a fresh run.
// Attach and AttachGated first remove, with a logged warning, the live strand of an earlier run.
// An earlier run qualifies when it has the same output-file set and the probe judged it respawn-eligible:
// re-running a producer supersedes the halted session.
// The removal is bounded to that set, so no other strand is touched:
// not another producer's, not a same-role strand with other outputs, and not a run the probe would attach to.
// A probe that attaches or refuses removes nothing.
// A strand that cannot be removed comes back as an error naming `lym reed remove <guid>`, never as a not-found answer.
// Runner.AttachIfLive is the same probe with the removal off.
// It serves a caller that waits on a live run and starts nothing after a not-found answer.
// Runner.AttachIfLiveGated is its gated form.
// Runner.ProbeGated is AttachGated's probe returning the reconstructed *Run unwaited,
// so a caller can hold probed runs and wait on them concurrently.
//
// Session usage: the optional UsageReader capability reads a finished session's tokens, forks included.
// It returns them as a provider-neutral SessionUsage.
// Run.finalize fills Result.Usage from it at every terminal outcome.
// It fills Result.StartedAt and Result.EndedAt for every run;
// an engine without the capability leaves Usage unknown, and nothing about the outcome changes.
//
// Session end: Runner.SetSessionEnd sets a hook called once for each ended session.
// The hook receives a SessionEnd carrying the session's id, engine, role, model, pane cwd, times, outcome and usage.
// Run.finalize calls it at every terminal outcome after the usage read.
// Runner.StopStrand calls it when it moves a running record to stopped, with the usage the record's engine reads.
// A session the stop verb ends while unawaited is reported the same way.
// RunState records the spec's role and model at launch so the stop path can fill them.
// A session with no session id is not reported, and a runner with no hook reports nothing.
// The hook runs synchronously and its panics are not recovered, so it must not block and must report its own failures.
//
// Record reading: an engine's finished records are read with no run and no shuttle config.
// The optional capabilities for it are TranscriptReader, MessagePricer and InstructionFileLister.
// TranscriptReader locates a worktree's sessions and reads each, forks included.
// It yields provider-neutral TranscriptEntry values with a per-message MessageUsage.
// The reader keeps every provider rule and its consumer keeps every lym-format rule.
// RecordReads bundles one engine's record-reading capabilities, nil where it lacks one.
// internal/shuttleengines hands it out.
//
// Segment color: Spec.Segment names the loom segment the spawning module gives its role.
// Runner.start forwards it to reed, which resolves the segment's palette color on the strand it returns.
// Once startup confirms the provider ready, start plays Engine.ColorSequence for that color into the strand's pane.
// For a spec that names skills it plays after the first load turn, which the launch line carries.
// Nothing is typed for an engine without a color command, a strand without a color or a spec with ColorByCaller.
// The command goes out through the guarded send path, like every other write:
// a paste-path engine pastes it once and the empty box confirms it,
// so the next step never finds it still sitting in the box.
// A held or failed color is logged and never fails the launch, since the color is display only.
// Runner.TypeColor sends the same sequence into a live shuttle strand for a caller that colors its own session.
// It returns a held color's error, wrapping ErrSessionBusy, so the caller may retry.
// Runner.ClearSession, Runner.CompactSession and Runner.ReloadPlugins send their slash commands through the same path.
// A draft in the box or an active operator holds them with ErrSessionBusy,
// so a caller's earlier idle probe never lets them type.
// The idle reading takes a box of whitespace as empty, and a provider takes a command behind whitespace as a prompt,
// so an engine with InputBoxClearer gets its clear right before the paste of a command or the color.
// The clear goes only into a box that reads blank or unreadable with no operator active.
// Text typed since the idle reading holds the command.
// A command's text starts no turn.
// A turn start past the paste with no instruction, or the command's text as one, means the provider took it as a prompt.
// The send then fails with ErrCommandTakenAsPrompt instead of confirming.
// A turn start with another instruction, such as an operator's prompt, is no signal for a command.
// Runner.ClearSession and Runner.CompactSession confirm on a session-start line the provider writes for their command.
// The line has source clear or compact and lies past the events offset taken before the paste.
// An empty box does not confirm them early.
// When the submit window closes with no such line, the box read decides as for any send,
// so an engine that records no session starts confirms on the empty box at the window's end.
// Once the box has read empty, the command counts as submitted:
// later box text, such as a draft the operator types during a compaction, sends no key and fails nothing;
// an unreadable box does the same.
// The window's end confirms them.
// An engine without SessionSignalParser confirms them on the empty box at once, as any other send.
// A provider with no command for the request types nothing and asks nothing.
// The palette-to-command mapping is provider grammar and lives in the engine.
//
// Skill loading: Spec.Skills names provider-neutral skills.
// Shuttle loads them into a fresh session, all in one turn, before the prompt.
// It needs the optional SkillLoader capability.
// A spec that names skills on an engine without it is refused before any run directory exists.
// An engine that realizes skills carries its one load message for the whole list on the launch line,
// so the first load turn is the session's first turn and is never typed.
// The engine returns the prompt pointer as Launch.PromptLine;
// once the provider is ready, shuttle polls until the first turn end reaches the events file or the timeout passes.
// The timeout is Spec.SkillLoadTimeout (zero means the engine's DefaultSkillLoadTimeout), measured from the first poll,
// and it counts only when no turn start has been read.
// The launch-carried message skips the verified send's landing check;
// a turn end in the events file proves it landed, and a turn that never starts is bounded by the timeout.
// The color follows the first turn, then any retry turn, then the prompt.
// The engine classifies that turn end into a SkillLoadReport:
// each unknown skill is skipped at once with a logged warning,
// and the missing ones are asked for once more in a second, typed load turn naming only them,
// whose still-missing skills are skipped with a logged warning.
// A turn whose evidence the engine could not read is confirmed unverified with one logged warning and no retry,
// and a turn no turn start is read for skips its skills with no retry at the timeout.
// A turn start read with no turn end after it is awaited until its end, the pane's death or the run's deadline;
// a provider retries a failing API inside the turn for minutes,
// and a deadline passing first ends the start as timeout with the prompt never typed.
// An open turn that an engine with SessionProber reads as interrupted skips its skills at once with no retry;
// an interrupt writes no turn end.
// So a launch runs at most two load turns the API does not fail,
// and a skipped skill never fails or hangs it, which only a turn still open at the run's deadline does.
// A load turn that ends on an API error, by the signal or by the transcript's marker on a turn end, skips nothing.
// It spends no retry; the error says nothing about the skills.
// Shuttle holds the launch as a pause holds a send.
// It waits a back-off that starts at apiErrorRetryBackoff and doubles to loadHoldBackoffCeiling.
// It then types the same load message through the verified send path, and goes on so until a load turn ends.
// That typed turn is the only probe of the API a session has.
// So the prompt line is never typed into a session whose load turn the API failed.
// A turn start read during the back-off takes the place of the typed line.
// A busy session moves the line to the next back-off, and a pane that dies ends the hold as a died startup.
// The hold has no bound of its own and moves the run's deadline later by its length;
// at the second API error it sends the run's parent one notifier line, skipped for an interactive run.
// A turn end the engine marks ModelRejected ends the start at once as api-error, with Result.APIError naming the error.
// No further turn changes the model.
// An attached run is the exception: it delivers a pending prompt without judging the API,
// so a driver that died during a hold has its prompt typed by the attach.
// PromptLine then goes out through the verified send path.
// Every verified send first waits for an idle session and types nothing until then.
// The wait lasts at most `send_ready_timeout_s` and never runs past the run's deadline.
// For an engine with the optional SessionCycler idle reading, every poll takes the readiness reading.
// It is built on the hook-derived session state.
// The reading is ready on idle-done, idle-stalled of any cause, idle-fresh, and asking at an awaited turn end.
// It is also ready on busy with background work.
// A ready reading requires no unmatched turn start and a blank input box, where the engine reads one;
// it is held, with a reason, on a running turn, a question awaiting an answer or a dead process.
// A draft in the box holds it too.
// So does a pane too short, or showing no input box to an engine that reads one, as when a menu or picker is open.
// A ready reading lets the send type even when the pane still shows the running-turn needle.
// The pre-write guard is the last step of every poll.
// Once the rest of the reading passed, shuttle asks reed once whether a client attached to the pane was active.
// A client active within `operator_quiet_s` holds the write.
// So each poll is one fresh reading of readiness, box and activity immediately before the caller types.
// An activity query that errors holds nothing, so a tmux that cannot list its clients stalls no run;
// the empty-box requirement still stands.
// The failure is logged at Warn once per episode per strand.
// It is logged at Debug while it repeats and at Info when the query reads again.
// The runner shares one episode per strand across its sends and idle probes.
// A polled probe of a failing wait and a skipped unreadable file of the files-only readers log the same way.
// A held gate re-prompt logs Info when it first holds and Debug while it stays held.
// Runner.SessionIdle applies the same hold after a ready reading,
// so the orch watcher's probe reports an active operator with its reason.
// An unmatched turn start releases after the pane has read idle on every poll for turnStartIdleOverride.
// It also releases at once when the engine reports that turn interrupted.
// An engine with TurnProgressReader that reads the unmatched turn in flight vetoes the idle-pane release,
// so the turn start keeps holding until a turn end, a reported interrupt or a later session start.
// A session start after the turn start lifts the veto, since the session's context began anew.
// Runner.SessionIdle reports busy while a turn reads in flight.
// The state is folded from the signals past the run's prompt offset;
// when none lies past a non-zero offset, it is folded from the whole events file,
// so the prompt send after the skill-load turns reads the last load turn's end rather than an unknown state.
// When the session state is unknown, the reading falls back to the pane, which must classify ready and idle.
// For an engine that parses session signals, no turn start may be left unmatched by a later turn end;
// the cause is logged at Debug.
// A fallback hold whose box reads non-empty quotes the box.
// Styled box read: for an engine with StyledBoxReader, every box reading of a decision takes one styled capture.
// That covers the idle probe's, the pre-write guard's and the landing check's reading.
// It reads the plain capture the engine makes of it, with a box holding only the provider's placeholder blanked.
// So a placeholder or prompt suggestion in an otherwise empty box reads as empty,
// and a typed draft still reads as a draft.
// An engine without the capability reads the plain capture;
// every other capture, such as the ready-pane, interrupt and picker checks, stays plain.
//
// Every interrupt key shuttle sends goes through a pane guard.
// Run.Interrupt and Runner.Interrupt send the engine's interrupt sequence only after two checks.
// They are the ready-agent-pane check and one fresh capture the engine's InterruptGuard reads as a running turn.
// A capture that fails sends nothing and fails the call, so the guard fails closed.
// A pane with no running turn gets no key and ErrNoRunningTurn, which means "nothing to stop".
// Before every interrupt and every verified send, an open rewind picker is closed.
// An engine with RewindPickerReader reads it from a capture; the close is one key of the interrupt sequence.
// The close waits the engine's interrupt settle, captures the pane once more and sends nothing further,
// so it never selects an entry.
// It always fails the operation with ErrRewindPickerOpen, which never wraps ErrSessionBusy.
// Nothing is typed or interrupted after it.
// A verified send whose picker capture fails skips the close when the idle wait's polls follow;
// it fails closed when no polls do.
// During a verified send's idle wait, a known mid-session picker gets that picker's keys, at most once per send.
// An engine with DismissiblePickerReader reads the picker from the capture of a held reading.
// The keys keep the provider's current setting.
// They go out only when the operator was not active in the quiet window.
// The engine's InterruptGuard, where it has one, must also read no running turn.
// The pane then settles for the engine's interrupt settle and the wait reads it again,
// so the send goes on once the input box is back.
// A send still held by a pane showing no input box fails with a SessionBusyError whose NoInputBox is set.
// Runner.InterruptRestoring is the interrupt for a caller that must leave the box empty.
// Its key goes only through the guarded interrupt, and it then clears the prompt the provider restores into the box.
// For an engine with RestoredPromptClearer and InputBoxReader, it reads the box before the key.
// Only a box read empty then is cleared after it.
// After the engine's interrupt settle it reads the box, and a box still empty gets one more settle and one more read.
// Restored text is cleared once, with the provider's restored-prompt clear.
// The clear needs a guard reading of the same capture plus one operator-activity query:
// the pane reads ready with no running turn, the box is readable and the operator is not active.
// A failed guard reading sends no clear, so the text stays for the receiver's next send to hold on as a draft.
// A failure after the key wraps ErrInterruptClearFailed, so a caller treats the interrupt as sent.
// Runner.InterruptSettle is the settle a caller that retries an interrupt waits after its first key.
// Runner.PaneRunningTurn reads whether a strand's pane shows a running turn.
// Stray character: a box holding exactly one stray-character candidate is cleared once per send attempt.
// This needs an engine with StrayInputReader, InputBoxClearer and InputBoxReader.
// It counts on the hook reading held only for that draft, and on the pane reading that is ready but not idle.
// An active operator holds the send instead, quoting the box.
// Otherwise the clear plays right after that reading, the box settles for `SubmitSettle`,
// and a second full reading is the write's guard reading.
// From the clear on, every reading of the attempt is a post-clear reading:
// a box still holding exactly the cleared character reads as empty, so the send goes out,
// and anything else holds as a draft with no second clear.
// A character that survives is submitted at the head of the message,
// and the post-paste confirm keeps its exact-text reading, so it fails with ErrSentTextInDraft.
// A session that stays held fails the send with a SessionBusyError wrapping ErrSessionBusy.
// The error names the reading and ends with the pane's last lines;
// its Draft field is true when the hold is text in the input box;
// IsDraftHold also reports it for the two paste refusals below.
// Run.Send and the gated wait loop's own send each hold the run's send lock for the whole send.
// The whole send is the idle wait, typing and delivery check.
// So an in-process send and a gate re-prompt on one run type one after the other, never at once;
// the lock skips no idle wait, delivery check or gate evaluation.
// An engine without the idle reading keeps the not-ready refusal and the operator-activity hold, with no wait:
// either fails the send at once.
// For an engine with the optional InputBoxReader capability, that path confirms a send is submitted.
// That confirmation takes more than the text appearing in the pane.
// The window is `submit_confirm_timeout_s` from the moment typing begins.
// The text is typed without its Enter.
// Once it has appeared, shuttle reads the input box every `submit_settle_ms` until two reads agree,
// so no Enter lands inside a typing burst;
// reads that agree only once the window has closed send no Enter.
// It then sends the Enter.
// It reads the box at an interval that starts at `submit_redraw_settle_ms` and doubles up to confirmBackoffCap.
// While the box still holds the sent text and the window is open, each read is followed by one more Enter.
// Every idle poll, box read and Enter runs a tmux process through reed,
// so each of these loops also stops at an attempt count derived from its window.
// That count ends the loop under a clock that never advances.
// The submission is confirmed when the box no longer holds the sent text.
// It is also confirmed when the engine's session signals show a turn start past the file's size before the send.
// A send that does not land fails with ErrSubmissionNotLanded, naming why and ending with the pane's last lines.
// An empty box then means the submission landed late and the send succeeds.
// Otherwise an engine with both the idle reading and the optional InputBoxClearer capability handles it:
// a box that holds exactly this send's own text, re-read just before the key, is cleared and read again;
// a box holding anything else gets no key, and a box the clear did not empty is named in the error.
// The text counts as held only when the box reads exactly the sent text, normalized,
// so a box that merely contains its leading part is never taken for it.
// An engine without the capability keeps the appearance-only check.
// A pasted send needs an engine that has the optional TextPaster capability, the box reader and the idle reading.
// It issues one reed paste-submit, with no Escape and no typed keys.
// The paste carries the trailing Enter when `PasteSubmits` is true;
// with it false, the box is settled first and the Enter goes out as the guarded one below.
// The box is first read `submit_redraw_settle_ms` after the paste, then at a doubling interval up to confirmBackoffCap.
// Each read decides:
// a turn start past the file's size before the send, or an empty box, means the paste landed;
// a box holding exactly the sent text, or the provider's own placeholder for it, means the Enter did not take;
// the guarded Enter then goes out, sent only when no operator was active in the pane just before the key,
// and the box is read again;
// no readable box sends nothing and is read again,
// and a window that ends that way, or on an exact match, fails with ErrSubmissionNotLanded;
// the sent text beside other text fails with ErrSentTextInDraft, which wraps ErrSessionBusy;
// any other text fails with ErrForeignBoxText, which wraps ErrSessionBusy, quoting the box;
// neither sends a key.
// A pasted send that fails with ErrSentTextInDraft or ErrSubmissionNotLanded may have left its text in the box,
// so it records `{text, events offset}` in `pending-send.json` in the run directory.
// The record has a file of its own so it never races run.json's writers.
// A paste that landed, was held before it wrote or failed in reed records nothing.
// The next send to the strand, once the idle wait passed on an empty box, reads and clears the record.
// When the text is the same and a turn start lies past the recorded offset, the text went out.
// It went out with the draft or with an Enter the operator pressed, and the send returns landed without pasting;
// otherwise the operator cleared the box, or the record was for another text, and the send pastes as usual.
// A record that cannot be read or cleared is logged at Warn and the send pastes, so a broken file never wedges a strand.
// A retry whose text started no turn, as a slash command's does not, always pastes afresh.
//
// Conditioned send: Runner.SendConditioned gives one line to a session only while it is in a named state.
// One ready reading shows the state, such as idle-stalled with cause `api-error`.
// The reading waits on nothing, plays no key and refuses at once:
// with ErrRewindPickerOpen when the rewind picker is open, which stays open;
// with ErrSendConditionUnmet when the session reads another state,
// so a busy session and a turn start after the stall are refused;
// and with a SessionBusyError for a pane too short or showing no input box.
// The same error refuses any text in the input box, a lone stray character included, and an operator active in the pane.
// It never clears a stray character, never closes the picker and never clears a text that did not land,
// so a send that does not land fails with ErrSubmissionNotLanded and leaves its text in the box.
// It never reads the pending-send record, so a line an earlier unlanded send left pending is typed again.
// Once the send lands it waits at most one submit window for a turn start past the events file's size before the send.
// It reports whether one read.
// Its caller waits for one reading.
// That is at most one submit window from the start of typing to the landing and at most one more for the turn start;
// the pane captures inside that span wait on reed's per-worktree operation lock and on a tmux call.
// That call has no timeout of its own, so the windows bound the polling and not a hung reed or tmux call.
// The run's events offset is the events file's size at the moment before the prompt is sent.
// That lies past every load turn end, the retry's included, and past any line appended after them.
// run.json records that offset as `promptOffset` before the prompt goes out,
// and every reader that replays the events file without Waiting on the Run starts there:
// Attach, and webster's recovery classification through its batch record.
// A pane that dies meanwhile is a died startup.
// An empty Skills list changes nothing.
// Runner.LoadSkills and Runner.ClassifySkillLoad are per-tick primitives for a caller reloading a live session.
// The first sends the load message for a list; the second classifies the turn end that followed.
// Neither carries provider command text.
//
// A gated run's GateSpec entries answer passed, failed or pending (GateResult.Pending, PassOnCap or MayHold only).
// An entry that sets GateEntry.Forked evaluates that closure in place of Gate for a run with fork authorization.
// The run's record carries it, copied from Spec.ForkSubagents at spawn;
// the record, not a later Call's Spec, also decides the fork audit.
// A failing result flagged GateResult.Terminal finalizes the run at once with no re-prompt,
// and its findings text rides GateOutcome.Reason.
// A failed outcome carries the failing entry's findings text on GateOutcome.Findings beside that terminal reason.
// Finalize removes the findings file with the run directory.
// A failed entry with budget remaining is re-prompted with one default line naming the findings file.
// An entry that sets GateEntry.Reprompt sends its rendered line instead.
// Every send validates its text.
// A renderer returning several lines, or an empty or whitespace-only line, fails the send.
// That ends the loop with the attempts spent so far.
// A re-prompt held by a busy session spends no attempt and is re-sent on a later tick.
// One held by a pane showing no input box maxNoInputBoxReprompts times in a row ends the loop.
// It ends with the attempts spent so far.
// So a picker shuttle cannot dismiss ends the wait instead of holding the run until its deadline.
// A pending answer holds the run at a turn boundary without re-prompting or counting a failure:
// the wait loop sends the entry's carried text once and keeps polling;
// it re-evaluates with no new arrival while the writer is idle at most once per gateRecheckEvery.
// The deadline and liveness checks keep running.
// A deadline or liveness finalize evaluates each entry's optional Final closure in place of Gate.
// It reports the entry waiting.
// An entry may also carry GateEntry.Expired, a read-only probe of whether its wait has run out.
// The writer may be off the boundary, mid-turn or attached with no boundary seen.
// Then the loop calls each probe once per gateRecheckEvery and finalizes the run done on the first that expires,
// so the bound ends the wait even when the writer never ends its turn;
// finalize then reads the entries once in list order through each Final,
// and a must-pass entry that fails first still fails the gate.
// Each GateEntryOutcome carries the GateSpan values its entry took on the run clock, in the order they started.
// They come in three kinds:
// GateSpanGate is one run of the entry's closure.
// GateSpanHeld is the time the entry was pending and held the run.
// GateSpanRework runs from a delivered re-prompt for the entry to the writer's next arrival, or to the run's end.
// While a held span is open no entry of the gate records a gate span, so no two spans of one entry overlap.
// Finalize ends the rework spans still open before its final evaluation and the held span still open after it.
// It stamps copies of the spans onto the outcome it returns, on every result that carries a gate.
// The spans live in the wait loop's memory like the failure counts, so an attached run starts them afresh.
//
// A turn end that leaves background work outstanding (EventWaiting) is judged in two rules.
// The rules read the output files and what is outstanding.
// A turn end whose outstanding tasks are all shells the session does not await is no work product's wait.
// Such a shell is one of either signal that no Spec.AwaitedShellPrefixes entry names.
// A gated run with an output file missing judges it as a plain turn end and holds it at once.
// A gated run with every output file present does not count it Done while such a shell runs.
// The shell may hold a commit or a gate run whose result the session never read.
// It types one line into the pane naming each such shell no sent line of the wait named.
// The line tells the agent to wait for each, read its result and act on it before it ends its turn;
// a send that fails is logged and not retried in place;
// the shell's completion starts the session's next turn anyway,
// and a line dropped under a pause or lost to a failed send is queued again by the next waiting turn end over the shell.
// It counts the turn end Done through the gate once every such shell has outlasted Config.BackgroundShellWaitMin.
// An ungated run with every output file present finishes Done at that turn end, whatever is outstanding;
// with an output file missing it keeps waiting until every such shell has outlasted Config.BackgroundShellWaitMin;
// it then holds the turn end once and notifies the parent once.
// A turn end with a fork or an awaited shell outstanding keeps waiting when an output file is missing.
// It is never held and never notified, and only the run's own Timeout and the liveness check bound it.
// lym reaps no shell.
// A gated run's waiting turn end on a fork or an awaited shell is never an arrival.
// The output files may predate the session's background work;
// the run waits until the task's completion starts the next turn, bounded by the run deadline and the liveness check.
// At the deadline classifyDeadlineExpiry ends the run OutcomeDone when the output files exist.
// Finalize evaluates the gate once more over the tree as it stands,
// and the shell lands in Result.EndedShells with its result never read by the run.
// Spec.AwaitedShellPrefixes keeps a shell out of the background-shell wait marker.
// It also keeps it out of the gated nudge and expiry and out of the ungated expiry;
// the prefixes are caller data, which Spec.validate does not inspect.
// Config.BackgroundShellWaitMin is the expiry of the gated Done's wait and of that ungated wait.
// It is also the threshold at which a shell outstanding that long is logged once at Warn and shown in the wait marker.
// The marker shows only while non-awaited shells are outstanding at staggered first-seen times.
// Result.EndedShells records each shell outstanding at run end, whatever ends it, whose result the run never read.
// An entry has the shell's label, id, signal and how long it was outstanding.
// Finalize logs the record at Info.
// lym kills no shell, since the provider reports no pid:
// strand removal ends the shells it can, and a shell detached from the pane's process tree survives it.
// ShellWaitBound exports the threshold.
//
// Wait shows each Go-side wait under a label.
// A gate entry's closure is `gate <entry name>`, the background-shell wait above is `background shells`,
// and a held turn end is `held`.
// It shows the label in two places:
// a WaitMarker file, `wait.yaml` in the run's own directory;
// the file carries the label, the start time and the pid of the process running Wait;
// and a pane mark that ReedOps.SetWaitMark puts on the run's strand.
// A running gate entry ranks first, then the background-shell wait, then the held wait.
// The held wait starts at a held turn end, stamped with the run clock's time.
// The next tick that reads a new event or a turn start past the held turn end clears it.
// So the following turn start, turn end or ask ends it.
// A held turn end read in the same batch as a turn start past it is logged and notified, and shows no hold.
// The next turn is already under way.
// ReadWaitMarker returns the first marker with a live pid among a run-directory root's runs.
// `lym loom status` reads it this way.
// ReadWaitMarkers returns every such marker, and WaitMarker.Held tells the held label,
// so the run notices' wait reading can skip a held wait:
// a held run is idle, and its quiet notice still fires.
// Both exist only for display and status: no shuttle decision reads either,
// and a failure to write, remove, set or clear one is logged and changes no verdict, error return, re-prompt or gate outcome.
// Wait clears the pane mark and removes the marker file on entry,
// so a mark a crashed step left behind is gone at the next touch, and again on every return.
//
// Agent activity: RunState records the pid of the process that waits on the run, which Start writes and an attach rewrites to the attaching process.
// A run counts as live when its outcome reads running and that pid is alive; a record without a pid, from an older binary, is not live.
// ReadAgentActivity returns one AgentActivity per run under a run-directory root whose record reads running, live or not, so an agent whose waiter died still reads as working.
// It logs a record that reads running with its recorded waiter gone at Warn once per episode per run directory, at Debug while it repeats and at Info once it reads live again, no longer reads running or has no record;
// such a run's reading carries its last activity only, with no API error or usage limit and WaiterGone set, since no process acts on its turn ends.
// Each reading carries the run's strand name, its last activity, and whether its newest turn end is an API error with the error's text, and its UsageLimit: whether the provider classifies that error as its usage limit, and when the limit resets as its message states.
// Runner.UsageLimit gives the same UsageLimit for one turn end of a named run, which the orch watcher reads at its own session's turn ends.
// The last activity is the newest of the session's last activity, its forks included, the time of the events file's newest Stop or ask line, and the modification time of the run's activity stamp, the file ActivityStampFileName names in the run directory, which an engine's hooks touch whenever the session works;
// it is the run's creation time when none is readable.
// A Stop or ask line carrying no time, an engine without the optional SessionSignalParser capability, and a file yielding no signals count the events file's last write instead, so an idle notice, a prompt submission or a stamp line never moves it.
// The events past the prompt offset are parsed through the Engine, and the newest turn end goes to the optional ActivityReader capability for the session part.
// With no turn end past the offset, the newest one before it, a skill-load turn end of the same session, goes instead, for the session's last activity only and never its API error,
// so a first turn spent in a fork reads active before it ends.
// An engine without the capability, and a run with no turn end at all, are judged by the events file alone.
// Like ReadWaitMarker it reads files only, so a process that runs no shuttle may call it.
// ReadNewestRunEnd returns when the newest run whose record reads anything but running ended, its record's modification time, so a caller that reads no running run still counts a session that has just ended as activity; it reads files only too.
// Every finalize also sets the run-end stamp, a file at the run-directory root, before a done run's cleanup removes the run directory, and ReadNewestRunEnd counts the stamp, so a removed run's end stays readable.
//
// Operator instructions carried over: every verified send into a run appends the text it types, as one JSON string line, to the run directory's sent-texts file before typing.
// A run's operator instructions are the SessionSignal.Instruction texts of its turn starts past its prompt offset, without the turn start of the run's prompt, and without any text the sent-texts file holds.
// RunState records a role key, the spec's NameOverride else its Role, and the instructions the run carried over.
// Before its orphan sweep, a start finds the newest run under the root with the same role key;
// when that run is unfinished, its record reading stopped, died, timeout or api-error or reading running with its waiter gone, its carried and operator instructions, without repeats, are appended to the new prompt as one block, recorded as the new run's carried instructions and named in one Warn log line and one notifier line.
// A done or live predecessor, an empty role key and a predecessor directory already swept carry nothing.
//
// Session state has two consumers: Runner.SessionIdle and the verified send's idle wait, which gate on the readiness reading built from it.
// The reading is ready or held, or unknown when the state is, and an unknown reading falls back to the pane probe.
// Runner.SessionIdle answers IdleProbe.Idle for a ready reading and, for a held one, IdleProbe.Reason, with TooShort set when the reason is the short pane and Draft set when the reading names a draft in the input box and NoInputBox set when the reason is a pane showing no input box to an engine that reads one;
// on the pane fallback, which a strand with no run record always takes through the engine of its live strand's provider, an answer that is not idle also sets Draft when the input box holds text and NoInputBox when an engine that reads an input box finds none on a pane tall enough to draw one;
// IdleProbe.NoTurn says no turn is in progress whatever the input box holds, so a draft, a pane too short or a missing box leaves it true while Idle stays false, and IdleProbe.TurnEndedAt says when the hook-derived reading behind it began;
// a reading with a running turn, an ask, a dead process or an unmatched turn start leaves both zero, and on the pane fallback NoTurn is the pane's idle answer or, for an engine with InterruptGuard, true when the box was found, the pane is not too short and no running turn shows, with TurnEndedAt zero;
// the runner keeps one turn-start hold per strand guid, so the orch watcher's probes keep one hold across its ticks as a send keeps one across its polls.
// A ready reading beside a pane that shows the needle still answers idle, and logs the disagreement with the pane's last lines at Debug.
// Wait's own shadow logging below stays display only.
// The optional SessionSignalParser capability reads the events file's hook lines as provider-neutral signals with hook-side times:
// a turn start, a turn end (with its outstanding background tasks), an API-error turn end, an ask, an idle notice, a session end and a session start (with its source).
// The time of the newest session start is read through RunSessionState.SessionStartAt, zero when none.
// A session start of source resume or clear sets idle-fresh with cause session-start, and so does one of source startup unless the derived state is a running turn;
// a compaction start, one with the empty source and a startup beside a running turn leave the state and the newest signal as they were, so an automatic compaction inside a turn keeps it busy.
// SessionFold reduces the signals in file order, with facts the caller reads, to one state: busy, idle-done, idle-stalled, idle-fresh, asking, dead or unknown, with a cause, a since time and the history of states passed through.
// Precedence, first that applies: a process-ending session end; a process proven dead; an unreadable events file; no signal at all; otherwise the state the signals give.
// Liveness left unproven settles nothing, so an engine whose prober never proves a process alive still reads the turn state its signals give.
// A turn end reads busy while it reports background tasks, idle-done when the run's output files exist, asking for an interactive run, and idle-stalled otherwise.
// The transcript's API-error marker turns the newest turn end into an API-error stall, and the interrupt marker turns a turn that is still open into an interrupt stall.
// The interrupt marker also releases an ask with cause ask, whichever signal raised it, to the same interrupt stall when the marker's time is not before the ask's, or either time is zero; an awaiting-input ask from a turn end is not released.
// A granted permission writes neither a turn signal nor a marker, so an ask holds every send until the next turn signal or an interrupt marker.
// Output files are consulted first only among the output-derived states, so dead and unknown outrank idle-done and a finished run can read dead:
// the first consumer that acts on dead applies PATTERN-shuttle-provider-seam's output check before acting.
// The facts and their sources: the record gives Interactive and the output files (a run that declares none has none that exist), taken once per read;
// the optional SessionProber gives process liveness for the session id of the newest signal that names one, else the record's, and the interrupt marker of the newest turn start;
// the optional ActivityReader gives the API-error marker of the newest turn end; the caller's clock gives the reading time.
// ReadSessionStates returns one RunSessionState per run under a run-directory root whose record reads running, whether or not its waiting process is alive, and Runner.SessionState reads the one run a guid names; Run.SessionState gives a run handle the same reading of its own strand.
// Both read the signals past the prompt offset and write nothing; an engine without the parser reads unknown with cause unsupported.
// The wait loop folds the same signals on a cursor of its own: it starts at the prompt offset on every path, never at the loop's event offset, and stops before an unpaired trailing stamp, so the stamp is read again with its payload.
// At every liveness tick it re-reads the markers of the newest turn start and turn end folded so far, since the transcript can mark them after the tick that folded them.
// Each tick, and once more just before a classified outcome is finalized, it logs every state change at Info as `shuttle: session state` with the loop's classification: running, waiting, held, done or died.
// The session state is a shadow reading the loop never acts on.
// The fold's states also reach the caller on Result.SessionStates, in order, as a record the loop never branches on.
// It logs `shuttle: session state disagrees` at Warn once per disagreement, again only after either side changed, when the pair is outside the fixed mapping, and the line carries `stands=loop`:
// waiting and running read busy, held reads idle-stalled, idle-fresh or asking, done reads idle-done, died reads dead.
// Two pairings are expected and log nothing: a gate wait with every output present beside idle-done, and done beside busy on background work.
// The loop never branches on the state; an engine without the parser, an unreadable events file or a failed fact read logs once at Debug and changes no verdict, notice, wait mark or return.
// Event.At is the hook-side time of the line an event was parsed from, zero when the engine gives none, and Runner.ReadEvents is the only place that sets it.
// For an engine with the parser, ReadEvents parses events from the bytes the signal reading consumed only and advances the offset by that count, so a stamp read ahead of its Stop payload stays unconsumed and is read with it on the next call.
// It pairs in file order each event with the signals whose Event flag is set and gives the event that signal's time;
// when the marked signals and the events do not pair one to one, every event of that read gets the zero time, so no Stop is stamped from another line.
// An engine without the parser keeps the last-newline offset rule and gives every event the zero time.
//
// Start, StartGated, Run and RunGated run the startup probe before issuing a handle:
// readiness plus dismissal of any one-time startup gate, through the Engine seam's startup classification and trust-dismiss sequence.
// No caller outside this package probes readiness or plays gate keys.
// A not-ready provider is torn down (strand removed, run dir and startup-capture.txt kept, Outcome persisted) and reported as ErrNotStarted from StartGated and as OutcomeDied/OutcomeTimeout from RunGated.
// The teardown tries the strand removal three times, waiting one and then two seconds on the run clock, before its error tells the operator to remove the strand by hand.
// Its error ends with the capture's last lines, and with its first lines before them when the capture is longer than one tail, so the first error line of a launch that scrolled away stays visible.
// A start whose teardown ended died with the strand removed, a provider that exited or a start-time send that failed before the first input, is launched again after five seconds in a fresh run directory, up to shuttle.yaml's start_retries extra times.
// A timed-out start, one whose strand could not be removed, so two agents never run for one start, and any failure before the strand registered are never launched again;
// the error a start returns is the last attempt's, and every failed attempt keeps its run directory and capture.
// RunGated's not-ready result carries the start error's text in Result.StartError.
// The startup probe starts at the poll interval and backs off to the dismissal gap (the poll interval times liveness_every_n_polls) while the pane does not change.
// Startup-gate dismissals are at least one dismissal gap apart.
// Runner.AcceptStartupGate is the one dismissal outside a run's own startup step, for a strand already live, such as a session added by hand:
// it captures the pane once, runs the engine's startup classification on it and plays the engine's gate-accept sequence for that capture only when the engine reads a gate.
// A ready or pending pane, and a gate whose accepting option the engine cannot locate, send nothing, and the call reports whether it sent any input.
// The engine is the one the run's record names, or the live strand's provider when there is no record.
// It plays only the provider's own accept sequence, on the strand the caller names, so it grants no permission the strand did not request;
// for claude that covers the folder-trust gate, which adds the strand's working directory to the operator's Claude trust list, and the bypass-permissions risk gate, shown only to a strand whose launch flags or settings already ask for bypass permissions.
// Its only caller is the llm smoke test's helper in internal/creelcli, and `TestProviderSeamImportRule` pins that no production file outside this package calls it; a production caller is a new decision.
// A launch that declares Launch.RecordsSessionStarts, recorded as RunState.RecordsSessionStarts, starts only once the events file holds a session-start line of any source beside the ready pane;
// until both hold the run stays not started and the startup error names the missing line or the unreadable events file, while gate detection and dismissal run as for any run.
// Such a launch is also started once its events file holds a session start followed by a turn start, checked before the pane capture and in start's probe only,
// so a blank or collapsed pane does not hold back a provider that is past every gate and working;
// Wait's probe of a not-yet-started run keeps the capture rule, since that events file may hold an earlier incarnation's lines.
// An engine that leaves the flag false, and a record an older binary wrote, start on the pane alone.
//
// Process death: for an engine with both SessionSignalParser and SessionProber, every liveness tick of Wait and of the startup step, once the strand reads live, asks whether the provider's process is gone behind the pane's shell.
// It is gone when the registry reads the newest session id dead and this wait read that id alive at an earlier tick;
// it is also gone when the events file ends, past its size at the wait's first liveness tick, in a process-ending session end and the registry does not read the id alive.
// An unproven reading, a session end at or before that first-tick size and a session end beside an alive reading end nothing, so an earlier launch's end never fails a resumed launch.
// A gone session ends the run died, or done when every output file exists, the way a dead pane does; a startup that finds it fails as died.
package shuttleengine
