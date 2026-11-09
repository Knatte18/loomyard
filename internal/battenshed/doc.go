// Package battenshed owns the task-worktree batten producers.
// The producers create the task worktree and seed its own inner run.
// They run that run inside the worktree to a terminal state, land it into the prime, and tear the pair down.
// Any producer list may name them by reference.
// The package also declares batten's config module, batten.yaml (config.go).
//
// Told-geometry tier: this package takes every absolute path it operates on from its caller,
// and it has no direct production import of internal/lymcwd, per PATTERN-cwd-resolution.
// Its seam_enforcement_test.go enforces that membership mechanically.
//
// This package describes one repository throughout.
// It is not in the Fabric Vocabulary Invariant's owner set.
// None of its identifiers, string literals, or comments may name either fabric-internal side;
// write "the task worktree" and "the pair" instead.
//
// The InnerRun row notices a child that halted or awaits a hand-off.
// It also notices a halted or awaiting child whose dead driver strand a revive failed to bring back.
// It notifies through the injected Notify seam (notice.go).
// The dobby that drives the run, never the row, sends a running child's notices:
// a dead driver, an API-error stall, a usage limit and idle agents.
// A notice is one line, sent once per episode, from a pass's check.
// An episode ends when the child's state changes.
// The state's since is the status file's modification time at the first sight of the child in it:
// a rewrite of the file in the same state does not move it, and only a batten start reads it from the file.
// The InnerRun row also reads the hub's pause record through the Pause seam, once per check.
// While it covers the slug, the row spawns, resumes and revives nothing, keeps a covered holder's parent lock,
// and it reports `held by the pause of <scope> since <paused_at>` as its wait reason;
// a child that halts still ends the pass with its state-changed notice, and batten's own pause is still honoured.
// A marker under the row's scratch directory records the notices sent, and only those posted;
// so a batten restart sends the notices not yet sent and never one already sent.
// A state-changed notice names the child's state and the row it stopped at, such as `awaiting at Plan-Bouncer`.
// A driver-dead notice adds its own text, such as `blocked at Publish; driver strand could not be revived; …`.
// The child's reason is labelled `error:` for a failed or blocked child and `waiting:` for any other state.
// Every notice carries `since` and `history`.
// A notice for a parked stop (blocked, paused, failed, awaiting) carries the driver's stop report.
// Without one it carries `report none yet`.
// Every path under the task pair is rendered relative to it, since the slug already names the pair,
// and a notice carries no attach command.
// A parked stop's notice waits for whichever comes first:
// the driver's stop report, the end of the driver strand, or three minutes after `since`.
// An older report never qualifies.
// While batten holds the watched marker, an awaiting child with a parent notice goes to the driver's relay:
// its notice goes when the driver strand has ended;
// it also goes when three minutes passed with no report;
// it also goes when ten minutes passed since the report with no decision record.
// A live arbiter or tackler in flight holds the notice of a halted or awaiting child,
// so the orch is not told "report none yet" for a stop either may settle:
// the hold ends when the marker is gone, its strand is dead, or its deadline plus the report wait has passed;
// the rules above decide again.
// The eligible-stop hold reads the ArbiterStop seam.
// It holds the notice of a blocked or awaiting child whose stop the arbiter may settle.
// The hold lasts until the arbiter has handed on or no arbiter will settle the stop, and it ends at the first of these:
// the applied mark of the stop's attempt;
// the moment the arbiter was seen off;
// a park marker written after the stop (unless the Go driver handed the run over at the stop);
// the attempt's hand-on;
// the backstop, the later of these:
// the stop plus one spawn's timeout plus the report wait;
// the attempt's deadline plus the report wait.
// A stop with the arbiter off or no spawns left and no attempt is never held.
// Error stops keep the marker hold alone.
// The report wait is then counted from the later of the stop and the hold's end.
// The start comes from the status file and every end from a record,
// so a batten restart neither resets the start nor loses the end;
// the first observation of the arbiter switched off rides in the memory record,
// so a later pass counts the report wait from it.
// When a resume moves the child back to running the episode ends and no notice goes;
// after a hand-on the marker is gone and the driver's stop report qualifies.
// A done child sends no notice: a landing that succeeds needs nothing from the orch.
// A child refuses a resume when its parked driver's pane holds a draft, an active operator or another busy reading;
// the spawn error wraps ErrChildDriverBusy.
// Such a refused resume is retried like a not-parked one.
// It is retried without a deadline until the resume lands, the child leaves awaiting, or batten is paused or stopped.
// Its wait reason names the draft and the remedy.
// One notice per stall tells the orch to clear the draft in the driver's pane.
// The notice quotes the box text the refusal found, as one line.
// A landed resume re-arms the notice.
// Delivery: the Notify seam reports whether the line was posted;
// with no orch strand recorded it is not called and one Warn is logged per episode;
// the strand is asked about again once per notice_probe_s;
// a Notify error or an unposted line is retried at most once per notice_probe_s and at most three times;
// it is then dropped with a Warn.
// A notice is informational: a delivery failure is only warned about.
// The outcomes, halts and waits are the same with or without it.
// Bound: the report wait delays a notice by at most three minutes.
// The relay wait delays it by at most ten minutes after the report.
// An arbiter hold delays it by at most the arbiter's deadline plus the report wait.
// No delay loses a notice, and there is one notice per episode.
// A driver can be alive but parked, for example a provider waiting on an interactive prompt its launcher never answered.
// A driver can also stop of its own accord with the run still non-terminal.
// Only the dobby's quiet notice, or its API-error notice when the stall is one, tells either from a working driver.
// An operator attaches to the child's session to tell the cases apart.
//
// A batten run is one pass after another:
// "lym batten run <slug>" runs one pass and exits.
// The run's parent worktree's dobby starts it, or an operator by hand in a terminal;
// a hand pass makes sure that dobby is live once it is over.
// The green end of the adopt check and `lym beam land` start a run's first pass detached,
// and a second batten for the slug is refused by the run's own driver lock inside the pass, in batten's own log.
// "lym batten run" steps the lifecycle through the Go driver,
// so an unexpected stop meets the driver's rule table and a cold tackler before the orch.
//
// A beam project's run starts at the Worktree-Create row too:
// the row claims the entry and skips a complete pair;
// otherwise it makes the pair from the beam above the project, unmarked and at the anchor the run's seed records;
// the pawl run's own Beam-Up marks it.
// A member's Worktree-Create reads its beam's hold again once its pair is made or found already complete:
// a beam whose run holds its entry removes the member's pair;
// the row refuses, naming the member's cancel and the board move off the beam;
// once the beam is back at Beam-Await it names its run instead.
// The refused member keeps its claim, so it stays an open member that stops the beam's landing.
//
// A resume, "lym batten run" over an existing status file, first runs the origin reconcile of the slug's run scope.
// An entry origin ended refuses, naming the landing or the removal and "lym cleanup --apply" from the prime;
// a raddle run standing at Raddle-File is the exception:
// origin shows its entry done, its own row ended it, and it resumes;
// a stopped entry refuses, naming the branch, its state and the way forward for it, then the re-run;
// an entry another machine owns refuses, naming the owner and "lym adopt <subject>" (the beam or `root` taking it over);
// an active entry resumes, and so does an unreachable origin, with a Warn.
// With origin answering, the resume first records this machine as the hub's prime owner when none is recorded.
// It also records this machine as the owner of a run-held entry that has none;
// a failed write refuses, naming the re-run once origin answers or the push's own way forward.
// With origin unreachable the resume reads the local board instead.
// It refuses a task the board shows owned elsewhere, and it writes no owner.
// A resume at Worktree-Teardown whose task worktree is gone runs no reconcile.
// It judges the owner on the local board alone, so the teardown retries what is left of the pair.
// A reconcile error refuses, with fixing its cause and then the re-run as the way forward.
// A fresh start runs no reconcile: creating the pair probes origin for it,
// and a parent behind origin costs only Parent-Align's merge.
// Instead it pulls the board, which needs origin.
// It refuses a task another machine owns before anything is seeded or claimed.
//
// Between Seed-Child and Run-Shed, the Command rows run one shell command each:
// Open-Window opens the operator's terminal window attached to the pair's reed session;
// Seed-IDE writes the pair's VS Code colour and .vscode/ config;
// Open-IDE opens VS Code on the pair.
// Each runs its command only when its entry gate's batten.yaml setting is on:
// open_window, seed_ide and open_ide in turn.
// Only open_ide is off by default.
// A row whose setting is off is skipped.
// A row is Done either way, a failed command only warning.
// The settings are read from the pair's own config stack when the row is entered.
// By then Seed-Child has written the task's run settings into the pair's local layer,
// so a task's run_settings sets them per task.
// The commands are the open_window_command, seed_ide_command and open_ide_command keys.
// They are read from the template and the board default layer only,
// so a task's run settings and a worktree's own batten.yaml cannot change them;
// `lym config batten --layer default --set <key>=<value>` from the prime changes one,
// and a different value in another layer is ignored with a warning.
// A command may hold the placeholders {lym}, {slug}, {task_dir} and {title}.
// {title} is the console title of the task's terminal window, `<shortname>: <P#>: <T#>: <slug>`.
// A part the hub or board does not give is left out.
// Open-Window's default command titles the window with {title}.
// A run resumed past Open-Window runs that command again when no client is attached to the pair's reed session.
// That needs batten.open_window on;
// so a window closed or left on another machine opens afresh, and a resume with a client attached opens none.
//
// The InnerRun row checks its child once per Call and never waits inside one:
// a Call is one pass, which the scheduling dobby runs again and again.
// A pass reads the child's status file, and decodes it again when its modification time moved since that read.
// It returns a budget-exempt Stuck only when something is worth a history entry:
// the child's state differs from the state the memory record holds as the child's current episode;
// the Stuck's Path and Reason name the change, even when the file's modification time did not move;
// batten's own status carries pause_requested;
// or the status file cannot be stat'ed.
// An arm's own event ends it too: an awaiting child's decision is acted on,
// and a done child's driver ending or its grace elapsing returns Done.
// A pass that ends nothing returns Yield with an empty pointer, which shed persists nothing for, and logs it at Debug;
// the run stays at the row, running.
// File stats and small file reads run on every pass;
// that includes the notice step's own reads (the decision record, the stop report);
// anything costing a process or a multiplexer round trip runs on a probe pass, at most once per notice_probe_s:
// the driver and review reads, a not-parked resume retry, the parent lock step and a notice's delivery.
// The memory record (innerrun_memory.go) is a tolerant JSON record under the row's scratch directory,
// and it is written through internal/state and carries what one pass knows for the next:
// the notice episode and each delivery's attempts;
// the owner trigger and the failure episodes;
// the binary re-step's interval and the parent lock's idle readings;
// the resume retry and the respawn count;
// the last probe and the wait reason with the keys it was logged under.
// A Call loads it first and saves it on every return;
// a missing or undecodable record reads as empty, the latter with one Warn,
// and a failed write warns and changes no verdict.
// RespawnCapReached reads the respawn count against the status file's modification time for a reader outside the row.
// That reader names the cap in the driver-dead notice of a running child.
// A status write since the last respawn reads as not capped.
// Bound: a running child is checked without a time limit.
// It is bounded by "lym batten pause" (honoured at the next pass), cancellation and the notices;
// a file that ends a wait between passes, such as a stop report or a strand's end, is seen at the next pass;
// that is up to one notice_probe_s later, and none is lost;
// a running child whose driver is dead and whose status file does not change yields on every pass;
// the dobby's driver-dead notice is the only signal.
//
// Seed-Child writes the task worktree's local config layer from the board task's run settings.
// It does so after reading the child driver and before the seed:
// it reads the task's profile and settings fresh on every Call;
// it lays the settings key by key over one block from the hub's `profiles.yaml` (see `internal/runprofile`);
// that block belongs to the recipe the row resolved.
// It comes from the task's run profile, or from the recipe's default profile when the task names none.
// It then writes the local file of each module they name.
// It removes the local file of every other registered module, with one Warn per removed file,
// so the layer is exactly the task's run settings over its profile's block.
// The profile is read only here, so a later change of the profile does not reach a run already seeded.
// An unknown profile, a profile holding no block for the recipe, or a key that fails validation stands the row Stuck.
// A key fails when a later binary retired it or closed it to the local layer,
// or it was written to `board.json` or `profiles.yaml` by hand.
// The Stuck names the profile or the module and the key, and the way forward:
// correct the task's `profile` or `run_settings` with `lym board upsert`, or the profile with `lym config profiles`;
// then run `lym batten run <slug>`.
// An unreadable board is Stuck as for the type, and any other prepare, write or remove failure is a hard error.
// A resume redoes the whole write from the current board entry, and no other row writes these files.
// Bound: a local file written while the row stood Stuck is discarded by the resume,
// so a hand edit to the pair's local layer before the resume is lost.
// Batten writes no seed field for run settings.
//
// Seed-Child and the InnerRun row also keep the batten-watched marker of the task worktree's run (MarkWatched).
// The marker holds this batten's pid while its notices have a destination that is also the child driver's parent.
// Seed-Child asks once the seed is committed, so the child's first driver launch already renders the watched rule;
// InnerRun asks at the start of every Call.
// A failure is a Warn, reads as not held, and never changes a verdict.
// Bound: only the driver's "run stopped" message is removed;
// it is removed only where a live batten with a notice destination replaces it at render time.
// A driver launched unwatched and adopted later keeps its own rule, a duplicate and never a loss.
// A driver launched watched keeps the silent rule whatever happens to batten afterwards,
// so the run's stops stay unannounced from a batten death or a vanished orch strand until a batten runs again;
// that batten then sends the notices not yet sent.
//
// A running child whose spawn this batten process has not confirmed is spawned again,
// so a restarted batten brings a driverless child back up:
// the confirmation marker holds the pid of the process that wrote it,
// and a marker from another pid or in the old layout reads as unconfirmed.
// A child is adopted instead if its reed state holds a live or retiring driver strand;
// a held run lock or a held Go driver lock adopts it too.
// The confirmation is recorded for this process, so a second driver is never stacked.
// The driver lock is a liveness signal of its own:
// a Go driver holds it for life while the run lock is free between steps,
// so a running go child whose driver lock is held has a driver at work and sends no driver-dead notice;
// a running child with neither lock, no loom bootstrap lock and no strand is noticed as driver-dead.
// That happens once it has been running for a minute of awake time from its running episode's since.
//
// A child that halts (blocked, paused or failed) is a budget-exempt wait, not a failure of the Run-Shed row:
// batten never spawns or resumes a halted child, bar the binary re-step below.
// It logs one Info line per halt episode.
// It keeps yielding.
// The reason names the child's state, error and current producer.
// It also gives the resume command ("lym loom resume" in the task worktree).
// A stuck verdict takes its log level from its cause:
// a failed producer step logs `producer stuck` at Warn, and an ordinary wait or move logs it at Info.
// A wait on the child logs at Info once per state of the child,
// so a reason that only counts up, such as the driver-exit grace, logs once;
// the reason file always holds the newest text.
// A halted or awaiting child whose reed state holds a dead driver strand has its pair's strands revived.
// The revive goes through reed resume.
// This happens once per halt episode, which the revived marker's presence records;
// the run stays halted and nothing is resumed,
// so the operator's "lym loom resume" wakes the revived driver.
// A retiring strand is not revived and a child with no driver strand has none to revive;
// a revive that fails, or a child with no driver strand, adds "lym loom start" in the task worktree to the reason.
// The wait has no time limit and spends no bounce budget;
// "lym batten pause" stops it at the next pass,
// and the row reads the child as running again once the operator resumes it.
//
// A run halted at Parent-Align or Finalize gets one state-changed notice like every other halt, and no goto:
// the recipe's own rows fix or hand back those failures.
//
// A halted go child is also re-stepped once per halt and per lym build (innerrun_binaryrestep.go),
// so a fix deployed after a Go driver handed back lands without a parent turn.
// The re-step goes through the Spawn seam, as every bootstrap does.
// It happens only when the child is blocked or failed and its reed state holds no driver strand;
// neither its run lock nor its driver lock may be held;
// the lym on disk must differ from the build the child's last step recorded,
// and the task pair must have no uncommitted or untracked path outside the run's records.
// A blocked halt that `InnerRunDeps.HandBack` marks as a landing halt is skipped and writes no marker;
// only the orch's `lym loom resume` lifts it;
// a tapestry, darn or sampler fixer's escalate halt, which the arbiter judges first, is marked and skipped too;
// every other halt keeps the re-step,
// so a fix deployed in a new build can still clear it.
// The `<row>-binary-restep` marker holds the halt's stop key and the build label,
// and it is written only after the spawn returns success,
// so the re-step happens once per halt and build and survives a batten restart.
// A spawn refused as not parked or as a busy pane, and a dirty tree, write no marker;
// they are tried again at the next due check;
// the spawns are capped at five per halt and build.
// Reading the build and the dirty paths costs a process,
// so the check runs once per interval, kept in the memory record:
// it starts at the notice probe and doubles up to thirty minutes while the halt and the build stay the same;
// it returns to the probe on a change.
// A pause covering the slug holds the re-step like every halted-arm action.
// A standalone go-driven run with no batten has no such watch.
// A nil build or dirty-path seam switches the re-step off.
//
// With deps.ParentLock wired, every probe pass looks after the parent lock of the child's run (innerrun_parentlock.go).
// A record is released once every agent of its owner has waited for the idle bound with none busy.
// The watch restarts whenever the owner changes.
// Its readings ride in the memory record on the Awake clock the caller keeps across passes;
// a failed agent read releases nothing.
// That release holds off while the child's run is at Finalize and running, or a verify of it is live;
// holding off keeps the record and restarts the watch;
// a failed read of either keeps the record and warns once per episode.
// Every release logs one Info, and batten releases only a record naming its own child's run.
// A polled seam that keeps failing, this lock step's included, warns at the first failure.
// It logs Debug while the failure repeats and one Info when the seam recovers.
// A retried spawn or goto logs Info on its first attempt and Debug on each retry.
//
// With deps.OwnerCheck wired, the check also re-checks the run's owner (innerrun_owner.go).
// The re-check is a judge-only reconcile of the run scope behind a seam.
// The check is due when the memory record holds it due.
// It is also due when a pause that covered the slug on the previous pass has lifted.
// It is due on a probe pass while an owner stands that no pause covers.
// A machine wake does not make it due here: it reaches the verdict through the full pass the scheduling dobby owes.
// It runs after the pass's status read and before any end that read gave;
// a due check and a state change seen together still run it;
// a failed stat or a vanished status file runs none and leaves it due for the next Call.
// An unreachable verdict and a check error keep it due and hold nothing.
// A verdict of another machine records that owner and releases the parent lock record naming the child's run.
// It pauses the slug under the owner caller through deps.PauseOwned.
// It sends one notice per episode naming the owner and "lym adopt <subject>".
// The subject is `OwnerVerdict.AdoptSubject`, or the slug when a spawn refusal brought none.
// A failed pause warns once per episode and is retried at the next probe while the owner stands.
// A Finalize already past its parent-side merge keeps its record and finishes its landing:
// a Finalize running, or failed on a transient cause with no re-step recorded for that stop;
// deps.ParentHoldsTaskWork must read true.
// A failed read of either seam keeps the record until the next check.
// Any other state at Finalize, blocked included, releases it.
// FinalizePastMerge is that decision over its seams as arguments,
// so a caller outside the wait reads the same carve-out;
// the owner pause's end spares such a Finalize and ends the processes of every other run.
// batten survives its own probe's end:
// the end spares the calling process and its ancestors;
// batten keeps its owner hold and probe, and the pair's removal ends it.
// A running child whose driver the end took has neither lock held and no strand.
// It is noticed as driver-dead once the owner pause lifts and an owned-here verdict follows.
// While the pause covers it, it is not noticed.
// A pause held for an owner releases the record the same way on a probe check, with no check of its own.
// Every spawn on an existing run reads a ChildOwnedElsewhereError as that verdict:
// the respawn of an unconfirmed running child returns a budget-exempt Stuck naming the owner;
// the awaiting resume and the binary re-step pause write no marker and keep waiting.
// While an owner stands every spawn waits,
// and the spawn hold re-runs the owner check once per probe until it reads the run as owned here,
// so a failed owner pause never makes the wait spawn and re-reconcile in a loop.
// Bound: the child can act until the full pass the dobby owes after a wake runs the check,
// and it can act for as long as origin stays unreachable;
// a loom child judges its owner again before each of its pushes,
// so it pushes nothing meanwhile once origin answers.
// A Finalize past its parent-side merge finishes its landing here.
// Once it is done its batten waits under the owner pause until "lym tender resume <slug>" lifts it;
// it then goes on to Land.
//
// A driver that finishes normally leaves its strand and its run directory behind, by design.
// Nothing here tears either down as part of a clean finish;
// only the whole-worktree teardown row cleans up, at the end, by removing the worktree they live in.
// The done arm waits for the driver strand, up to the driver-exit grace window,
// so the driver can finish its stop report (loom's post-run friction reflection included);
// a driver still alive past the window is ended by the teardown's session shutdown and recorded as an abandoned session.
//
// Worktree-Teardown has two halves, session shutdown and worktree removal.
// They call the pair-teardown composite (internal/pairteardown) as its EndSession and RemovePair phases.
// Session shutdown does not skip a gone task worktree, since the composite ends that session by name.
// A pair of which nothing remains is the done post-condition for both halves:
// session shutdown has nothing left to end, and the removal reports the pair not found.
//
// Every producer that holds the hub's prime lock waits for a contended lock instead of halting on the first try:
// they poll it every two seconds for up to ten minutes and stop at once when the context is cancelled;
// they return Stuck only when the bound is spent (primelockwait.go).
// Worktree-Teardown holds it for its whole body.
// Worktree-Create hands its create closure a HoldPrimeLock instead.
// The closure takes it only around the steps that read the prime's tree or write the hub's worktree registry.
// The origin probes start before the hold and overlap its wait, but Add waits for them under it,
// so the hold covers the probe time left after the pre-flight reads;
// the adopt fetch, the fast-forward fetch of a behind branch and the pair's archived-leftover replace also run under it.
// Runs creating different pairs therefore overlap on everything else:
// the board claim, the start of the probes and the new pair's push.
// The row releases any hold the closure left taken when it returns.
// Worktree-Create halts blocked at its own row when the create closure returns a PreconditionError.
// No bounce budget is spent, since no tackler can settle the precondition:
// the halt's reason starts with the precondition-unmet prefix and names what is unmet and its way forward,
// and the Go driver hands such a halt back.
// battenshed never imports fabric, so the closure alone builds the error.
//
// The Land row lands a child whose Run-Shed returned Done, in this order.
// Each step is idempotent, so a resume re-runs the row from the top:
// it takes the hub-level land lock for its whole body;
// a contended lock is polled from one second, doubling to thirty, with no time limit;
// it asks whether the child landed code, which only a child that finished a Finalize did,
// and a child that landed nothing, such as a motif run, only prunes the board and posts no notice;
// it checks the prime;
// a child whose parent branch is not the prime's branch skips the pull and deploy;
// a prime with uncommitted tracked paths is waited on for ten minutes;
// it pulls the prime under the prime lock and releases that lock at once, since batten never pushes the prime;
// outside the prime lock it runs the landing config's hub-wide `deploy` command in the prime;
// it runs only for a parent branch in `deploy_branches`;
// it has a thirty-minute timeout and its output goes to the row's deploy log;
// it prunes the board;
// only when the pull's records side stopped, it posts one notice naming the stop;
// the notice goes once per batten run, under its row-notice marker line.
// A clean landing posts nothing: a landing that succeeds needs nothing from the orch,
// and the beam's dobby reads which tasks are ready from the board.
// A records stop is the one notice of a successful landing.
// It leaves the prime's board behind and needs the orch's judgement.
// The two locks differ in what they hold: the land lock serialises whole landings, the prime lock only the pull.
// A failed step is Stuck naming it, and posts no notice of its own.
// A pause seen during the land-lock or dirty-prime wait halts the row blocked, with no notice.
// It does not halt paused, since the row has no on_stuck route to yield through;
// shed's blocked persist leaves the pause flag set,
// so the way back is `lym batten goto <slug> --to Land`, which clears it, then `lym batten run <slug>`.
//
// The failure notice of a row after Run-Shed waits for the driver's hand-back,
// so the orch is told only what the rule table, the tackler and the arbiter did not settle (rownotice.go):
// RowStopNotice sends one line for a Land or Worktree-Teardown stop the driver gave back.
// The line names the row, the reason cut to one line, the stop report and the way forward.
// The Land row also sends its records-stop line through its row notifier, on success.
// A hand-back notice goes once per stop, a stop being told apart by the length of batten's own status history;
// the row's `<row>-row-notice` marker keeps the length of the last hand-back noticed,
// so a restarted driver never repeats a posted notice.
// A notice is tried at most three times, thirty seconds apart, and dropped with a Warn after that.
// The failed row stays halted and visible in `lym batten status`.
// Every marker that must last one batten run is named by a suffix in runMarkerSuffixes.
// Batten clears them all through ClearRunMarkers where it seeds a fresh status.
//
// The package declares its own unexported entryErr and cancelErr helpers (ctx.go).
// It also declares its own reportStuck carrier (stuck.go).
// Each producer-owning package carries its own copy of them,
// so that no package depends on another only to share them.
package battenshed
