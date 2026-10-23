// Package battenshed owns the task-worktree batten producers: creating the task worktree,
// seeding its own inner run, running that run inside it to a terminal state, landing it into the prime, and tearing it down.
// Any producer list may name them by reference, the same way internal/landingshed frames its own
// two producers.
// It also declares batten's config module, batten.yaml (config.go).
//
// Told-geometry tier: this package takes every absolute path it operates on from its caller and
// has no direct production import of internal/lyxcwd, per PATTERN-told-geometry.
// Its seam_enforcement_test.go enforces that membership mechanically.
//
// This package describes one repository throughout. It is not in the Fabric Vocabulary
// Invariant's owner set, so none of its identifiers, string literals, or comments may name either
// fabric-internal side -- write "the task worktree" and "the pair" instead of naming either side
// by name.
//
// The InnerRun row notices a child that left running, a driver strand found dead while the child runs, an agent run of a running child stalled on an API error, and a running child whose agents are all idle for the quiet window (notice_quiet_min), and it notifies through the injected Notify seam (notice.go).
// A notice is one line, sent once per episode, from inside the wait.
// A state-changed and a driver-dead episode ends when the child's state changes, and the state's since is the status file's modification time at the first sight of the child in it:
// a rewrite of the file in the same state does not move it, and only a batten start reads it from the file.
// The api-error and quiet notices need a running child with a live driver strand, and read the agents through the injected Activity seam at most once per notice_probe_s; a failed read sends neither.
// The api-error notice goes when the newest turn end of any live run is an API error and that run has not been active for two minutes since; it names the producer and the error text cut to one line, and takes precedence over quiet.
// The quiet notice goes when every live run has been idle for the quiet window and no verify marker, shuttle wait marker or parent lock wait record is live, and says how long the agents have been idle;
// with no live run found, it falls back to "no agent activity readable" counted from the later of the child's newest history entry and since.
// Both episodes are keyed by the newest agent activity, so they end when an agent is active again, with `since` on the line as content only.
// Both idle clocks measure awake time, read through the injected Awake seam: each sleep seen between two checks is an interval, and the time since an activity is net of the sleeps after it.
// A sleep under a minute still counts as idle, and the subtraction can delay a notice and never raise one earlier.
// The InnerRun row also reads the hub's pause record through the Pause seam, once per check.
// While it covers the slug, the row spawns, resumes and revives nothing, sends no notice for a running child, keeps a covered holder's parent lock and reports `held by the pause of <scope> since <paused_at>` as its wait reason;
// a child that halts still ends the wait with its state-changed notice, and batten's own pause is still honoured.
// The paused intervals leave the idle clocks, so after the resume a run idle for its window is noticed as before.
// Limits: a run whose pid is dead is not read, a run that keeps writing or hangs in a live wait is never quiet, and both notices are informational.
// A marker under the row's scratch directory records the notices sent, and only those posted, so a batten restart sends the notices not yet sent and never one already sent.
// A state-changed notice names what happened, the child's state and the row it stopped at, for example `awaiting at PR-Gate`.
// Every notice names the child's state and current row, and a quiet, an api-error and a driver-dead notice follow them with their own text, for example `running at Plan-Write; agents idle for 25m0s`.
// The child's reason is labelled `error:` for a failed or blocked child and `waiting:` for any other state.
// Every notice carries `since` and `history`, and a notice for a parked stop (blocked, paused, failed, awaiting) carries the driver's stop report, or `report none yet`.
// Every path under the task pair is rendered relative to it, since the slug already names the pair, and a notice carries no attach command.
// A parked stop's notice waits for the driver's stop report, for the driver strand to end, or for three minutes after `since`, whichever comes first, and an older report never qualifies.
// An awaiting child whose status carries a parent notice, while this batten holds the watched marker, is left to its driver's relay:
// its notice goes when the driver strand has ended, or three minutes passed with no report, or ten minutes passed since the report with no decision record.
// A live arbiter or tackler in flight holds the notice of a halted or awaiting child, so the orch is not told "report none yet" for a stop either may settle:
// the hold ends when the marker is gone, its strand is dead, or its deadline plus the report wait has passed, and the rules above decide again.
// The eligible-stop hold holds the notice of a blocked or awaiting child whose stop the arbiter may settle, read through the ArbiterStop seam, until the arbiter has handed on or no arbiter will settle it:
// it ends at the applied mark of the stop's attempt, at the moment the arbiter was seen off, at a park marker written after the stop (unless the Go driver handed the run over at the stop), at the attempt's hand-on, or at the backstop, the later of the stop plus one spawn's timeout plus the report wait and the attempt's deadline plus the report wait.
// A stop with the arbiter off or no spawns left and no attempt is never held, and error stops keep the marker hold alone.
// The report wait is then counted from the later of the stop and the hold's end.
// The start comes from the status file and every end from a record, so a batten restart neither resets the start nor loses the end;
// after the arbiter was switched off, a restarted batten takes its own first observation.
// When a resume moves the child back to running the episode ends and no notice goes; after a hand-on the marker is gone and the driver's stop report qualifies.
// A done child sends no notice: a landing that succeeds needs nothing from the orch.
// A resume the child refuses because its parked driver's pane holds a draft, an active operator or another busy reading (a spawn error wrapping ErrChildDriverBusy) is retried like a not-parked one, without a deadline, until the resume lands, the child leaves awaiting or batten is paused or stopped.
// Its wait reason names the draft and the remedy, and one notice per stall tells the orch to clear the draft in the driver's pane and quotes the box text the refusal found, as one line; a landed resume re-arms it.
// Delivery: the Notify seam reports whether the line was posted; with no orch strand recorded it is not called, one Warn is logged per episode and the strand is asked about again once per notice_probe_s;
// a Notify error or an unposted line is retried at most once per notice_probe_s and at most three times, then dropped with a Warn.
// Control flow is unchanged: a notice is informational, a delivery failure is only warned about, and the outcomes, halts and waits are the same with or without it.
// Bound: the report wait delays a notice by at most three minutes, the relay wait by at most ten minutes after the report, and an arbiter hold by at most the arbiter's deadline plus the report wait; none loses it, and there is one notice per episode.
// A driver that is alive but parked -- a provider waiting on an interactive prompt its launcher never answered -- or that stops of its own accord with the run still non-terminal is told apart from a working one only by the quiet window, or by an API-error notice when the stall is one; an operator attaches to the child's session to tell the cases apart.
//
// A batten run starts with "lyx batten run <slug>" in a terminal, or with "--window" in its own tmux window of the orch's reed session, which returns at once.
// The tmux window lives as long as the reed session, and a second batten for the slug is refused by the run's own driver lock inside the window, in batten's own log.
// "lyx batten run" steps the lifecycle through the Go driver, so an unexpected stop meets the driver's rule table and a cold tackler before the orch, and the driver lock refuses a second run for the slug.
//
// A resume, "lyx batten run" over an existing status file, first runs the origin reconcile of the slug's run scope.
// An entry origin ended refuses, naming the landing or the removal and "lyx cleanup --apply" from the prime; a stopped entry refuses, naming the branch, its state and the way forward for it, then the re-run;
// an entry another machine owns refuses, naming the owner and "lyx adopt <slug>"; an active entry resumes, and so does an unreachable origin, with a Warn.
// With origin answering, the resume first records this machine as the hub's prime owner when none is recorded, and as the owner of a run-held entry that has none; a failed write refuses, naming the re-run once origin answers or the push's own way forward.
// With origin unreachable the resume reads the local board instead, refuses a task it shows owned elsewhere, and writes no owner.
// A reconcile error refuses, with fixing its cause and then the re-run as the way forward.
// A fresh start runs no reconcile: creating the pair probes origin for it, and a parent behind origin costs only Merge-In's merge.
// It pulls the board instead, which needs origin, and refuses a task another machine owns before anything is seeded or claimed.
//
// Between Seed-Child and Run-Shed, three Command rows run one shell command each: Open-Window opens the operator's terminal window attached to the pair's reed session, Seed-IDE writes the pair's VS Code colour and .vscode/ config, and Open-IDE opens VS Code on the pair.
// Each runs its command only when its batten.yaml switch is on, open_window, seed_ide and open_ide in turn, of which only open_ide is off by default, and is Done either way, a failed command only warning.
// The switches are read from the pair's own config stack when the row runs, after Seed-Child has written the task's run settings into the pair's local layer, so a task's run_settings sets them per task.
// The commands are the open_window_command, seed_ide_command and open_ide_command keys, read from the template and the board default layer only, so a task's run settings and a worktree's own batten.yaml cannot change them;
// `lyx config batten --layer default --set <key>=<value>` from the prime changes one, and a different value in another layer is ignored with a warning.
// A command may hold the placeholders {lyx}, {slug} and {task_dir}.
// "--window" passes the hidden "--open-terminal", which turns Open-Window on for that run whatever batten.yaml says.
// A run resumed past Open-Window runs that command again when no client is attached to the pair's reed session, through the same switch, so a window closed or left on another machine opens afresh; a resume with a client attached opens none.
//
// The InnerRun row waits on its child inside its call rather than returning once per poll.
// Every poll_interval_s it stats the child's status file, and decodes it when its modification time differs from the last decode or notice_probe_s has passed.
// It returns a budget-exempt Stuck only when something is worth a history entry:
// the child's state changed, and the Stuck's Path and Reason name the change;
// batten's own status carries pause_requested;
// or the status file cannot be stat'ed.
// An arm's own event ends it too: an awaiting child's decision is acted on and then waited out, and a done child's driver ending or its grace elapsing returns Done.
// File stats and small file reads run on every check, the notice step's own reads (the decision record, the stop report) included;
// anything costing a process or a multiplexer round trip (the driver, review and agent-activity reads, the forced decode, a not-parked resume retry, the watched-marker refresh, a notice's delivery) runs at most once per notice_probe_s.
// Bound: a running child is waited on without a time limit, bounded by "lyx batten pause" (honoured within one check), cancellation and the notices;
// a running child whose driver is dead and whose status file does not change returns nothing from the wait, the driver-dead notice being the only signal.
//
// Seed-Child writes the task worktree's local config layer from the board task's run settings, after reading the child driver and before the seed:
// it reads the task's profile and settings fresh on every Call, lays the settings over the named run profile's from the hub's `profiles.yaml` (see `internal/runprofile`) key by key, writes the local file of each module they name, then removes the local file of every other registered module with one Warn per removed file, so the layer is exactly the task's run settings over its profile.
// The profile is read only here, so a later change of the profile does not reach a run already seeded.
// An unknown profile, or a key that no longer validates (a key a later binary retired or closed to the local layer, or one written to `board.json` or `profiles.yaml` by hand), stands the row Stuck, naming the profile or the module and the key, and the way forward, correcting the task's `profile` or `run_settings` with `lyx board upsert` or the profile with `lyx config profiles`, and then `lyx batten run <slug>`; an unreadable board is Stuck as for the type, and any other prepare, write or remove failure is a hard error.
// A resume redoes the whole write from the current board entry, and no other row writes these files.
// Bound: a local file written while the row stood Stuck is discarded by the resume, so a hand edit to the pair's local layer before the resume is lost.
// Batten writes no seed field for run settings.
//
// Seed-Child and the InnerRun row also keep the batten-watched marker of the task worktree's run (MarkWatched), which holds this batten's pid while its notices have a destination that is also the child driver's parent.
// Seed-Child asks once the seed is committed, so the child's first driver launch already renders the watched rule;
// InnerRun asks at the start of every Call and once per notice_probe_s inside the wait.
// A failure is a Warn, reads as not held, and never changes a verdict.
// Bound: only the driver's "run stopped" message is removed, and only where a live batten with a notice destination replaces it at render time.
// A driver launched unwatched and adopted later keeps its own rule, a duplicate and never a loss.
// A driver launched watched keeps the silent rule whatever happens to batten afterwards,
// so a batten that dies after the render, or an orch strand that disappears after it, leaves the run's stops unannounced until a batten runs again, which then sends the notices not yet sent.
//
// A running child whose spawn this batten process has not confirmed is spawned again,
// so a restarted batten brings a driverless child back up:
// the confirmation marker holds the pid of the process that wrote it,
// and a marker from another pid or in the old layout reads as unconfirmed.
// A child with a live or retiring driver strand in its reed state, a held run lock, or a held Go driver lock, is adopted instead, with the confirmation recorded for this process,
// so a second driver is never stacked.
// The driver lock is a liveness signal of its own: a Go driver holds it for life while the run lock is free between steps, so a running go child whose driver lock is held has a driver at work and sends no driver-dead notice, and one with neither lock and no strand is noticed as driver-dead.
//
// A child that halts (blocked, paused or failed) is a budget-exempt wait, not a failure of the Run-Shed row:
// batten never spawns or resumes a halted child, bar the binary re-step below, logs one Info line per halt episode, and keeps waiting with the child's state, error, current producer and the resume command ("lyx loom resume" in the task worktree) as its reason.
// A stuck verdict takes its log level from its cause: a failed producer step logs `producer stuck` at Warn, and an ordinary wait or move logs it at Info.
// A wait on the child logs at Info once per state of the child, so a reason that only counts up, such as the driver-exit grace, logs once while the reason file always holds the newest text.
// A halted or awaiting child whose reed state holds a dead driver strand has its pair's strands revived through reed resume, once per halt episode per batten process;
// the run stays halted and nothing is resumed,
// so the operator's "lyx loom resume" wakes the revived driver.
// A retiring strand is not revived and a child with no driver strand has none to revive;
// a revive that fails, and a child with no driver strand, make the reason also name "lyx loom start" in the task worktree.
// The wait has no time limit and spends no bounce budget; "lyx batten pause" stops it within one check, and the row reads the child as running again once the operator resumes it.
//
// A run halted at Merge-In or Finalize gets one state-changed notice like every other halt, and no goto: the recipe's own rows fix or hand back those failures.
//
// A halted go child is also re-stepped once per halt and per lyx build (innerrun_binaryrestep.go), so a fix deployed after a Go driver handed back lands without a parent turn.
// The child is re-stepped, through the same Spawn seam as every bootstrap, only when it is blocked or failed, its reed state holds no driver strand, neither its run lock nor its driver lock is held;
// the lyx on disk must differ from the build the child's last step recorded, and the task pair must have no uncommitted or untracked path outside the run's records.
// A blocked halt that `InnerRunDeps.HandBack` marks as a landing halt is skipped and writes no marker, so only the orch's `lyx loom resume` lifts it; every other halt keeps the re-step, so a fix deployed in a new build can still clear it.
// The `<row>-binary-restep` marker holds the halt's stop key and the build label, and is written only after the spawn returns success, so the re-step happens once per halt and build and survives a batten restart.
// A spawn refused as not parked or as a busy pane, and a dirty tree, write no marker and are tried again at the next due check; the spawns are capped at five per halt and build.
// Reading the build and the dirty paths costs a process, so the check runs once per interval:
// it starts at the notice probe, doubles up to thirty minutes while the halt and the build stay the same, and returns to the probe on a change.
// A pause covering the slug holds the re-step like every halted-arm action, a standalone go-driven run with no batten has no such watch, and a nil build or dirty-path seam switches the re-step off.
//
// With deps.ParentLock wired, the wait also looks after the parent lock of the child's run on every probe check (innerrun_parentlock.go).
// A record is released once every agent of its owner has waited for the idle bound with none busy, a watch that restarts whenever the owner changes; a failed agent read releases nothing.
// That release holds off while the child's run is at Finalize and running, or a verify of it is live, which keeps the record and restarts the watch; a failed read of either keeps the record and warns once per episode.
// Every release logs one Info, and batten releases only a record naming its own child's run.
// A polled seam that keeps failing, this lock step's included, warns at the first failure, logs Debug while it repeats and logs one Info when it recovers, and a retried spawn or goto logs Info on its first attempt and Debug on each retry.
//
// With deps.OwnerCheck wired, the wait also re-checks the run's owner (innerrun_owner.go), a judge-only reconcile of the run scope behind a seam.
// The check is due when the producer's own sleep watch records a sleep of a minute or more, when a pause that covered the slug on the previous check no longer does, and on a probe check while an owner stands that no pause covers.
// It runs after the check's status read and before any end that read gave, so a sleep and a state change seen together still run it; a failed stat or a vanished status file runs none and leaves it due for the next Call.
// An unreachable verdict and a check error keep it due and hold nothing.
// A verdict of another machine records that owner, releases the parent lock record naming the child's run, pauses the slug under the owner caller through deps.PauseOwned, and sends one notice per episode naming the owner and "lyx adopt <slug>".
// A failed pause warns once per episode and is retried at the next probe while the owner stands.
// A Finalize already past its parent-side merge keeps its record and finishes its landing: a Finalize running, or failed on a transient cause with no re-step recorded for that stop, while deps.ParentHoldsTaskWork reads true.
// A failed read of either seam keeps the record until the next check, and any other state at Finalize, blocked included, releases it.
// FinalizePastMerge is that decision over its seams as arguments, so a caller outside the wait reads the same carve-out; the owner pause's end spares such a Finalize and ends the processes of every other run.
// batten survives its own probe's end: the end spares the calling process and its ancestors, so batten keeps its owner hold and probe, and the pair's removal ends it.
// A running child whose driver the end took, with neither lock held and no strand, is noticed as driver-dead once the owner pause lifts and an owned-here verdict follows, and not while the pause covers it.
// A pause held for an owner releases the record the same way on a probe check, with no check of its own.
// Every spawn on an existing run reads a ChildOwnedElsewhereError as that verdict: the respawn of an unconfirmed running child returns a budget-exempt Stuck naming the owner, and the awaiting resume, and the binary re-step pause, write no marker and keep waiting.
// While an owner stands every spawn waits, and the spawn hold re-runs the owner check once per probe until it reads the run as owned here, so a failed owner pause never makes the wait spawn and re-reconcile in a loop.
// Bound: the child can act for up to one check interval after a wake before the check runs, and for as long as origin stays unreachable.
// A Finalize past its parent-side merge finishes its landing here, and once it is done its batten waits under the owner pause until "lyx resume <slug>" lifts it, then goes on to Land.
//
// Its counterpart is equally by design: a driver that finishes NORMALLY leaves its strand and its
// run directory behind. Nothing here tears either down as part of a clean finish -- only the
// whole-worktree teardown row cleans up, at the very end, by removing the worktree they live in.
// The done arm does wait for the driver strand, up to the driver-exit grace window, so the driver can finish its stop report (loom's post-run friction reflection included); a driver still alive past the window is ended by the teardown's session shutdown and recorded as an abandoned session.
//
// Worktree-Teardown's two halves, session shutdown and worktree removal, call the pair-teardown composite (internal/pairteardown) as its EndSession and RemovePair phases.
// Session shutdown no longer skips a gone task worktree, since the composite ends that session by name.
// A pair of which nothing remains is the done post-condition for both halves: session shutdown has nothing left to end, and the removal reports the pair not found.
//
// The two producers that hold the hub's prime lock, Worktree-Create and Worktree-Teardown, wait for a contended lock instead of halting on the first try:
// they poll it every two seconds for up to ten minutes, stop at once when the context is cancelled, and return Stuck only when the bound is spent (primelockwait.go).
// Worktree-Create halts, blocked at its own row and with no bounce budget spent, when the create closure returns a PreconditionError, for a precondition no tackler can settle:
// the halt's reason starts with the precondition-unmet prefix and names what is unmet and its way forward, and the Go driver hands such a halt back.
// battenshed never imports fabric, so the closure alone builds the error.
//
// The Land row lands a child whose Run-Shed returned Done, in this order, each step idempotent so a resume re-runs the row from the top:
// it takes the hub-level land lock for its whole body, polling a contended one from one second doubling to thirty with no time limit;
// it checks the prime, where a child whose parent branch is not the prime's branch skips the pull and deploy and a prime with uncommitted tracked paths is waited on for ten minutes;
// it pulls the prime under the prime lock and releases that lock at once, since batten never pushes the prime;
// outside the prime lock it runs the landing config's hub-wide `deploy` command in the prime, for a parent branch in `deploy_branches` only, with a thirty-minute timeout and its output in the row's deploy log;
// it writes the ready snapshot before pruning the board, so the tasks the prune made ready are named exactly once across a restart; and it posts one landing notice per batten run, naming the tasks the prune made ready when there are any.
// The two locks differ in what they hold: the land lock serialises whole landings, the prime lock only the pull.
// A failed step is Stuck naming it, with one failure notice per episode.
// A pause seen during the land-lock or dirty-prime wait halts the row blocked with no notice, rather than paused, since the row has no on_stuck route to yield through;
// shed's blocked persist leaves the pause flag set, so the way back is `lyx batten goto <slug> --to Land`, which clears it, then `lyx batten run <slug>`.
//
// The rows after Run-Shed send the orch a failure notice through a row notifier (rownotice.go): Worktree-Teardown on every failure it returns, and the Land row on the same terms; the Land row also sends its landing notice there, on success.
// A notice goes once per failure episode, an episode being one entry of the row from its predecessor's Done or from a halt at that row, told apart by the length of batten's own status history;
// the row's `<row>-row-notice` marker keeps the length of the last noticed failure, so a re-entry that appended nothing sends nothing, and a batten that died inside the row never repeats a posted notice.
// A notice is tried at most three times, thirty seconds apart, and dropped with a Warn after that, leaving the failed row halted and visible in `lyx batten status`.
// Every marker that must last one batten run is named by a suffix in runMarkerSuffixes, and batten clears them all through ClearRunMarkers where it seeds a fresh status.
//
// It declares its own unexported entryErr/cancelErr helpers (ctx.go) and its own reportStuck
// carrier (stuck.go) for the same deliberate-duplication reason internal/preflightshed/doc.go and
// internal/landingshed/stuck.go already record: each producer-owning package carries its own copy
// rather than sharing one across packages it otherwise has no reason to depend on.
package battenshed
