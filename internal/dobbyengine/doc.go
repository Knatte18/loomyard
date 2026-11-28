// Package dobbyengine is the engine of dobby, the daemon that drives the batten runs of one beam worktree.
// It runs behind the hidden `lym beam dobby` verb, one process per beam worktree on its owning machine.
// Its caller tells it every path it touches.
//
// The prime is the root beam:
// its dobby has an empty slug in Geometry, and the dobby of any other beam carries that beam's slug.
//
// "The nearest live orch" names where the hub wiring sends the dobby's lines.
// That is the orch of the dobby's own worktree while its strand stands, even when it reads as down.
// Otherwise it is the nearest live orch up the beam chain, else the prime's.
//
// The package holds:
//
//   - Geometry is the absolute paths and names the hub wiring tells the engine.
//   - State is dobby's tolerant JSON record under the worktree's `.lym/beam/` directory.
//     A record that does not decode reads as absent with one Warn, so a hand-damaged file never wedges the beam.
//   - The PassExit codes are what a `lym batten run` pass exits with for a refusal dobby reads by code.
//   - EnsureLive starts the dobby of each worktree it is named, only when Live reads its lock free.
//     It reports per worktree and is idempotent.
//     It refuses a worktree whose owner reads as another machine, naming `lym adopt root` or `lym adopt <slug>`.
//     The engine compares no machine name: the hub wiring judges each owner and hands over the answer as a flag.
//     It refuses a worktree with an anchor change standing, and a beam whose landing hold stands.
//   - Config is beam.yaml, loaded strictly because dobby only ever runs inside a hub.
//     Its `beam.adopt_fix_tries` key is never read by dobby itself.
//   - Run is the daemon loop, with every seam it reads in Deps.
//   - `dobby.log` holds the dobby's own lines only, and only the holder of the dobby lock rotates it.
//   - BuildView is the View of one dobby a reader outside its process sees, built from what it is told.
//
// The loop:
//
//   - The lock at Geometry.LockPath is the one decision whether this process is the dobby.
//     A lock that stays held returns ErrDobbyRunning.
//   - A beam dobby ends when its worktree is gone, after it stops the tidy and describer agents it last saw in flight.
//     The test is the absence of the worktree's `.git` and never of its root,
//     because every state write recreates the record's directory.
//     The prime's dobby never takes the exit.
//   - A beam dobby whose owner reads as another machine hands its runs over through the catch-up, archives
//     the beam's local tips and exits, unless a hold this machine wrote covers the beam's subject.
//     A failed step logs once per episode and keeps the dobby up to retry.
//   - A tick's failure is logged once per episode and the loop goes on.
//   - Between ticks the binary check re-execs onto a changed lym binary.
//   - Any existing path of Geometry.MoveRecords means an anchor change stands.
//     While it stands, a tick spawns no pass, starts no task, runs no tidy or landing step and sends no notice.
//     An owed full pass stays owed.
//     An empty list gates nothing.
//   - The log records changes only, each one line stamped with its time, and a heartbeat.
//     The in-process memory of logged states means a re-exec logs no change it did not see.
//
// The passes:
//
//   - Each tick lists the runs whose parent worktree is this dobby's, from the prime's local batten status files.
//     It never reads the board for this, so a run whose task Land pruned is still driven through its teardown.
//   - A run gets a pass only while batten's state is running and its Verdict is act.
//     A run never gets a pass while one of its passes is in flight, and a held driver lock reads as in flight.
//     A halted or paused run gets no pass until the orch or the operator resumes it with a hand pass.
//   - A done batten run is driven no further and takes no slot under `beam.max_runs`.
//   - A continue pass needs the run at Run-Shed with no full pass owed and no lift to answer.
//     Every other pass is full.
//     The state record keeps a full pass owed until one exits 0.
//   - A pass that exits as owned by another machine keeps the run's full pass owed and marks it owned elsewhere.
//     Dobby pauses nothing: it releases the parent lock record naming its child and owes the catch-up from origin.
//     It posts one notice per owner episode naming `lym adopt <subject>`.
//   - A covered pass, or a pass its catch-up stopped, counts as no failure.
//     The third failure in a row with one error text posts one notice to the nearest live orch.
//   - A task waits on the board as `draft` until released, and dobby starts it from an empty status.
//     A task starts only when its dependencies are done, its parent is this dobby's worktree,
//     no other machine owns it, no refusal awaits every start of its slug and no pause covers the hub or the slug.
//     Starts stop while `beam.max_runs` runs the dobby drives are in flight; `beam.max_runs: 0` starts nothing.
//   - While fewer than `beam.task_headroom` cgroup tasks are free, the tick starts no task.
//     The hold only delays starts, never a pass of a run already started.
//     `beam.task_headroom: 0`, a nil seam and a limit that does not read hold nothing.
//   - A first pass refused for a covering pause is no refused start and waits for the lift.
//     A first pass whose catch-up could not reach origin is no refused start either:
//     it owes the catch-up from origin, and the task starts once the catch-up succeeds.
//     It is a refused start when the dobby has no catch-up,
//     or when a catch-up has succeeded since an earlier such exit of the same entry.
//     Any other first pass that exits non-zero and leaves no status file is a refused start:
//     it gets one notice per refusal text and a retry backoff that doubles up to an hour.
//   - A front task is a task entry whose `front` is true, filed ahead of the worktree's other starts.
//     While one is open, the worktree starts no other task, and the view shows those behind it as `front_first`.
//     Deps.SettleFronts records an open front entry and runs the land action of one that landed.
//   - A released task whose slug has a finished run record carries the refusal in TaskView.StartRefusal,
//     except a done motif run of a task whose recipe resolves to raddle, which the start clears.
//   - Across machines only the owner machine's dobby of the parent runs.
//
// The slot rule:
//
//   - A run whose RunView carries NoSlot takes no slot under `beam.max_runs`.
//     NoSlot marks a pawl run whose landing hold does not stand, while it only waits on its members.
//   - A run whose RunView carries EntryEnded takes no slot either, whatever batten's state says.
//
// The leftover pairs:
//
//   - A listed run's pair on disk is left over, by kind, when no pass of it is in flight,
//     its driver lock is free and no cancel is under way, and its run is handed over, landed,
//     cancelled without the cancel keeping it, or ended on the board.
//     A paused run whose work reached its parent is not left over, since its Land is unfinished.
//   - An orphan pair is one that neither a board entry nor a batten record names; only the prime's dobby reads them.
//     It is left over once OrphanGrace has passed since its first sight and its last commit,
//     with no process of its slug alive.
//     The first-sight record keeps the grace across a restart.
//   - Each tick hands every leftover pair whose wait ran out to one call of Deps.TearDownLeftovers,
//     with a retry wait that doubles up to an hour after each failure.
//     A teardown in flight holds the binary check until it ends.
//     A nil TearDownLeftovers tears nothing down.
//   - The pair of a cancelled run whose cancel kept it stays, and each fresh start posts one notice naming it.
//
// The landing step:
//
//   - Each tick of a beam's dobby reads Deps.CanLand: the beam is flagged ready with nothing working under it.
//     The prime's dobby and a nil CanLand take no step.
//   - A passing reading with no closed episode in State.Land asks for the landing through Deps.Land,
//     which writes the land request the beam's pawl run consumes.
//     A refusal leaves the episode open and Land is asked again each tick.
//     A reading that does not pass clears the episode.
//     A failed post leaves the episode open.
//   - The step lands nothing by itself: Beam-Await re-checks the beam before anything lands.
//   - While Deps.BeamReady reads true, the start step starts no task of the worktree,
//     and each open member of the beam gets one notice naming `lym batten cancel <member> --remove`.
//     A nil BeamReady holds nothing.
//
// The run notices:
//
//   - On each probe tick dobby reads the child of every running, non-cancelling run through Deps.RunNotices.
//     It sends the notice the child's condition calls for: a dead driver, a provider usage limit,
//     an API error stall, or every agent idle for the quiet window.
//     A nil RunNotices.ReadChild sends none.
//   - The idle clocks leave out machine sleeps and paused intervals,
//     so a sleep or a pause can delay a notice and never raise one earlier.
//   - A notice goes once per episode.
//     The marker RunNotices.MarkerPath names keeps the keys, and the state record keeps the child's state,
//     so a re-exec sends no notice twice.
//   - RunNotices.Post tags each notice with the run's slug, so a newer notice replaces one the orch has not read.
//   - While an anchor change stands no notice goes.
//
// The wait reminders:
//
//   - A child running at a row of WaitReminders.Rows with no pause covering it is in a wait.
//     Beam-Await is not among the rows, since it waits on the beam's members as well as the operator.
//   - While WaitReminders.Away reads true, a wait as old as WaitReminders.Interval gets one reminder line
//     through WaitReminders.Post, and each further interval brings another.
//   - A nil WaitReminders.Post or RunNotices.ReadChild checks nothing, and an anchor change stops the check.
//
// The landing notice:
//
//   - `beam.pull_request`, `beam.land_notice`, `beam.autonomy` and `beam.run_watch` are per-task settings,
//     read only through LookupPullRequest, LookupLandNotice, LookupAutonomy, LookupRunWatch and LookupWithSource.
//     LookupWithSource also names the worktree of the chain whose own files set the value, or the hub-wide default.
//     A task's value is the one its own worktree's files set, else each parent worktree's own value up to the prime,
//     else the hub's default layer's, else the template's.
//   - LookupAutonomy answers an AutonomyLevel: false, true or unattended.
//     Every path that checks the key takes the three alike and refuses any other value, naming them.
//   - Each tick reads Deps.LandNotice for each listed run ahead of its Land row and remembers the reading.
//     A failed read keeps the last reading and is logged once per episode; a nil LandNotice reads every run as off.
//   - A run seen past its Land row that dobby remembered ahead of it with `beam.land_notice` on owes one line
//     through Deps.Notice to the nearest live orch.
//     A run whose `beam.land_notice` read off owes none.
//   - A remembered motif run that landed owes one line saying its entry is now a Sketch,
//     whatever `beam.land_notice` reads, since a Sketch runs only once it is promoted;
//     its line replaces the landed line.
//   - A landed line asks the orch for nothing:
//     a member landing under a recorded pull request gets the description rewrite.
//   - An owed line stays in the state record and is posted every tick until a post succeeds.
//   - A run first seen past its Land row gets no line, and a done board entry owes none by itself.
//
// The run watch:
//
//   - Each scheduling tick reads every listed run:
//     its derived state, batten's state, row, reason and history, and the child's state, row, reason and history.
//     The child is read only when a status file was written since its last read.
//     A child that does not read or has no status keeps the child part read last.
//     So does a child written after the runs were listed, which the next tick reads;
//     with no reading yet, the run keeps none for that tick.
//   - A run's `beam.run_watch`, read through Deps.RunWatch.Level, is stops, rows or off.
//     Stops owes a line for a change to a stop, rows for every change, off for none.
//     A run whose level never read keeps no reading, so its first read tells the standing stop.
//   - The level kept is the one read ahead of the Land row:
//     the task worktree that sets it is torn down before batten reads done, so a later read would answer a parent's.
//   - A stop is a derived state of paused, halted, cancelling, cancelled or landed,
//     or a batten or child state other than running.
//     A stop is new when the derived state, batten's state, the child's state, whether the child is read
//     or the episode changed.
//     The episode is a history length plus its newest entry's folded repeats, so a repeated block at one row counts.
//   - A run listed Waiting, whose live driver waits out a network failure, keeps its last reading and owes no line.
//   - A run listed WaitDied, whose driver died while it waited, reads as one stop,
//     and its line names the producer and failure class the wait record holds.
//   - A landed run whose `beam.land_notice` read on owes no line, since the landing notice reports it.
//   - The memory is State.Watches, keyed by slug, which outlives the run's entry in the pass records.
//     It is dropped once the run is no longer listed, owes nothing and has no batten status.
//   - An owed line stays in the memory until Deps.RunWatch.Post posts it, oldest first.
//     A post that finds no orch counts no attempt.
//     A failing post is retried a probe interval later and dropped after three failures,
//     and a line owed an hour of awake time is dropped.
//   - A nil Level or Post watches nothing.
//
// The gate-hold notice:
//
//   - On each probe tick that schedules, dobby reads the held gate slots through Deps.GateHolds.
//     The slots one process holds are one hold, which starts at the earliest of them.
//     A hold belongs to the listed run whose task worktree it works in.
//   - The bound is `beam.gate_hold_factor` times the largest `gate.tiers.<tier>.timeout_min` of the hold,
//     or `beam.gate_hold_min` minutes when it has none; a factor of 0 sends nothing.
//   - The bound runs on awake time, so a machine sleep delays the notice.
//   - A hold past its bound gets one `[dobby notice]` line, whatever the run's watch level.
//     It names the slots, tier, site, start, bound, the holder's process tree with each process's CPU time,
//     and the processes outside the tree that carry the holder's environment marker.
//     Each list names the busiest processes and counts the rest.
//     A tree or marker scan that fails still sends the notice, naming the tree as unreadable.
//   - The noticed holds are kept in State.Watches, so a re-exec sends no second line, and a failed post is retried.
//     A new hold, by another process or with another start, is noticed again.
//   - The step only reads: it ends no process and frees no slot.
//
// The board tidy:
//
//   - Each tick records every Land Done that names this machine as counted in the state record,
//     so a restart counts none twice and a record read for the first time counts every earlier landing as seen.
//     While a tidy agent is in flight nothing is counted.
//   - At every `beam.board_tidy_every` landings, 0 meaning never,
//     dobby spawns the board-tidy agent unless one is in flight.
//     The agent's one output is the report `.lym/beam/tidy/<UTC timestamp>.md`, which dobby posts to the nearest live orch.
//     Past `beam.board_tidy_timeout_min` dobby ends the agent and posts a no-report line.
//   - On the prime's dobby, whose worktree is on the main branch, the agent also runs the upkeep checks before its report.
//     Deps.UpkeepCall renders their gate call, and the agent files a board note for each failure they report.
//     With no call the agent runs no upkeep checks: the hub's wiring renders none under a scoper other than go,
//     and a call that does not render is logged.
//   - A landing whose run records `lym batten cleanup --apply` removed before dobby read them is not counted.
//
// The description rewrite:
//
//   - A beam's dobby whose Deps.BeamDescribe.Record is set records each landing it counts as one to describe.
//     A landing that waits on a board-tidy agent in flight is described once the tidy settles.
//   - While Deps.PullRequestRecorded reads a pull request recorded for the beam,
//     dobby spawns the describer agent over the landings to describe, unless one is in flight.
//     The agent rewrites the beam's description whole.
//     It reads the recorded one, the beam's commits and every member pull request.
//     Its one output is a draft under `.lym/beam/describe/`, named by its UTC timestamp.
//   - Landings counted while no pull request is recorded are dropped;
//     an unreadable landing record keeps them until it reads, logged once per episode.
//   - Once the draft passes Deps.BeamDescribe.Findings, dobby ends the agent.
//     It records the draft through Deps.BeamDescribe.Record,
//     which replaces the beam's description, commits it and updates the pull request.
//   - A spawn that fails, a record that fails, and an agent with no passing draft by its timeout
//     leave the recorded description standing.
//     Dobby posts one line to the nearest live orch naming why,
//     and the next member landing rewrites the description again.
//     A record that committed the description but not the pull request update posts the way forward `lym beam pr update`.
//   - An error of the description step is logged once per episode and holds no other step of the tick.
//   - The prime's dobby rewrites nothing.
//
// The done-run clear:
//
//   - Each tick with no anchor change standing removes the records, scratch and task branch of each listed run
//     whose batten status reads done, through Deps.ClearDoneRun,
//     so a finished run never refuses a later start under its slug.
//   - A run is cleared only when no pass of it is in flight, its driver lock is free, its pair is gone,
//     and none of its landing notice, its board-tidy count and a watch line is still owed.
//     A run whose watch entry holds no level waits too, as does every run on a tick whose watch step failed to record.
//     The clear drops the run's watch entry.
//   - A failed clear is logged once per episode and never fails the tick.
//
// The catch-up:
//
//   - CatchUp brings this machine's view of its runs up to origin's after an absence, before any act rule answer.
//     It acts through the seams of CatchUpDeps, so the package imports no fabric or teardown package.
//   - A run owned here that is Ahead of origin gets PushOwed.
//     A run owned elsewhere gets HandOver, naming its owner.
//   - CatchUpReport lists the runs pushed, handed over and owned here, and a caller reads every listed run as touched.
//   - An unreachable origin returns an error wrapping ErrOriginUnreachable before any run is acted on.
//     A failed push or hand-over is joined into the returned error and the other runs go on.
//     A nil seam is a wiring error returned at once.
//   - The dobby owes the catch-up at a fresh start, after a sleep,
//     at a re-exec only while State.CatchUp records a wait,
//     when a run's pass exits PassExitOriginWait, PassExitCatchUpRefused or PassExitOwnedElsewhere,
//     and when a first pass exits PassExitOriginWait that the catch-up can still answer.
//     Deps.CatchUp with no PullBolt means no catch-up.
//   - While the catch-up is owed the tick spawns no pass, starts no task, takes no landing step and runs no tidy step.
//     A failure retries with a doubling delay and records State.CatchUp as a CatchUpWait,
//     which keeps when failures of its reason began.
//     A wait on origin posts no notice of its own;
//     the prime's push summary reports every live dobby's wait on origin past its threshold.
//
// The push summary:
//
//   - A failed push sends no mail of its own.
//     The prime's dobby, the one wired with Deps.PushSummary.Ledger, reads each tick what is owed to origin:
//     the push ledger's owed entries, every run of the hub whose driver waits on the network,
//     and the catch-up of its own and of every live beam dobby while it waits on origin.
//     Only an anchor change stops the read.
//   - The owed period starts at the earliest thing owed and lasts while anything is.
//     Once it has lasted PushSummaryDeps.Threshold, one notice lists each owed branch with its minutes,
//     a refused one with its way forward, each waiting run with its producer,
//     and each catch-up waiting on origin with its minutes.
//     No further summary follows in that period.
//   - An entry holding only held records, dropped landings or owed hand-overs is listed as held.
//     It opens and holds no period, so it does not block the all-clear.
//   - Once nothing is owed after a summary, one notice says every branch is pushed, and the period ends.
//   - A failed post is tried again on the next tick and logged once per episode.
//
// The desktop launcher:
//
//   - Each start of the dobby adds the hub's desktop launcher when Deps.Owner reads the worktree owned by this machine.
//   - Every catch-up removes it once OwnsAnything reads that this machine owns nothing in the hub.
//   - A machine with no desktop keeps no launcher; any other failure is logged at Warn and stops nothing.
//
// The view:
//
//   - BuildView carries `beam.max_runs` and the tasks waiting with the reason each waits.
//     It also carries the kept pairs of cancelled runs and the listed runs that are not done.
//     Each listed run says whether it asks for a landing notice.
//   - The view does not read the task-limit hold, and the dobby log records it.
//
// Told-geometry tier: the package takes every absolute path from its caller and imports neither internal/lymcwd nor any CLI package, per PATTERN-cwd-resolution.
// Its seam_enforcement_test.go enforces that membership mechanically.
package dobbyengine
