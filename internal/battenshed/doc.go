// Package battenshed owns the task-worktree batten producers: creating the task worktree,
// seeding its own inner run, running that run inside it to a terminal state, landing it into the prime, and tearing it down.
// Any producer list may name them by reference, the same way internal/landingshed frames its own
// two producers.
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
// A marker under the row's scratch directory records the notices sent, and only those queued, so a batten restart sends the notices not yet sent and never one already sent.
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
// When a resume moves the child back to running the episode ends and no notice goes; after a hand-on the marker is gone and the driver's stop report qualifies.
// A done child sends no notice: a landing that succeeds needs nothing from the orch.
// A resume the child refuses because its parked driver's pane holds a draft, an active operator or another busy reading (a spawn error wrapping ErrChildDriverBusy) is retried like a not-parked one, without a deadline, until the resume lands, the child leaves awaiting or batten is paused or stopped.
// Its wait reason names the draft and the remedy, and one notice per stall tells the orch to clear the draft in the driver's pane and quotes the box text the refusal found, as one line; a landed resume re-arms it.
// Delivery: the Notify seam reports whether the line was queued; with no orch strand recorded it is not called, one Warn is logged per episode and the strand is asked about again once per notice_probe_s;
// a Notify error or an unqueued line is retried at most once per notice_probe_s and at most three times, then dropped with a Warn.
// Notice log: every notice a batten process composes, the InnerRun row's and the later rows' alike, is appended once per composition, at the first delivery attempt in that process, to the hub's notice log through the NoticeLog seam (rownotice.go).
// A line holds the UTC time, the notice text and the run's attach command, tab-separated, and is written in one write so concurrent batten processes never interleave inside a line.
// A held-back or dropped notice is logged all the same, a restarted batten composing an episode's notice again logs it again, and a failed append is only a Warn.
// Control flow is unchanged: a notice is informational, a delivery failure is only warned about, and the outcomes, halts and waits are the same with or without it.
// Bound: the report wait delays a notice by at most three minutes, the relay wait by at most ten minutes after the report, and an arbiter hold by at most the arbiter's deadline plus the report wait; none loses it, and there is one notice per episode.
// A driver that is alive but parked -- a provider waiting on an interactive prompt its launcher never answered -- or that stops of its own accord with the run still non-terminal is told apart from a working one only by the quiet window, or by an API-error notice when the stall is one; an operator attaches to the child's session to tell the cases apart.
//
// A batten run starts with "lyx batten run <slug>" in a terminal, or with "--window" in its own tmux window of the orch's reed session, which returns at once.
// The tmux window lives as long as the reed session, and a second batten for the slug is refused by the run's own lock inside the window, in batten's own log.
// "--window" also asks the run to open the operator's terminal window: the InnerRun row opens it once per run through its OpenTerminal seam, after the child's first successful spawn, gated by a once-marker in its scratch directory.
// Batten resets the marker when it seeds a fresh batten status, so a resume keeps it and a re-spawned child opens no second window.
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
// it reads the settings fresh on every Call, writes the local file of each module they name, then removes the local file of every other registered module with one Warn per removed file, so the layer is exactly the task's run settings.
// A key that no longer validates (a key a later binary retired or closed to the local layer, or one written to `board.json` by hand) stands the row Stuck, naming the module, the key and the way forward, correcting the task's `run_settings` with `lyx board upsert` and then `lyx batten run <slug>`; an unreadable board is Stuck as for the type, and any other prepare, write or remove failure is a hard error.
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
// batten never spawns or resumes a halted child, bar one exception below, logs one Warn per halt episode, and keeps waiting with the child's state, error, current producer and the resume command ("lyx loom resume" in the task worktree) as its reason.
// A halted or awaiting child whose reed state holds a dead driver strand has its pair's strands revived through reed resume, once per halt episode per batten process;
// the run stays halted and nothing is resumed,
// so the operator's "lyx loom resume" wakes the revived driver.
// A retiring strand is not revived and a child with no driver strand has none to revive;
// a revive that fails, and a child with no driver strand, make the reason also name "lyx loom start" in the task worktree.
// The wait has no time limit and spends no bounce budget; "lyx batten pause" stops it within one check, and the row reads the child as running again once the operator resumes it.
//
// The one exception is the first verify failure of a batten run, at Merge-In or Publish (publishretry.go).
// A child blocked at one of those rows whose failure record names that row and a target, carries the child's HEAD and was written during the halting attempt is sent to that target through the child's goto verb and then resumed through its bootstrap, with no notice for the halt or the paused state in between.
// The `<row>-publish-retry` marker holds the retry's state and start time: `started` before the goto, `goto-done` after it, `resumed` after a resume that landed and `capped` when the retry gave up.
// `resumed` and `capped` each spend the run's one retry, and the marker is cleared only where batten seeds a fresh status, so a restart neither repeats a finished retry nor resets the cap.
// A goto refused as busy and a resume refused as not parked or as a busy pane are retried once per notice probe until the cap of thirty minutes after the start; any other goto refusal gives up at once.
// A retry that gave up, or a second Publish halt after the retry ran, sends the usual blocked notice with words saying so, which tells the orch the failure needs judgement.
// Any record that does not match, and any read error on the way to the decision, leaves the halt to the handling above.
// The retry is split between two records: a Go driver's verify retry is counted in the child's settle records, and batten's in its marker.
// With no marker, a blocked child at a verify row whose settle records show a Go verify retry counts as spent, so a run is never retried by both.
//
// With deps.ParentLock wired, the wait also looks after the parent lock of the child's run on every probe check (innerrun_parentlock.go).
// A lock record handed to this batten at PR-Gate is kept while the child awaits there, until that awaiting has lasted the idle bound with no decision to resume on, and while the child runs after a resume, so the resumed loop takes it back; in any other state the record is released.
// A record owned by another process is released once every agent of that owner has waited for the idle bound with none busy, a watch that restarts whenever the owner changes; a failed agent read releases nothing.
// Every release logs one Warn, and batten releases only a record naming its own child's run.
// A child awaiting at PR-Gate with no decision pending whose parent branch moved upstream is resumed through the same spawn as a decided one, so the gate's next call reroutes it to Merge-In under the lock, and its awaiting notice is suppressed.
// The moved-parent read may fetch, so it runs only for a child awaiting at the gate row, at most once per probe.
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
//
// The Land row lands a child whose Run-Shed returned Done, in this order, each step idempotent so a resume re-runs the row from the top:
// it takes the hub-level land lock for its whole body, polling a contended one from one second doubling to thirty with no time limit;
// it checks the prime, where a child whose parent branch is not the prime's branch skips the pull and deploy and a prime with uncommitted tracked paths is waited on for ten minutes;
// it pulls the prime under the prime lock and releases that lock at once, since batten never pushes the prime;
// outside the prime lock it runs the landing config's hub-wide `deploy` command in the prime, for a parent branch in `require_pr_to_base` only, with a thirty-minute timeout and its output in the row's deploy log;
// it writes the ready snapshot before pruning the board, so the tasks the prune made ready are named exactly once across a restart; and it queues one notice naming them.
// The two locks differ in what they hold: the land lock serialises whole landings, the prime lock only the pull.
// A failed step is Stuck naming it, with one failure notice per episode.
// A pause seen during the land-lock or dirty-prime wait halts the row blocked with no notice, rather than paused, since the row has no on_stuck route to yield through;
// shed's blocked persist leaves the pause flag set, so the way back is `lyx batten goto <slug> --to Land`, which clears it, then `lyx batten run <slug>`.
//
// The rows after Run-Shed send the orch a failure notice through a row notifier (rownotice.go): Worktree-Teardown on every failure it returns, and the Land row on the same terms.
// A notice goes once per failure episode, an episode being one entry of the row from its predecessor's Done or from a halt at that row, told apart by the length of batten's own status history;
// the row's `<row>-row-notice` marker keeps the length of the last noticed failure, so a re-entry that appended nothing sends nothing, and a batten that died inside the row never repeats a queued notice.
// A notice is tried at most three times, thirty seconds apart, and dropped with a Warn after that, leaving the failed row halted and visible in `lyx batten status`.
// Every marker that must last one batten run is named by a suffix in runMarkerSuffixes, and batten clears them all through ClearRunMarkers where it seeds a fresh status.
//
// It declares its own unexported entryErr/cancelErr helpers (ctx.go) and its own reportStuck
// carrier (stuck.go) for the same deliberate-duplication reason internal/preflightshed/doc.go and
// internal/landingshed/stuck.go already record: each producer-owning package carries its own copy
// rather than sharing one across packages it otherwise has no reason to depend on.
package battenshed
