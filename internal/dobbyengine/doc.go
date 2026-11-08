// Package dobbyengine is the engine of dobby, the daemon that drives the batten runs of one beam worktree.
// It runs behind the hidden `lym beam dobby` verb, one process per beam worktree on its owning machine.
// Its caller tells it every path it touches.
//
// The prime is the root beam:
// its dobby has an empty slug in Geometry, and the dobby of any other beam carries that beam's slug.
//
// The package holds:
//
//   - Geometry is the absolute paths and names the hub wiring tells the engine.
//   - State is dobby's tolerant JSON record under the worktree's `.lym/beam/` directory.
//     ReadState reads it, and UpdateState rewrites it under its lock.
//     A record that does not decode reads as absent with one Warn, so a hand-damaged file never wedges the beam.
//   - The PassExit codes are what a `lym batten run` pass exits with for a refusal dobby reads by code.
//   - EnsureLive starts the dobby of each worktree it is named, the prime's name or beam slugs.
//     It starts each only when Live reads its lock free, through SpawnDetached.
//     It records this machine as the prime owner first when the prime is named and none is recorded.
//     It reports per worktree and is idempotent.
//     A named prime or beam that another machine owns is refused.
//     The refusal names its owner and `lym adopt root` or `lym adopt <slug>`.
//     A named beam whose landing hold stands is refused naming `lym batten status <slug>` and spawns nothing.
//     StartDeps.LandingHeld reads the hold, and a nil one reads as no hold standing.
//     OwnedWorktrees lists the prime and every local beam this machine may start.
//     A caller that narrows nothing names that set.
//     A worktree with an anchor change on its own records worktree or the prime's is refused and starts nothing.
//     The refusal carries the change's way forward.
//     StartDeps.MoveInProgress reads the change, and a nil one reads as no change standing.
//   - Config is beam.yaml, loaded strictly because dobby only ever runs inside a hub.
//     Its `pull_request` key is the receiving branch's pull-request mode, on, off, required or merge.
//     It is read with the receiving branch's worktree as the anchor.
//     Set at the default layer, it marks every branch of the hub.
//     Its `land_notice` key, all, per-task or off, decides which runs get a landing notice.
//     Its `adopt_fix_tries` key is the adopt check's try limit, which dobby itself never reads.
//     It caps the fix tasks a failed adopt check files for the worktree's subject between two hand adopts.
//   - Run is the daemon loop, with every seam it reads in Deps.
//   - `dobby.log` holds the dobby's own lines only.
//     AppendLog appends each line through its own open, which creates no directory.
//     A removed worktree therefore gets nothing back.
//     Only the holder of the file's dobby lock passes `rotate`:
//     a write that would take the file past 1 MiB first renames it to `dobby.log.1`, replacing an older one.
//     SpawnDetached points the child's raw standard output and error at `dobby.out`.
//     The holder of the lock rotates it the same way.
//     The one rotation a process without the lock makes is on Windows.
//     There RotatePastBound renames `dobby.out` before a spawn, only when Live reads the lock free.
//     Bound: `dobby.out` is rotated at the start of the lock holder,
//     so between starts it grows by what the dobby's standard error alone writes.
//   - BuildView is the View of one dobby a reader outside its process sees, built from what it is told:
//     its liveness, its state record, beam.yaml, its runs and the board.
//
// The loop:
//
//   - The lock at Geometry.LockPath is the one decision whether this process is the dobby.
//     ReadsAsReexec reads the state record first:
//     a record holding the process's own pid and its start ticks, or no start ticks, is a re-exec.
//     A re-exec waits Deps.ReexecLockWait for the lock, ten seconds when it is zero.
//     The process it replaced releases the lock as it execs.
//     Any other start waits lock.ProbeGrace.
//     A fresh dobby that reused the pid of a record without ticks, after a reboot, reads as a re-exec.
//   - A lock that stays held returns ErrDobbyRunning.
//     It logs `dobby: start refused: another dobby holds the lock` without rotating.
//     A refused re-exec whose record still holds the pid posts a notice naming `lym beam start`;
//     a refused fresh start posts none.
//   - Holding the lock, Run calls Deps.Locked, which the CLI fills with the run-process registration.
//     Run then records the start and logs `start fresh pid` or `start re-exec pid`.
//   - A record read error before the lock logs `dobby: stopped on error` with its way forward.
//     So does an error out of Run after the lock other than ErrDobbyRunning.
//     The log rotates only while the lock is held, and an error after a re-exec posts a notice.
//   - It reads its beam's owner at start and on each probe tick.
//     When another machine owns the beam, it exits with the logged reason `stop: another machine owns this beam`.
//     The reason names `lym adopt root` or `lym adopt <slug>`.
//     A cancelled context logs `stop: the context ended`.
//   - A record with the process's own pid and start ticks owes no new full pass;
//     a record with the pid and no ticks gains the ticks and owes one;
//     any other start rewrites the pid, the start time and the ticks and owes one.
//     A start owes every run a full pass until a tick succeeds.
//     So do a machine sleep the sleep watch sees and a tick that finds the record without a pid.
//   - It wakes on a watch event and on its timer, coalescing events to at most one tick a second.
//     The timer starts at the probe interval and doubles to a minute while no run is in flight.
//     It returns to the probe interval on an event or while a run is in flight.
//     Where the watch does not open, as on WSL or through a junction, the timer alone drives the loop.
//     It runs at the probe cadence, and no wake is shorter than a second.
//   - A beam dobby ends when its worktree is gone.
//     The test is the absence of the worktree's `.git` and never of its root.
//     Every state write recreates the record's directory, which is why the root cannot be the test.
//     The loop tests it at the top of every iteration and again after each tick, before the binary check;
//     the prime's dobby never takes the exit.
//     The exit appends `stop: the worktree of beam <slug> is gone` to Geometry.PrimeLogPath without rotating.
//     It also stops the tidy agent that the last tick with the worktree present recorded.
//     The stop goes through Deps.PrimeTidyShuttle.
//     The stop, or its failure with `lym reed remove <guid>`, is logged to the same file the same way.
//     It writes nothing at the removed worktree's path.
//     Bound: a tick in flight when the worktree is removed can leave an empty `.lym/beam` stub.
//     The stub holds the `state.json` and lock file its state access wrote.
//     The rest of that tick runs over the fresh empty State it read there.
//     A tidy that tick spawned is not stopped, and one it settled is stopped again.
//     The second stop removes a strand only while reed still tracks it.
//     It logs a failed stop when reed's status probe fails.
//   - Between ticks it calls the binary check, which re-execs onto a changed lym binary.
//     The check logs `re-exec onto the changed lym binary` right before the exec.
//     An exec that fails logs `re-exec failed` and posts a notice naming `lym beam stop` and `lym beam start`.
//     The notice goes once per binary identity, and the dobby keeps running.
//   - A tick's failure is logged once per episode and the loop goes on;
//     a cancelled context ends it after the tick in flight.
//   - Any existing path of Geometry.MoveRecords means an anchor change stands.
//     The change is on the dobby's own records worktree or the prime's.
//     While it stands, a tick spawns no pass, starts no task and runs no board-tidy step.
//     The tidy agent is a strand whose state sits at the anchor.
//     The state record's upkeep and the landing count go on.
//     A full pass the dobby owes stays owed, and the first tick of the episode logs the change once.
//     An empty list, as a hand-built Geometry leaves it, gates nothing.
//   - The log records changes only, each one line through the Log seam stamped with its time, and a heartbeat.
//     A task start logs `start <slug>`; a pass spawn logs nothing.
//     A run's batten state or producer changing logs `run <slug>: <old> -> <new>`.
//     The line carries the state alone for running and done;
//     for any other state it adds ` at <producer>`.
//     A run first seen logs its state once.
//     The memory is in process only, so a re-exec logs no change it did not see.
//     The first failing pass of an episode logs `pass <slug> failing: <error>`.
//     The exit 0 that ends the episode logs `pass <slug> succeeds again`.
//     A failing tick and an unreadable beam owner each log one line when the episode starts and one when it ends.
//     A board-tidy agent starting, posting its report and timing out each log one line.
//   - A heartbeat is `heartbeat: <slug> <state> <producer>; …` per run still driven, or `heartbeat: no runs`.
//     It is written right after the start line, which also records the logged states.
//     The first tick at least ten minutes after the last heartbeat writes another, whatever that tick did.
//     A run listing that fails writes `heartbeat: runs unreadable: <error>` in its place.
//     It rides the loop's ticks with no wait or wake of its own,
//     so a dead dobby shows in its log within ten minutes plus the longest gap between ticks.
//
// The passes:
//
//   - Each tick lists the runs whose parent worktree is this dobby's, from the prime's local batten status files.
//     It never reads the board for this, so a run whose task Land pruned is still driven through its teardown.
//   - A run gets a pass only while batten's state is running, or paused with its work on its parent,
//     and its cancel marker is absent.
//     A run never gets a pass while one of its passes is in flight.
//     A held driver lock reads as in flight, so a pass left by a dead dobby is waited for and never doubled.
//     A halted batten run gets no pass, nor does a paused one whose work has not reached its parent.
//     The orch or the operator resumes it with a hand pass.
//     A paused run whose work reached its parent, RunView.ReachedParent, has nothing left to decide:
//     while no pause entry covers it, each trigger gives it a full pass.
//     That pass resumes it through its Land and Worktree-Teardown rows, and a covering pause holds it until the lift.
//   - A done batten run is driven no further:
//     once no pass of it is in flight, it takes no slot under `max_runs` and gets no pass.
//     Its entry leaves the state record.
//     The board tidy still reads its history.
//   - A pass is triggered by a write of the child's status file;
//     by a write of batten's own status file later than the run's last pass exit;
//     by a probe tick once per probe interval;
//     by the dobby's start; and by the lift of a pause that covered the run.
//     A lift while a pass is in flight gives the run its pass once that one exits.
//   - A continue pass needs the run at Run-Shed with no full pass owed and no lift to answer.
//     Every other pass is full, and it runs with `--no-reopen` while the run's origin-owed marker stands.
//     A start and each recorded sleep owe every run a full pass.
//     The state record keeps it until a full pass exits 0 with no origin-owed marker standing.
//   - While a pause covers the run, only continue passes run, and the full pass at the lift.
//     A run a hold-caller entry covers gets no pass of any kind and no owner-pause recheck.
//     The lift of the hold gives it the full pass.
//   - A refused continue pass owes a full pass, and a covered pass counts as no failure.
//     Any other non-zero exit counts a failure by its error text and is retried on the next probe tick.
//     The third failure in a row with one text posts one notice to the nearest live orch.
//     An exit 0 clears the count and the noticed texts.
//   - A pass that exits as owned by another machine keeps the run's full pass owed.
//     The state record marks the run as owned elsewhere.
//     Dobby pauses the run under the owner caller and releases the parent lock record naming its child.
//     It posts one notice per owner episode naming `lym adopt <subject>`, and no failing-pass notice.
//     Deps.AdoptSubject reads the subject; the slug stands in when that seam is nil or fails.
//     A failed pause warns once per episode and is recorded.
//     It is never retried blind:
//     while the mark stands and no pause covers the run, each probe tick gives it a full pass.
//     That pass is refused again, or it exits 0 and clears the mark.
//     While the mark stands and a pause covers the run, it gets no pass, a start's and a wake's included.
//     The covering pause's lift still gives it a full pass.
//     Once the owner pause stands, each probe tick reads the run's effective owner from the local Bolt with no pull.
//     When the owner no longer reads as another machine, dobby lifts its own owner pause and runs a full pass.
//     An owner that a continue pass's own check finds sets no mark:
//     that check pauses the run itself, and dobby goes on passing it.
//     A marked run is in flight for the backoff only while no pause covers it.
//   - A task waits on the board as `draft` until released, and dobby starts it from an empty status.
//     A task starts when all of these hold:
//     the entry is a task whose dependencies are all done;
//     its parent is this dobby's worktree;
//     no other machine owns it;
//     no refusal awaits every start of its slug;
//     and no pause entry covers the hub or the slug.
//     Each tick reads the local board, which a write of `board.json` and the probe tick both cause.
//     It starts each ready task while fewer than `max_runs` runs it drives are in flight:
//     halted runs count; done runs, runs recorded owned elsewhere and pawl runs with no landing hold do not.
//     `max_runs: 0` starts nothing.
//   - Right before each start, dobby reads the cgroup task limit it sits under through Deps.TaskLimit.
//     The limit is that of the level of its own cgroup ancestry with the fewest free tasks, such as the user slice.
//     While fewer than beam.yaml's `task_headroom` tasks are free, the tick starts no task.
//     A held task gets no refused-start record, notice or backoff and is tried on the next tick.
//     The first held start of an episode logs this line:
//     `hold starts: <current> of <max> tasks in <cgroup>, below task_headroom <n>`.
//     The first start let through after it logs `starts resume: …`.
//     The episode is kept in process only.
//     `task_headroom: 0`, a nil seam and a limit that does not read hold nothing.
//     A limit does not read on Windows or a cgroup v1 host.
//     Bound: the hold only delays starts, never a pass of a run already started.
//     A start goes within the idle timer's minute of the headroom returning.
//     First passes fork little at their start.
//     Starts in one tick or by several dobbies can therefore each pass the check before the earlier ones grow.
//   - Starting is the run's first pass, a full pass, which claims the task as batten does;
//     a slug with a pass in flight is never started twice.
//     A first pass refused for a covering pause is no refused start:
//     it gets no notice, no marker and no backoff, and the start waits for the lift.
//     Any other first pass that exits non-zero and leaves no status file is a refused start:
//     it gets one notice per refusal text, an owned-elsewhere refusal included.
//     The retry comes after one probe interval, doubling up to an hour, or at once when the entry changes.
//     The state record keeps the backoff, the last refusal text and the noticed texts.
//     It clears them when a pass claims the task or its board entry changes.
//   - A front task is a task entry whose `front` is true, filed by lym ahead of the worktree's other starts.
//     It is open while its status is empty or a run status.
//     It belongs to this dobby when its `project` is the dobby's beam slug, empty for the prime's.
//     While one is open, the step starts no other task of the worktree.
//     The view shows each released task behind it as `front_first`, not ready.
//     It starts the front task itself once it is released and ready by the start condition.
//     It does so even when the runs in flight, halted and held ones included, fill `max_runs`.
//     Once started, the front task takes its slot like any run.
//     `max_runs: 0` still starts nothing.
//     The task-limit hold, a covering pause and the start backoff hold the front task as they hold any task.
//     A draft front task is closed and lifts the hold.
//     Bound: the hold reaches only new starts in this worktree, never a nested beam's.
//   - After the passes and the notices and before the start step, each tick calls `Deps.SettleFronts`.
//     It settles the worktree's front tasks:
//     it records an open front entry that has no record and runs the land action of one that landed.
//     An error stops nothing.
//     The first error of an episode is logged and posted once to the nearest live orch.
//     The notice reads `[dobby notice] front task: <error>` and names the slug, the action and its hand step.
//     A tick that settles cleanly ends the episode.
//   - A released task whose slug has a finished run record carries the refusal that every start of that slug gets.
//     TaskView.StartRefusal holds the refusal, with its way forward.
//     The exception is a done motif run of a task whose recipe the board resolves to raddle:
//     that start clears the record, so the task carries no refusal.
//     Dobby spawns no pass for it.
//     A task owned elsewhere, with open dependencies or covered by a pause gets no record.
//     Any other such task has the refusal recorded as the start record's refusal, with no backoff.
//     The same one notice per refusal text goes out, again when the entry changes.
//     Bound: the record is read every tick, so the task starts on the first tick after the record is gone.
//   - A task is started only from an empty status, by its parent's dobby or a hand-run `lym batten run`.
//     It is never started twice while a pass of it is in flight.
//     Across machines only the owner machine's dobby of the parent runs.
//     A release written on another machine reaches this dobby when this machine's board is next pulled.
//   - Bound: per run at most one pass per probe interval plus one per write of the child's status file,
//     and at most one in flight.
//     The loop's backoff counts a run as in flight while the planner could give it a pass on the next probe tick.
//
// The slot rule:
//
//   - A run whose RunView carries NoSlot takes no slot under `max_runs`.
//     NoSlot marks a pawl run whose landing hold does not stand.
//     Such a run only waits on its members at Worktree-Create, Preflight, Beam-Up and Beam-Await.
//     It takes a slot from Finalize on.
//     The parent's dobby still schedules the run's passes, owed full passes and probes as for every run it drives.
//     The view's `max_runs` reasons follow, since BuildView counts through the same rule.
//
// The landing step:
//
//   - After the scheduling steps, each tick of a beam's dobby reads Deps.CanLand.
//     Its LandReading says whether the beam is flagged ready with nothing working under it.
//     It also says which other machine, if any, owns the beam's parent.
//     Without the flag nothing working under the beam is no cause to act, however long the beam has been idle.
//     The prime's dobby and a nil CanLand take no step.
//     The tick runs on every board change the dobby watches and on every pass exit.
//     A member's teardown, a cancel with `--remove`, and a member moved off the project each trigger it;
//     so does a member marked done by hand.
//   - A passing reading with no closed episode recorded in State.Land asks for the landing through Deps.Land.
//     Deps.Land writes the land request the beam's pawl run consumes, and the step records the episode as closed.
//     It posts one notice to the beam's own orch only when it cannot ask for the landing.
//     When another machine owns the parent, the notice names that machine and `lym adopt <subject>`.
//     That episode closes once the notice goes out.
//     When Land refuses, the notice carries the refusal.
//     A refusal leaves the episode open:
//     Land is asked again each tick, as a refusal may only wait on the run reaching Beam-Await.
//     The refusal's notice is not posted twice.
//     A reading that does not pass clears the episode, so a member opened again re-arms the step.
//     A restarted dobby reads the record and takes no second step.
//   - Bound: the step lands nothing by itself:
//     Beam-Await re-checks the beam before anything lands.
//     A land request Beam-Await refuses is not asked for again in the same episode.
//   - A failed read or post is logged once per episode and changes nothing:
//     a failed post leaves the episode open for the next tick.
//   - While Deps.BeamReady reads true, the beam's ready record stands.
//     Its start step then starts no task of the worktree, so no member begins once the beam is flagged ready.
//     Each open member of the beam gets one notice.
//     An open member is a task not done with no run listed, drafts and tasks held by a dependency included.
//     The notice names the member, says that it blocks the landing, and names `lym batten cancel <member> --remove`.
//     State.ReadyHold lists the members noticed, each recorded only once its post went out.
//     A reading that is false clears State.ReadyHold, so a later episode notices again.
//     A read that fails starts nothing, is logged once per episode and changes no record.
//     A nil BeamReady holds nothing.
//     The view shows every waiting task as held by the flag.
//
// The member landing announcement:
//
//   - After the landing step, each tick of a beam's dobby reads Deps.LandingPullRequest.
//     With no pull request recorded in the beam's landing record, it does nothing.
//     Otherwise it reads the last announced head from Geometry.AnnouncePath, a never-tracked file.
//     When the file is missing, it reads the record's announce head instead,
//     so a fresh machine repeats at most one notice.
//     It then reads the landings past that head through Deps.BeamLandingsSince.
//   - With landings it posts one line through Deps.Notice.
//     The line names the landings and `lym beam describe <slug>`,
//     which records the revised description and updates the pull request,
//     so the orch keeps both listing every member.
//     Only after the post succeeds does it write the head the read returned.
//     A head that moved past only commits that record lym's own state is written with no line,
//     so the notice cannot feed itself.
//   - Every landing counts, a member filed after the pull request opened included.
//     No entry's `notify_on_land` is read; a member that carries one also gets its own landing notice.
//   - A failed read, post or record is logged once per episode and retried at the next tick;
//     nothing else depends on it.
//     The prime's dobby and a nil LandingPullRequest or BeamLandingsSince announce nothing.
//
// The run notices:
//
//   - On each probe tick, after the scheduling steps, dobby reads children through Deps.RunNotices.
//     It reads the child of every listed run whose batten state is running with no cancel marker.
//     It sends the notice the child's condition calls for, and a nil RunNotices.ReadChild sends none.
//   - A child that is not running, or that a pause covers, is in no condition.
//     A dead driver is no live driver strand and no held driver, run or loom bootstrap lock.
//     It is checked first, once the child has been running for a minute of awake time,
//     so a driver still starting is no dead one.
//     Once batten respawned the child the cap of times with no progress, the notice changes:
//     it names the respawn cap and `lym loom start`.
//     With a live driver, the conditions are checked in this order:
//     an agent run on the provider's usage limit and inactive for two minutes gives the usage-limit notice;
//     an agent run stalled on an API error and inactive for two minutes gives the api-error notice;
//     every agent idle for the quiet window with no live wait gives the quiet notice.
//     With no agent run read, quiet counts from the latest of these:
//     the child's newest history entry;
//     the end of its newest ended agent run, which RunNotices.NewestAgentEnd reads;
//     and its state's since.
//     A session ending before its row writes its history entry is therefore no idle row.
//     The idle clocks leave out the machine sleeps in the state record and the paused intervals,
//     so a sleep or a pause can delay a notice and never raise one earlier.
//   - With RunNotices.ReviveAPIError set, the first sight of an api-error episode revives the stalled sessions.
//     That sight notices nothing.
//     The notice goes when the episode still stands at a later probe tick;
//     it also goes at the third revived episode in a row with no write of the child's status file between.
//   - A notice reads `[dobby notice] <slug>: <state> at <row>; <condition>`.
//     The child's reason follows as `waiting:`, `since` and `history`.
//     Paths under the task pair are rendered relative to it.
//     The notice goes through Deps.Notice;
//     one not posted is tried on each probe tick, at most three times, then dropped with a Warn.
//   - A notice goes once per episode:
//     its key is the condition, the child's state and a stamp.
//     The marker RunNotices.MarkerPath names keeps the keys, one per line.
//     The hub wiring places the marker in the run's records directory, so batten's status commit carries it to git.
//     A driver-dead key ends when the child's state or its since changes.
//     A quiet or api-error key ends when the newest agent activity moves.
//     A usage-limit key ends once no run reads usage-limited.
//     The child's state, its since and the revive streak ride in the run's entry of the state record,
//     so a re-exec sends no notice twice.
//   - A failed read is logged once per episode and sends nothing.
//   - Bound: while an anchor change stands no notice goes.
//     A machine that takes a run over dates the child's since from its own first sight,
//     so a driver-dead notice the other machine sent can go once more.
//     It goes after the start grace counted from that first sight.
//
// The wait reminders:
//
//   - Every dobby, the prime's included, checks at most once a minute, riding its ticks with no wait of its own.
//     It checks the child of each listed run whose batten state is running with no cancel marker.
//     It does so through Deps.RunNotices' child and pause reads.
//     A nil WaitReminders.Post or RunNotices.ReadChild checks nothing.
//   - A child running at a row of WaitReminders.Rows with no pause covering it is in a wait, whatever the presence.
//     RunState.OperatorWait keeps the wait: its row, and as its start the first check that read the child there.
//     The hub wiring names PR-Review and Sampler-Session;
//     Beam-Await is not among them, since it waits on the beam's members as well as the operator.
//     WaitReminders.LatestResume moves the start to a later resume of a pause covering the child,
//     so a pause and resume the dobby did not read, between two checks or while it was down, restarts the wait too.
//     The wait is cleared when the child leaves the row, stops running or a pause covers it.
//     It is also cleared when its run's batten state stops being running.
//   - While WaitReminders.Away reads true, a wait as old as WaitReminders.Interval gets one line:
//     `<slug>: waiting at <row> for <n> minutes`.
//     WaitReminders.Post carries it, and each further interval brings another.
//     A post that fails is logged at Warn and tried at the next check.
//   - The last reminder's time is in memory only.
//     It is forgotten when the wait ends and while the operator is not away,
//     so a return to away reminds at once of a wait past the interval.
//   - A read that fails is logged once per episode:
//     a child's leaves its wait as recorded and gets no reminder, and the presence's or the interval's sends none.
//   - Bound: while an anchor change stands no wait is checked.
//     A restarted dobby keeps the start but may remind once early.
//     A child that leaves the row and returns between two checks with no pause keeps its wait.
//
// The landing notice:
//
//   - Each tick with no anchor change standing runs this step after the run notices.
//     Dobby remembers in the state record every listed run ahead of its Land row.
//     The memory holds whether the board entry carries `notify_on_land`, refreshed while the entry is on the board.
//     A RunView tells a run past its Land row, and whether it landed, from `battenrecipe.LandOutcomeOf`.
//     It carries the landing commit Finalize recorded.
//   - A remembered run seen past its Land row owes one line through Deps.Notice to the nearest live orch.
//     It owes it when beam.yaml's `land_notice` is all.
//     It also owes it when `land_notice` is per-task and the entry's request, remembered or read that tick, asks for it.
//     Under off it owes none.
//     The line is `[dobby notice] <slug>: landed as <commit>`.
//     With no commit recorded it reads `landed, commit unknown`.
//     For a run that did not land it reads `ended without landing` with `lym batten status <slug>`.
//     A beam's batten run is driven by its parent's dobby, so a beam's landing gets its line the same way.
//   - A done board entry owes the landed line as well when all of these hold:
//     its parent is the dobby's worktree;
//     this machine owns it;
//     it carries `notify_on_land`;
//     and it has no memory of its own.
//     The line names the commit when its run is listed past Land, and none goes under off.
//     A done entry is read as landed whatever set its status,
//     so an entry marked done by hand gets a landed line for a landing it never made.
//     A request set after its run passed Land is noticed at the first tick that reads the entry.
//     So is a landing the dobby never saw ahead of Land, one made while it was down included.
//     Board prune keeps the entry until its request is cleared.
//   - Deps.ClearNotifyOnLand clears the entry's field, so prune can drop the landed entry.
//     It clears once a requested run's or entry's line went out, or at once when it owes none.
//     A failed clear is tried again at the next tick.
//   - An owed line is kept in the state record and posted at every tick until a post succeeds, with no cap.
//     It survives the end of the run's listing, a restart and a re-exec.
//     Each failed post is logged once per episode.
//     Creel keeps mail to an orch whose strand is not listed until a strand of that name is,
//     so a post fails only while creel or the orch record cannot be reached.
//   - Bound: a run first seen past its Land row whose entry asks nothing gets no line,
//     so `land_notice: all` notices no landing the dobby did not see ahead of Land.
//
// The board tidy:
//
//   - Each tick reads the batten history of every run whose parent worktree is this dobby's, whatever its state.
//     It records each Land Done that names this machine as counted in the state record.
//     So a landing that a hand pass, or a pass a dead dobby left, made is counted, and a restart counts none twice.
//     A record read for the first time counts every landing already there as seen.
//     So does a record rewritten fresh over one that did not decode.
//     Run records left from before the dobby therefore trigger no tidy.
//     While a tidy agent is in flight nothing is counted,
//     so the record's slugs since the last tidy are exactly the agent's.
//   - At every `board_tidy_every` landings, 0 meaning never, dobby spawns the board-tidy agent unless one is in flight.
//     The agent is a fresh shuttle run in a window of the worktree's reed session, with role BoardTidyRole.
//     Its prompt is TidyPrompt over the landed slugs and a parent directive naming the nearest live orch.
//     Its one output is the report `.lym/beam/tidy/<UTC timestamp>.md`.
//     The agent in flight is kept in the state record, so a restart or a re-exec spawns no second.
//   - Once the report exists, dobby posts its path to the nearest live orch.
//     It ends the agent through shuttle's stop verb.
//     Past `board_tidy_timeout_min` dobby ends the agent the same way and posts a no-report line naming the slugs.
//     It keeps the slugs in the state record for the next tidy.
//   - Bound: a fresh record loses a tidy in flight and the slugs landed since the last tidy,
//     and the next tidy may spawn while the old agent runs until its timeout.
//     A landing whose run records `lym batten cleanup --apply` removed before dobby read them is not counted.
//
// The view:
//
//   - It carries beam.yaml's `max_runs` and `land_notice`.
//     It marks each run and waiting task whose board entry asks for a landing notice.
//     A run is also marked when the state record remembers the request.
//   - The runs are the listed ones that are not done.
//     Each carries batten's state and producer, and a pass in flight while its driver lock is held.
//     It also carries its last pass exit, owed full pass, owner mark and failing count from the state record.
//   - A task waits while it is released to the worktree and not started.
//     Released means an empty status and the worktree as parent.
//     Its reasons are the start step's own conditions:
//     the dependencies not done; another machine's ownership; a covering pause;
//     a refused start whose backoff has not run out for the entry as it reads now;
//     the beam's ready hold; and `max_runs`.
//     The `max_runs` reason falls on the tasks nothing else holds, in board order as the start step walks.
//     It applies once the runs that take a slot fill it.
//     A task with no reason starts on the dobby's next tick unless the task-limit hold stands.
//     The view does not read the task-limit hold, and the dobby log records it.
//   - Bound: a first pass in flight that has left no status file is in no file,
//     so its task reads as ready and its slot as free until the status appears.
//
// Told-geometry tier: the package takes every absolute path from its caller and imports neither internal/lymcwd nor any CLI package, per PATTERN-cwd-resolution.
// Its seam_enforcement_test.go enforces that membership mechanically.
package dobbyengine
