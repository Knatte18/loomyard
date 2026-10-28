// Package orchcli is the CLI for the hub orchestrator: the long-lived session in the hub's prime worktree, or in any beam, that runs the task loop for the operator.
// `BeamUp` and `BeamDown` bring that session up and take it down, and its verbs (`status`, `refresh`, `distill`, `withdraw`, `stop`, `rules` and the watcher behind them) host it as an interactive agent in a reed strand and cycle its context before it fills; this comment records the loop the session runs, which the verbs exist to serve.
//
// # The orch in a beam
//
// Every verb runs in the hub's prime worktree and in a beam, a pair whose own origin record carries the beam mark, and is refused in every other worktree with the beam verbs that make a pair one.
// There is one orch per worktree: a beam's orch keeps its files under that worktree's own `.lym/orch/` and `_lym/orch/`, its strand is `<shortname>:<slug>:orch`, and its watcher is its own.
// `BeamUp` refuses a worktree another machine owns, naming the owner and `lym adopt <slug>` (`lym adopt root` for the prime), a worktree with an anchor change standing on it or on the prime, naming the change's way forward, and a beam whose board entry its landing holds, naming `lym batten status <slug>`.
// It judges them from the local Bolt, after the reconcile when `Prepare` ran and before it starts any dobby, window or orch, and `beamUpRefusal` is the pure judgment over the facts `readBeamFacts` reads.
// The hub-scope origin reconcile runs from the prime whichever orch is brought up.
// After the refusals `BeamUp` starts this worktree's dobby alone through `dobbyengine.EnsureLive`, only when it is not live, then ensures the runs strand and starts the orch and its watcher.
// A dobby that does not start never stops the bring-up: it becomes a warning naming the cause and `lym beam start`.
// `BeamDown` removes the orch strand, after which the watcher exits on its own, and ends the worktree's dobby through `runproc.EndRole`, so no other process of the beam is signalled; each part stops only when live.
// `OrchLive` is the liveness predicate other modules call: the worktree's orch state records a strand that reed tracks and reports live.
//
// # The role file and the note
//
// The procedure below lives in the role stencil, not in the launch prompt.
// `BeamUp` renders it to `.lym/orch/role.md` and launches the session with a one-line pointer at it; skills (`scribe:prose`, `scribe:conversation`, `ly:board`) are loaded by shuttle before that pointer on every launch path, a resume included.
// `BeamUp` records the orch session's id in orch's state and resumes it after a reboot, a `stop` or a dead strand, falling back to the last note, else the start stencil, when the resume fails; see the internal/orchengine package documentation's `Continuing after a reboot` section.
// A context cycle asks the session for an orch note at the path it names, following `.lym/orch/note-template.md`, in place of a general handoff skill.
// `lym orch refresh` requests a clear cycle and `lym orch distill` a compact cycle, whatever `cycle_mode` says; both write the note first.
// The role file's `## Commands` section holds the operator command index.
// Its `## Hub-wide rules` and `## This orch's rules` sections hold the hub-wide rules file on Bolt and this orch thread's own rules file under its worktree's `_lym/orch/`, re-read at every delivery.
// `BeamUp` renders it from the `help index` of the binary it runs as, and `resume-context` in-process from the binary it runs as, which the pane names;
// the watcher outlives a deploy, so it runs `help index` from the `lym` path the orch strand recorded and goes idle with a reason naming `lym orch stop` when that path is missing or gone.
//
// # The cycle request
//
// `refresh` and `distill` write a request marker the watcher acts on at the session's next idle moment.
// A hub pause holds the watcher's cycle machine, so a request made under a pause waits for the resume, and the timeout that abandons it counts no pause time.
// A request whose cycle has not started within the handoff timeout is abandoned;
// once started, the session has the full handoff timeout to write its note, counted from when it reads the watcher's note request.
// With a live watcher and a live hub pause entry, the envelope of both verbs adds `held_by_pause: true` and a `note`, and the request is still written; with no live watcher the request is removed and the envelope carries no such key.
// `status` reports the marker as `cycle_request`: null while none is pending, else an object with `mode`, `requested_at` (RFC 3339 UTC, null for a marker that does not parse) and `held_by_pause`.
// A pause record that cannot be read counts as no pause.
// `withdraw` removes a pending request of either mode in any phase and answers `withdrawn: true`.
// With none pending it answers `withdrawn: false` with `phase`, and with `cycle_trigger` only when the phase is neither idle nor resuming, since `toIdle` does not reset the trigger and a reload start does not set it.
// It never refuses: it removes only the marker, never a started cycle's state, and a marker present during a cycle has not started, because a cycle start consumes the marker it starts from under the orch state lock.
//
// # The watcher leaves a beam it no longer owns
//
// `watch` tells its watcher the owner read `beamOwnerElsewhere` and the strand removal, so a beam another machine owns loses its orch strand with its dobby.
// The read takes the prime's owner for the prime and a beam's effective owner through `hubgeom.ParentOf`, from the hub's local Bolt, and compares it with this machine's name from `machinename`.
// The verdict is as fresh as that Bolt, and a watcher started by an older build reaches the check only once it restarts, `lym beam status` showing the orch up meanwhile.
//
// # Status restarts a dead watcher
//
// `status` is what the orch and the operator run when the orch goes quiet, so a live strand with no watcher holding `watch.lock` gets a new watcher from it.
// A strand counts as live by the engine liveness predicate, which a shell-only pane past the grace fails.
// The restart takes `start.lock` with a try-lock and restarts nothing, adding a `hint` to rerun, when a `start` or `stop` holds it.
// Holding the lock it reads the state, the strands and the watch lock again and spawns only when the recorded strand is still tracked and live with no watcher, then logs `orch: restarted a dead watcher` with the previous watcher's recorded exit.
// A spawn failure is the error envelope with the way forward `lym beam start`.
// The envelope's `watcher_restarted` is true only when this call spawned one, `watcher_live` keeps what the probe found, and `strand_elsewhere` names the placement of a pane sitting in another session of reed's server.
// A dead strand gets no watcher from `status`: `BeamUp` relaunches strand and watcher together, and a shell-only orch reads dead, so it resumes its recorded session in one step.
// Bound: `status` writes only this one spawn, and two calls in sequence, the second before the first one's watcher takes `watch.lock`, may spawn two watchers, of which `watch.lock` keeps one.
// `stop` takes `start.lock` around its strand read and removal, waiting up to a minute for a running `start` and then refusing with a way forward, so a restart never binds a watcher to a strand `stop` removes.
//
// # The orch rules
//
// `lym orch rules --from <file>` replaces the hub-wide rules file on Bolt with the file's content, pulling Bolt first and pushing before and after the commit.
// The verb commits only that one path, and an empty file clears the rules.
// The orch thread's own rules file is committed through `lym fabric commit`.
//
// # The orchestrator acts and fixes
//
// The orchestrator talks to the operator about what to build, priorities and design choices.
// Everything else is its own: start runs, un-wedge stuck runs, answer the landing halts, land, deploy, clean up, record findings and start the next task.
// Un-wedging includes small code fixes committed and pushed to `main` from the prime, followed by the production deploy.
// Runs that can run in parallel do, and only a task whose packages overlap a running task is held back, through its board `depends_on`.
// Findings go on the board, never into GitHub issues.
// Every status report on a running run carries its `lym reed attach` command.
//
// # The run loop, per task
//
//  1. Create the task worktree with `lym fabric add <slug>`, then start its run from inside it with `lym loom start`.
//     The run's status file is `_lym/shed/<slug>/status.json` under the worktree, and its `state` is one of running, awaiting, blocked, failed or done.
//  2. Keep no watch loop of your own: a batten notice arrives in full as its own quoted `[creel] #<id> from "<sender>": …` line through the orch session's hooks, whichever claim it was posted under, and you check the run with `lym batten status <slug>`.
//  3. The run's own tail merges, reviews and lands without an approval step: Parent-Align and its fixer, the Code-Review ring, Describe and a Finalize that takes the parent lock first and lands on the parent.
//     No approval step follows; a landing halt at Parent-Align or Finalize is the orchestrator's to read.
//  4. On blocked or failed, read `error` in the status file and rerun the failing tests to tell a flaky failure from a real one.
//     A landing halt at Parent-Align or Finalize reaches the orchestrator as it stands, since no driver, batten or arbiter routes it to another row, and the status marks it handed back.
//     A real regression in the task is fixed in the task worktree by a background subagent, with code commits only, no `lym fabric`, no `_lym` or `.lym` edits and no push, and the run is started again.
//     A lym bug is fixed on `main`, deployed, and the run is started again.
//     Never merge the parent into a task while webster is mid-batch, because record-batch then refuses the batch; a merge-in is safe only once every batch is recorded, for example at a blocked integration verify.
//  5. Batten's Land row lands the run: it pulls the prime, runs the hub's landing `deploy` command and prunes the board, and Worktree-Teardown then removes the pair.
//     The orchestrator acts on a Land or teardown failure notice and on a ready-task notice, starting the tasks it names, and deploys by hand with `update-plugins.sh` on a ready notice or at its next wake after a landing while the hub's landing `deploy` key is empty.
//     It reads the drive report only when the run had trouble (friction findings are filed as LoomYard issues, which reach the board through intake when the operator asks for it) and records findings.
//
// # Priorities
//
// In order: fewer and recoverable stops, since a false stop costs as much as a wrong landing and every refusal needs a way forward;
// an automated loop, through batten, the driver's own repairs and the orchestrator's context cycling, by `/compact` or `/clear` as orch.yaml's `cycle_mode` says;
// records that survive teardown; and the operator's surface, meaning panes, the IDE workspace and the launch line.
//
// # Config after a binary change
//
// No orch verb reconciles the hub's config after a binary change; `lym beam start` does, through `hubreconcile.Ensure`, before it calls `BeamUp`.
//
// # The origin reconcile at bring-up
//
// With `BeamUpOptions.Prepare`, once `BeamUp` holds the start lock, and before it decides anything about the strand, it runs the hub-scope origin reconcile from the prime, so the orch never acts on a stale view of origin.
// A caller that already judged origin, adopt after its own reconcile and the pawl run's Beam-Up, leaves `Prepare` false, and the refusals still run.
// The reconcile never refuses the bring-up: a stop, an unreachable origin and a reconcile error are reported for the orch to act on, and a run already driving is reported live or stopped when origin moved its branch.
// The `BeamUpReport` gains `Reconcile`, `FailedPushes` (the hub's outstanding failed pushes), and the `Mutations` and `Partial` pair of a mutating step, since the reconcile can remove pairs, write snapshot tags and move branches.
// An unreachable origin or a reconcile error is a warning.
// A failure after the reconcile ran carries the report's mutations, with `Partial` true when the record is non-empty.
// A fresh launch's prompt gains a line summarising the report and the outstanding failed pushes, the owned-elsewhere lines of the summary included.
// Under an operator's live hub pause the reconcile's network calls wait at the network gate until the lift.
//
// # The prime claim and the owner pause at bring-up
//
// After the reconcile, `Prepare` claims the prime for this machine through `claimPrime` in the prime and in a beam alike, writing only when the board records no prime owner.
// A claim, a claim lost to another machine and a failure are reported under the warnings, and none refuses the bring-up.
// Before it touches any strand, `Prepare` then pauses each owned_elsewhere entry of the report through `pauseOwned`, which `InstallOwnerPause` stores for the process; the root pre-run installs `tendercli.PauseOwnedElsewhere`, since `tendercli` imports this package and the reverse import would cycle.
// The paused slugs with their owners are the report's `OwnerPauses`, and a failed or unwired pause is a warning naming that `lym beam start` pauses it again.
// A nil `claimPrime` or `pauseOwned`, as a hand-built start harness has, is skipped.
//
// # Withdrawing around a fresh launch
//
// Wherever a bring-up resets the orch state onto a session, a relaunch, a resume and an adopted live strand alike, it signs creel calls as the watcher's program and runs two rounds on the orch strand's name, the listed strand's name or else the fixed `<shortname>:orch`.
// A hub with no shortname record fails the bring-up before any round when no orch strand is listed.
// The first round runs before the bring-up starts or binds a session and before it removes the old strand: a withdraw of the old cycle's queued steps, and a release only when an orch strand is listed.
// A failed withdraw, a refusal or an unreachable daemon alike, fails the bring-up with creel's error and a way forward, and nothing is started, bound or removed.
// Creel's refusals for a program claim from a non-orch pane or for an unknown pane process carry no code, so that way forward is one fixed text naming both causes.
// The second round runs after the reset state is saved, a withdraw and a release that catch a step an old watcher still running queued in between.
// A failed first release or a failure in the second round does not fail the bring-up; the error text is recorded in the orch state's `fresh_launch_failure`, which the watcher retries on every tick.
// A resume line a withdraw removed is handed to `orchengine.ClearResumeNoticed` with the orch GUID the state records at that call, so a resumed session gets the line posted again.
//
// # The session-start hook
//
// The orch run's spec is the one that installs the provider's context-after-compaction hook.
// Its command is built through the POSIX shell dialect, which the provider runs hooks under on every OS: a change of directory to the acting orch's own anchor, the beam's in a beam, then the bare hidden verb `lym orch resume-context`.
// The verb therefore runs in that worktree whatever directory the session has moved to, and its own prime-or-beam refusal bounds a failed change of directory.
// It prints the resume pointer as the provider's additional-context JSON and nothing else, and writes the delivery mark the watcher reads.
// A failure prints the JSON error envelope and writes no mark, so the watcher types the pointer itself.
//
// # Reaching the orch from another module
//
// `PrimePaths` is the one accessor other modules use for the orch's told paths, so the orch state is reached without re-deriving it.
// A module sends the orch a notice as creel mail and never types into the orch session itself.
// `NotifyPrime` is the way a task-worktree module does that: it takes the task worktree's location, the module's program name and a line, resolves the prime itself, and posts the line to the prime's orch strand through creel.
// It reports whether the line was posted, and a prime with no orch strand recorded posts nothing and is no error.
// `PrimeOrchStrand` answers the name of that strand: the name reed's state holds for the recorded GUID, or the fixed `<shortname>:orch` name for a GUID reed no longer holds, which creel then accepts or refuses.
// `OrchStrandOf` answers the same for any worktree's orch, the fixed name being `<shortname>:<slug>:orch` in a beam.
// A notice of a run goes to the nearest live orch up its pair's beam chain, the prime's when none is live:
// `NotifyParent` resolves it from the pair's recorded parent worktree at each call, `NotifyFrom` starts the walk at the given worktree itself, for a run whose pair is absent,
// and `ParentOrchRecorded` answers whether that destination records a strand, the watched-or-silent decision a driver reads.
// `NotifyAsker` is the hold notice's route: it resolves the orch with `orchroute.Asker`, the rule `lym creel post --parent` uses, so a held agent's notice reaches the orch its own question reached, the walk starting at the worktree itself from a beam or the prime.
// Callers that act on no run's pair, hibernate's notice and fabric's push notifier, keep `NotifyPrime`.
// `HubOrchs` lists the worktrees an orch may run in, the prime's location and each local pair whose origin record carries the beam mark, for the readers that must cover every orch of the hub.
// It skips a pair still being created, in its staging directory or without its `_lym` junction, and reads nothing through it, so a poll during `lym fabric add` leaves nothing the wiring refuses.
// A beam's `watch` tells its watcher the orch's own agent name, so the pause checks match an orch caller or network operation marker against that name, and the prime's watcher keeps matching an orch name with no slug.
// `InstallPushNotifier` routes fabric's failed-push notice to the prime orch as mail signed `FABRIC`, for every lym process from the root pre-run.
// It resolves the hub from the working directory as the network gate does, so a detached push child starting in the board worktree is covered, and installs nothing outside a hub.
package orchcli
