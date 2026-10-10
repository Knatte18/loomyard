// Package shuttleengine runs one LLM agent as an interactive session and returns its result.
// It is the unit review and loom call once per spawn: "run this producer / handler /
// progress-judge, give me back its output files."
// shuttle owns which provider (via an engine), the prompt envelope, and what "done" means.
// It does not own panes, layout, or tmux mechanics — it asks reed for a strand and drives the LLM
// in it.
//
// Every agent runs as an interactive tmux session, never headless `claude -p` — an economic
// constraint (subscription coverage), not a technical one.
// This is why the whole proc -> reed -> shuttle stack exists instead of a plain headless exec.
//
// shuttle runs a provider through an engine: a per-LLM adapter that knows how to launch and drive
// its provider as a tmux session — construct the launch command, inject the prompt, recognize the
// completion edge, locate the output.
// A Claude engine now;
// Gemini etc. later.
// The verdict/output contract is provider-invariant, which is what makes engines swappable:
// shuttleengine defines the Engine interface and its value types, and never imports a concrete
// engine implementation (the provider-seam import rule, enforced by seam_enforcement_test.go in a
// later batch) — concrete engines import shuttleengine, not the reverse.
//
// shuttle is told its anchor path and worktree root as plain strings, at Runner construction, and
// derives neither — internal/lyxcwd is consequently absent from the package's production imports.
// Told, however, does not mean unchecked: because the two are adjacent parameters of one type whose
// consumers are semantically distinct, construction validates the pair and every public method
// refuses on an unusable one, so a swapped or relative pair fails loudly instead of succeeding
// against the wrong tree. There are two constructors, one rule each: NewRunner validates its pair
// (absolute, non-empty, anchor inside-or-equal worktree root) for hub geometry, where the anchor is
// always the worktree root or a subdirectory of it; NewDetachedRunner validates its own triple
// (absolute, non-empty, anchor and worktree root strictly disjoint) for standalone geometry, where
// the anchor is deliberately outside the worktree root it names.
//
// The only channel in and out of a shuttle run is files: the prompt is handed to the provider as
// the launch argument (never typed into a live pane),
// and the agent writes its structured result to a file the caller reads.
// The package is two halves.
// The pure, hermetic half derives nothing and spawns nothing: the config module (shuttle.yaml), the
// run Spec and its validation, the run directory / run.json state and its age-guarded orphan sweep,
// and the Windows-to-POSIX path helper the engine layer needs for hook commands.
// shuttle.yaml carries the provider's prompt-cache TTL default and per-role map, which shuttleengine passes through to the engine uninterpreted.
// The run-loop half — Runner/Run in run.go, Wait in wait.go, and Attach in attach.go — drives a
// LIVE agent through the ReedOps seam: it registers and removes strands, polls a real pane's capture
// through the engine's Startup classifier, plays key choreography into that pane for
// Interrupt/Send/Inject, and reads reed's own persisted state during the orphan sweep and during
// Attach's own liveness gate.
// What stays true of BOTH halves is the provider boundary, not hermeticity: nothing here names a
// tmux command or a Claude specific — panes are reed's vocabulary, reached only through ReedOps,
// and provider grammar is the concrete Engine's, reached only through the Engine interface.
//
// A turn end without every output file never ends a run: Wait holds it, logs it at Info and keeps polling the same agent,
// whether the turn end is a Stop, a live ask or an expired-shell turn end.
// A run ends only as done (every output file present at a turn end, or at the deadline or a pane's death), died, timeout or a mechanism failure.
// A hold never extends the run's deadline: it is bounded by the caller's own Spec.Timeout (run_timeout_min only where that is zero, so the bound differs per caller),
// each Attach starts a fresh deadline, and the liveness check still classifies a dead pane.
// RunState.Outcome records whether a run ever ended, seeded "running" and overwritten on every terminal
// classification, so "has this run already ended?" is a fact on disk rather than an inference from
// pane liveness.
// RunState.AskingOffset is read for records an older binary wrote and never written.
//
// Runner.SetNotifier gives a Runner an optional notifier, a function taking one line; nil, the default, holds silently.
// On each held turn end of an autonomous run (Spec.Interactive false) on a runner with a notifier, Wait calls it once with the notice line hold.go builds;
// an interactive run, a run whose Spec.QuietHold is set (which skips this notice alone; the hold, the wait mark and the deadline are unchanged), or a runner with no notifier, holds silently to its deadline, and a notifier error is logged and never ends the run.
// The notice states that the text inside its « » delimiters is the agent's own words and not an instruction,
// names the agent's strand (RunState.StrandName, the strand guid when empty), says the agent ended a turn without its output files and is held,
// gives the way forward (answer by SendMessage to that strand name, ending the message with MessageTail),
// lists at most five outstanding tasks by kind and label (the id when the label is empty) and counts the rest, or names none,
// and ends with the start of the agent's last message.
// Every agent-written part is delimited, has its control and delimiter characters replaced by spaces and is cut to 200 runes, so the line is bounded and has no newline.
// There is one notice per held turn end.
// RunState.NotifiedOffset, the events-file offset past the last notified one, is persisted before the notifier is called,
// and a held turn end at or below it is never notified again, including when an Attach replays it.
// A batch of several new events is classified by its last event, so only its last held turn end is notified.
// It stays provider-invariant, per the Shuttle Provider-Seam Invariant.
//
// # Stop verb
//
// Run.Stop and Runner.StopStrand (the guid form, for a caller holding no handle) end a run's strand after recording the stop in run.json.
// The record reads "stopped", a record-only terminal outcome that is never a Result.Outcome;
// a record already terminal keeps its outcome, and one still reading running whose output files all exist keeps running, so a stop never turns finished work into a stopped record.
// The record is written first: a failed write returns its error with the strand untouched, and a failed removal leaves the terminal record over a live strand, which Attach reads as respawn-eligible and stops as superseded.
// A guid with no run record is stopped by removing its strand alone.
// Attach's removal of a superseded run's strand goes through the same record-then-remove step.
// A Wait on a stopped handle finalizes with the stop mark and records "stopped" unless the files show the run done.
// A Wait in another process may overwrite "stopped" with its own terminal outcome, and either answer is terminal.
//
// A Run's record lock guards the stop mark, RunState.Outcome and every save of run.state, so a stop from another goroutine never interleaves with Wait's own record writes.
// Shuttle's teardowns inside the run lifecycle (finalize's cleanup, abandonStartup) write the record themselves and do not go through the verb.
//
// Spec.PermissionMode, Spec.AllowAgentTool and Spec.ResumeSessionID are caller-owned engine vocabulary, like Spec.Effort: Spec.validate never inspects them,
// and the engine is the sole validator and realizer.
// Empty PermissionMode keeps the run mode's default, AllowAgentTool left false keeps shuttle's config-driven Agent deny,
// and an empty ResumeSessionID mints a new session, so a caller that sets none of them sees no change.
// A non-empty ResumeSessionID launches that existing session instead.
//
// An ungated run's strand is registered with its output files as reed's done-when list,
// so reed's resume drops a finished agent strand instead of reviving it;
// a gated run's strand carries none,
// since its outputs existing does not mean its gate passed,
// and the gate re-prompts the live agent.
// The list is provider-neutral, set by the runner and never by a provider engine.
//
// Runner.Attach answers one question: is there a still-live-or-already-finished, never-terminated
// run for this exact output-file set, and if so, wait on it instead of starting a second agent.
// That question is answered on the persisted RunState.Outcome plus, in a fixed precedence, the run's
// own file contract and then reed's live-agent evidence — never on output-file existence at the
// CALLER's level, which is the shortcut the Completion Signal Invariant warns
// against, since a bounce that re-runs a producer over already-present files looks identical to a
// genuinely finished run from the caller's side. Inside Attach the same file-existence question is
// safe, and for the reason that ladder gives: it is asked only of a matched run.json still declaring
// itself "running", which is an agent to attribute the files to and which a bounce never leaves
// behind.
// The three signals are a deliberate defence-in-depth framing, not redundancy. The persisted record
// answers "did this run end"; the file contract answers "did its agent finish"; reed answers "is the
// process there" — and they fail in different directions. A crash mid-run leaves the record at
// "running" with reed still tracking a live pane; a crash AFTER the agent wrote every output file
// leaves the record at "running" with reed's answer irrelevant, which is why the file contract
// outranks reed rather than following it (and why an unreadable, absent, or unanswerable strand
// table still harvests such a run rather than refusing — see attach.go's soleFinishedCandidate);
// a corrupted or torn-down reed session leaves the record honest but the process unreachable.
// Attach reconstructs its *Run explicitly, without ever calling Start, so sweepOrphansOpportunistic
// never runs on the attach path; a caller must therefore probe Attach before anything else that could
// sweep the very directory it is looking for.
// Two of Attach's dispositions are errors rather than found == false, because neither is evidence a
// run has ended: an absent or unreadable reed state file, and a Status() failure. Both name the run
// directory and the strand guid in a logged warning, since the operator's only escape from either is
// out of band, via "lyx reed status".
// A run held at a turn end keeps outcome "running", so the same probe finds it.
// A record an older binary left at outcome "asking" is attached too when reed tracks its strand as live,
// with or without an AskingOffset and whether or not its events grew; with a dead or untracked strand it is classified as before.
// The attach resets such a record to "running", replays from its AskingOffset (the prompt offset when it has none)
// and counts the old ask as notified, so only a held turn end after it notifies the parent.
// Only a live, tracked strand is attached, and the attach starts a fresh deadline.
// An attached record with no StrandName takes the name reed's status reports for its guid.
// A not-found answer is the caller's cue to start a fresh run,
// so Attach and AttachGated first remove, with a logged warning, the live strand of every earlier run of the same output-file set that the probe judged respawn-eligible:
// re-running a producer supersedes the halted session.
// The removal is bounded to exactly that set,
// so another producer's strand, a same-role strand with other outputs and a run the probe would attach to are never touched,
// and a probe that attaches or refuses removes nothing.
// A strand that cannot be removed comes back as an error naming `lyx reed remove <guid>`, never as a not-found answer.
// Runner.AttachIfLive is the same probe with the removal off, for a caller that waits on a live run and starts nothing after a not-found answer.
// Runner.ProbeGated is AttachGated's probe returning the reconstructed *Run unwaited, so a caller can hold two probed runs and wait on both concurrently.
//
// Session usage: the optional UsageReader capability reads a finished session's tokens, its forks included, as a provider-neutral SessionUsage.
// Run.finalize fills Result.Usage from it for a done run, and Result.StartedAt and Result.EndedAt for every run;
// an engine without the capability leaves Usage unknown, and nothing about the outcome changes.
//
// Segment color: Spec.Segment names the loom segment the spawning module gives its role, and Runner.start forwards it to reed, which resolves the segment's palette color on the strand it returns.
// After startup confirms the provider ready and before the skill load, start plays Engine.ColorSequence for that color into the strand's pane;
// an engine without a color command, a strand without a color and a spec with ColorByCaller type nothing.
// The play then waits, at most colorSettleAttempts pane reads, until the provider has consumed the command:
// the pane classifies StartupReady and the input box (InputBoxReader) no longer holds the command,
// so the next step never finds the command still sitting in the box.
// A command still in the box after the last read is submitted again through the verified send's extra Enters.
// A failed play and an unconfirmed command are logged and never fail the launch, since the color is display only.
// Runner.TypeColor plays the same sequence, with the same wait, into a live shuttle strand for a caller that colors its own session.
// The palette-to-command mapping is provider grammar and lives in the engine.
//
// Skill loading: Spec.Skills names provider-neutral skills that shuttle loads into a fresh session, all in one turn, before the prompt.
// It needs the optional SkillLoader capability, and a spec that names skills on an engine without it is refused before any run directory exists.
// An engine that realizes skills leaves the prompt pointer off its launch line and returns it as Launch.PromptLine;
// once the provider is ready, shuttle sends the engine's one load message for the whole list through the verified send path and polls until the first turn end reaches the events file or Spec.SkillLoadTimeout passes (zero means the engine's DefaultSkillLoadTimeout).
// The engine classifies that turn end into a SkillLoadReport: each unknown skill is skipped at once with a logged warning,
// and the missing ones are asked for once more in a second load turn naming only them,
// whose still-missing skills are skipped with a logged warning.
// A turn whose evidence the engine could not read is confirmed unverified with one logged warning and no retry,
// and a turn that never ends skips its skills at the timeout with no retry.
// So a launch runs at most two load turns, each bounded by the timeout,
// and a skipped skill never fails or hangs it.
// Then PromptLine goes out through the verified send path.
// Every verified send first waits for an idle session, for at most `send_ready_timeout_s` and never past the run's deadline, and types nothing until then.
// For an engine with the optional SessionCycler idle reading, every poll takes the readiness reading, built on the hook-derived session state.
// The reading is ready on idle-done, idle-stalled of any cause, asking at an awaited turn end and busy on background work, provided no turn start is unmatched and the input box, where the engine reads one, is blank;
// it is held, with a reason, on a running turn, a question awaiting an answer, a dead process, an unmatched turn start, a draft in the box and a pane too short.
// A ready reading lets the send type even when the pane still shows the running-turn needle or no box.
// An unmatched turn start releases after the pane has read idle on every poll for turnStartIdleOverride, or at once when the engine reports that turn interrupted.
// When the session state is unknown, the reading falls back to the pane, which must classify ready and idle, and for an engine that parses session signals no turn start may be left unmatched by a later turn end; the cause is logged at Debug.
// A session that stays held fails the send with ErrSessionBusy, naming the reading and ending with the pane's last lines.
// Run.Send and the gated wait loop's own send each hold the run's send lock for the whole idle wait, typing and delivery check, so an in-process send and a gate re-prompt on one run type one after the other, never at once; the lock skips no idle wait, delivery check or gate evaluation.
// An engine without the idle reading keeps the not-ready refusal alone.
// That path confirms a send is submitted, not only that its text appeared in the pane, for an engine that implements the optional InputBoxReader capability, inside a window of `submit_confirm_timeout_s` from the moment typing begins.
// The text is typed without its Enter, and once it has appeared shuttle reads the input box every `submit_settle_ms` until two reads agree, so no Enter lands inside a typing burst;
// reads that agree only once the window has closed send no Enter.
// It then sends the Enter and reads the box at an interval that starts at `submit_redraw_settle_ms` and doubles up to confirmBackoffCap.
// While the box still holds the sent text and the window is open, each read is followed by one more Enter.
// Every idle poll, box read and Enter runs a tmux process through reed, so each of these loops also stops at an attempt count derived from its window, which ends it under a clock that never advances.
// The submission is confirmed when the box no longer holds the sent text, or when the engine's session signals show a turn start past the file's size before the send.
// A send that does not land fails with ErrSubmissionNotLanded, naming why and ending with the pane's last lines.
// An empty box then means the submission landed late and the send succeeds;
// otherwise, for an engine with both the idle reading and the optional InputBoxClearer capability, a box that shows this send's own text, or the provider's collapsed paste placeholder, is cleared and read again;
// a box holding anything else gets no key, and a box the clear did not empty is named in the error.
// An engine without the capability keeps the appearance-only check.
// The run's events offset ends past every load turn end, the retry's included,
// so Wait never reads one as a held turn end.
// run.json records that offset as `promptOffset` before the prompt goes out,
// and every reader that replays the events file without Waiting on the Run starts there: Attach, and webster's recovery classification through its batch record.
// A pane that dies meanwhile is a died startup.
// An empty Skills list changes nothing.
// Runner.LoadSkills and Runner.ClassifySkillLoad are the same capability's per-tick primitives for a caller reloading a live session: the first sends the load message for a list, the second classifies the turn end that followed.
// Neither carries provider command text.
//
// A gated run's GateSpec entries answer passed, failed or pending (GateResult.Pending, PassOnCap or MayHold entries only).
// A failing result flagged GateResult.Terminal finalizes the run at once with no re-prompt, and its findings text rides GateOutcome.Reason.
// A pending answer holds the run at a turn boundary without re-prompting or counting a failure:
// the wait loop sends the entry's carried text once, keeps polling, and re-evaluates on poll ticks while the writer is idle.
// The deadline and liveness checks keep running, and a deadline or liveness finalize evaluates each entry's optional Final closure in place of Gate, reporting the entry waiting.
//
// A turn end that leaves background work outstanding (EventWaiting) keeps the run waiting, with one bound for transcript-reported shells.
// A shell the turn-end payload itself reported (SignalPayload) is live work and never expires: it keeps the turn waiting, never held and never notified, bounded only by the run's own Timeout and the liveness check, and lyx reaps no shell.
// Once it has been outstanding for Config.BackgroundShellWaitMin minutes the wait logs that once at Info, which changes no decision.
// A gated run whose output files all exist and whose outstanding tasks are all unawaited payload-reported shells finishes Done at once, through the gate, with those shells in Result.ExpiredShells;
// an outstanding fork or awaited shell keeps even that run waiting.
// An ungated run with every output file present finishes Done at any turn end, whatever is outstanding.
// For any other shell, those the transcript fallback reported or a record with no signal, once every outstanding task is a background shell and each has been outstanding for Config.BackgroundShellWaitMin minutes (stamped when first seen, kept across ticks),
// the wait loop counts the turn end as a Stop would: OutcomeDone when every output file exists, otherwise a held turn end naming the expired shells.
// A gated run reaches its gate through the Done branch, so the expiry is an arrival.
// A fork in the list keeps the turn waiting however long it runs, as does a shell whose label starts with one of Spec.AwaitedShellPrefixes;
// both are bounded only by the run's own Timeout.
// The prefixes are caller data, which Spec.validate does not inspect.
// A shell once waited out stays expired for the rest of the run, so a later turn end listing it again ends at once.
// Result.ExpiredShells names the waited-out shells' labels in expiry order, and each expiry is logged as a warning.
//
// Wait shows its three Go-side waits, a gate entry's closure (`gate <entry name>`), the background-shell wait above (`background shells`) and a held turn end (`held`), in two places:
// a WaitMarker file, `wait.yaml` in the run's own directory, carrying the label, the start time and the pid of the process running Wait,
// and a pane mark that ReedOps.SetWaitMark puts on the run's strand.
// A running gate entry ranks first, then the background-shell wait, then the held wait.
// The held wait starts at a held turn end, stamped with the run clock's time, and the next tick that reads a new event clears it, so the following turn start, turn end or ask ends it.
// ReadWaitMarker returns the first marker with a live pid among a run-directory root's runs, which is how `lyx loom status` reads it.
// ReadWaitMarkers returns every such marker, and WaitMarker.Held tells the held label, so batten's wait reading can skip a held wait: a held run is idle, and its quiet notice still fires.
// Both exist only for display and status: no shuttle decision reads either,
// and a failure to write, remove, set or clear one is logged and changes no verdict, error return, re-prompt or gate outcome.
// Wait clears the pane mark and removes the marker file on entry,
// so a mark a crashed step left behind is gone at the next touch, and again on every return.
//
// Agent activity: RunState records the pid of the process that waits on the run, which Start writes and an attach rewrites to the attaching process.
// A run counts as live when its outcome reads running and that pid is alive; a record without a pid, from an older binary, is not live.
// ReadAgentActivity returns one AgentActivity per live run under a run-directory root:
// the run's strand name, its last activity, and whether its newest turn end is an API error with the error's text.
// The last activity is the newer of the transcript's last write and the time of the events file's newest Stop or ask line, the run's creation time when neither is readable.
// A Stop or ask line carrying no time, an engine without the optional SessionSignalParser capability, and a file yielding no signals count the events file's last write instead, so an idle notice, a prompt submission or a stamp line never moves it.
// The events past the prompt offset are parsed through the Engine, and the newest turn end goes to the optional ActivityReader capability for the transcript part;
// an engine without it, and a run with no turn end yet, are judged by the events file alone.
// Like ReadWaitMarker it reads files only, so a process that runs no shuttle may call it.
//
// Session state has two consumers: Runner.SessionIdle and the verified send's idle wait, which gate on the readiness reading built from it.
// The reading is ready or held, or unknown when the state is, and an unknown reading falls back to the pane probe.
// Runner.SessionIdle answers IdleProbe.Idle for a ready reading and, for a held one, IdleProbe.Reason, with TooShort set when the reason is the short pane;
// the runner keeps one turn-start hold per strand guid, so the orch watcher's probes keep one hold across its ticks as a send keeps one across its polls.
// A ready reading beside a pane that shows the needle or no box still answers idle, and logs the disagreement with the pane's last lines at Debug.
// Wait's own shadow logging below stays display only.
// The optional SessionSignalParser capability reads the events file's hook lines as provider-neutral signals with hook-side times:
// a turn start, a turn end (with its outstanding background tasks), an API-error turn end, an ask, an idle notice, a session end and a session start (with its source).
// The fold does not move on a session start: the reading stays as it was, and the time of the newest one is read through RunSessionState.SessionStartAt, zero when none.
// SessionFold reduces the signals in file order, with facts the caller reads, to one state: busy, idle-done, idle-stalled, asking, dead or unknown, with a cause, a since time and the history of states passed through.
// Precedence, first that applies: a process-ending session end; a process proven dead; liveness left unproven; an unreadable events file; no signal at all; otherwise the state the signals give.
// A turn end reads busy while it reports background tasks, idle-done when the run's output files exist, asking for an interactive run, and idle-stalled otherwise.
// The transcript's API-error marker turns the newest turn end into an API-error stall, and the interrupt marker turns a turn that is still open into an interrupt stall.
// Output files are consulted first only among the output-derived states, so dead and unknown outrank idle-done and a finished run can read dead:
// the first consumer that acts on dead applies PATTERN-completion-signal's output check before acting.
// The facts and their sources: the record gives Interactive and the output files (a run that declares none has none that exist), taken once per read;
// the optional SessionProber gives process liveness for the session id of the newest signal that names one, else the record's, and the interrupt marker of the newest turn start;
// the optional ActivityReader gives the API-error marker of the newest turn end; the caller's clock gives the reading time.
// ReadSessionStates returns one RunSessionState per run under a run-directory root whose record reads running, whether or not its waiting process is alive, and Runner.SessionState reads the one run a guid names.
// Both read the signals past the prompt offset and write nothing; an engine without the parser reads unknown with cause unsupported.
// The wait loop folds the same signals on a cursor of its own: it starts at the prompt offset on every path, never at the loop's event offset, and stops before an unpaired trailing stamp, so the stamp is read again with its payload.
// At every liveness tick it re-reads the markers of the newest turn start and turn end folded so far, since the transcript can mark them after the tick that folded them.
// Each tick, and once more just before a classified outcome is finalized, it logs every state change at Info as `shuttle: session state` with the loop's classification: running, waiting, held, done or died.
// It logs `shuttle: session state disagrees` at Warn once per disagreement, again only after either side changed, when the pair is outside the fixed mapping:
// waiting and running read busy, held reads idle-stalled or asking, done reads idle-done, died reads dead.
// Three pairings are expected and log nothing: a gate wait with every output present beside idle-done, a held turn end after transcript-reported shells expired beside busy on background work, and done beside busy on background work.
// The loop never branches on the state; an engine without the parser, an unreadable events file or a failed fact read logs once at Debug and changes no verdict, notice, wait mark or return.
//
// Start/StartGated/Run/RunGated run the startup probe (readiness plus dismissal of any one-time
// startup gate, through the Engine seam's startup classification and trust-dismiss sequence) before
// issuing a handle, so no caller outside this package probes readiness or plays gate keys.
// A not-ready provider is torn down (strand removed, run dir and startup-capture.txt kept, Outcome
// persisted), reported as ErrNotStarted from StartGated and as OutcomeDied/OutcomeTimeout from
// RunGated.
package shuttleengine
