// Package orchcli is the CLI for the hub orchestrator:
// the long-lived session in the hub's prime worktree, or in any beam, that runs the task loop for the operator.
// `BeamUp` and `BeamDown` bring that session up and take it down.
// Its verbs host it as an interactive agent in a reed strand and cycle its context before it fills:
// `status`, `refresh`, `distill`, `withdraw`, `stop`, `rules` and the watcher behind them.
// The sections below also record the loop the session runs, which the verbs exist to serve.
//
// # The orch in a beam
//
// Every verb runs in the hub's prime worktree and in a beam, a pair whose own origin record carries the beam mark.
// Every other worktree refuses it, naming the beam verbs that make a pair one.
// There is one orch per worktree.
// A beam's orch keeps its files under that worktree's own `.lym/orch/` and `_lym/orch/`.
// Its strand is `<shortname>:<slug>:orch`, and its watcher is its own.
// `BeamUp` refuses three cases:
// a worktree another machine owns, naming the owner and `lym adopt <slug>` (`lym adopt root` for the prime);
// a worktree with an anchor change standing on it or on the prime, naming the change's way forward;
// and a beam whose board entry a run other than the beam's own landing holds, naming `lym batten status <slug>`.
// A beam held by its own pawl run's landing is no refusal:
// `ownLandingHold` reads it from the beam pair's own run status.
// The status is found, not done, and standing at the producer the board status names.
// The bring-up then starts the orch alone, with a warning that the dobby stays down while the landing runs.
// It judges the refusals from the local Bolt.
// The judgment runs after the reconcile when `Prepare` ran, and before any dobby, window or orch starts.
// `beamUpRefusal` is the pure judgment over the facts `readBeamFacts` reads.
// The hub-scope origin reconcile runs from the prime whichever orch is brought up.
// After the refusals `BeamUp` starts this worktree's dobby through `dobbyengine.EnsureLive`, only when it is not live.
// The dobby is the first step of the pair it brings up with the runs strand, the orch and its watcher.
// A dobby that does not start never stops the bring-up: it becomes a warning naming the cause and `lym beam start`.
// After the dobby step the prime's bring-up starts the hub's Slack relay through `slackengine.EnsureRelay`.
// It starts nothing for a hub with no Slack setup, and a beam's bring-up starts no relay.
// A relay that does not start never stops the bring-up either: it becomes a warning naming `lym slack start`.
// The runs strand holds the session's first overview window.
// A `loom-status` strand a pawl run's `lym loom start` added sits beside it.
// A runs strand the bring-up could not add, replace or record is a warning on the report, never a stop.
// The warning names the cause and `lym beam start`.
// `BeamUpWorktree` is the bring-up for any other verb that needs a worktree's dobby live:
// it names the worktree from the prime and runs `BeamUp` without `Prepare`.
// It returns a dobby that was refused or failed as an error, so no verb starts a dobby without its orch.
// `BeamDown` removes the orch strand, after which the watcher exits on its own.
// It ends the worktree's dobby through `runproc.EndRole`, so no other process of the beam is signalled;
// each part stops only when live.
// For the prime it then ends the test-map refresh the dobby started, whether or not the dobby is live.
// The refresh is the `runproc.RoleTestMap` record under `runproc.HubOwner`.
// That refresh has its own record and process group and so outlives the dobby's end.
// `OrchLive` is the liveness predicate other modules call:
// the worktree's orch state records a strand that reed tracks and reports live.
//
// # The role file and the note
//
// The procedure lives in the orch role, one fixed text for every orch built into the binary, not in the launch prompt.
// `BeamUp` renders it to `.lym/orch/role.md` and launches the session with a one-line pointer at it.
// Shuttle loads the orch's skills before that pointer on every launch path, a resume included;
// the skills come from the `orch` role in agents.yaml.
// `BeamUp` records the orch session's id in orch's state and resumes it after a reboot, a `stop` or a dead strand.
// When the resume fails it falls back to the last note, else the start stencil;
// the internal/orchengine package documentation's `Continuing after a reboot` section has the detail.
// A context cycle asks the session for an orch note at the path it names, following `.lym/orch/note-template.md`.
// The note stands in for a general handoff skill.
// `lym orch refresh` requests a clear cycle and `lym orch distill` a compact cycle, whatever `cycle_mode` says;
// both write the note first.
// The role's on-demand files render beside it, and the role carries no command list:
// it sends the session to `lym help index`.
//
// # The cycle request
//
// `refresh` and `distill` write a request marker the watcher acts on at the session's next idle moment.
// A hub pause holds the watcher's cycle machine, so a request made under a pause waits for the resume,
// and the timeout that abandons it counts no pause time.
// A request whose cycle has not started within the handoff timeout is abandoned;
// once started, the session has the full handoff timeout to write its note.
// The timeout counts from when the session reads the watcher's note request.
// With a live watcher and a live hub pause entry, the envelope of both verbs adds `held_by_pause: true` and a `note`,
// and the request is still written;
// with no live watcher the request is removed and the envelope carries no such key.
// `status` reports the marker as `cycle_request`: null while none is pending, else an object.
// The object holds `mode`, `requested_at` (RFC 3339 UTC, null for a marker that does not parse) and `held_by_pause`.
// A pause record that cannot be read counts as no pause.
// `withdraw` removes a pending request of either mode in any phase and answers `withdrawn: true`.
// With none pending it answers `withdrawn: false` with `phase`.
// It adds `cycle_trigger` only when the phase is neither idle nor resuming.
// Returning to idle does not reset the trigger, and a reload start does not set it.
// It never refuses: it removes only the marker, never a started cycle's state.
// A marker present during a cycle belongs to a cycle that has not started:
// a cycle start consumes its marker under the orch state lock.
//
// # The watcher leaves a beam it no longer owns
//
// `watch` tells its watcher the owner read `beamOwnerElsewhere` and the strand removal.
// A beam another machine owns therefore loses its orch strand with its dobby.
// The read takes the prime's owner for the prime and a beam's effective owner through `hubgeom.ParentOf`.
// It reads from the hub's local Bolt and compares the owner with this machine's name from `machinename`.
// The verdict is as fresh as that Bolt.
//
// # Status restarts a dead watcher
//
// `status` is what the orch and the operator run when the orch goes quiet,
// so a live strand with no watcher holding `watch.lock` gets a new watcher from it.
// A strand counts as live by the engine liveness predicate, which a shell-only pane past the grace fails.
// The restart takes `start.lock` with a try-lock.
// When a `start` or `stop` holds it, the restart does nothing and adds a `hint` to rerun.
// Holding the lock it reads the state, the strands and the watch lock again.
// It spawns only when the recorded strand is still tracked and live with no watcher.
// It then logs `orch: restarted a dead watcher` with the previous watcher's recorded exit.
// A spawn failure is the error envelope with the way forward `lym beam start`.
// The envelope's `watcher_restarted` is true only when this call spawned one.
// `watcher_live` keeps what the probe found.
// `strand_elsewhere` names the placement of a pane sitting in another session of reed's server.
// A dead strand gets no watcher from `status`:
// `BeamUp` relaunches strand and watcher together,
// and a shell-only orch reads dead, so it resumes its recorded session in one step.
// `status` writes only this one spawn.
// Two calls in sequence may spawn two watchers when the second starts before the first's watcher takes `watch.lock`;
// `watch.lock` keeps one.
// `stop` takes `start.lock` around its strand read and removal.
// A restart therefore never binds a watcher to a strand `stop` removes.
// `stop` waits up to a minute for a running `start` and then refuses with a way forward.
//
// # The orch rules
//
// A repository's orch rules are the heddle entry `orch-rules`, aimed at heddle's `orch-prompt` target.
// Heddle appends that target to the system prompt of an orch session alone.
// `lym orch rules --from <file>` replaces that entry's text with the file's content through heddle's write path.
// The write runs under the Bolt write lock and is followed by the board's detached Bolt sync.
// An empty file removes the entry, clearing the rules.
//
// # The orchestrator acts and fixes
//
// The orchestrator brings the operator design questions only:
// what to build, a task's scope and a choice between designs.
// Every run-level call is its own and recorded as a parent decision:
// an arbiter's hand-on question, a `decision` escalation, a hard-cap accept, a landing stop.
// It releases a task only on the operator's go-ahead naming it.
// It watches runs through their notices rather than by polling their status, and it never changes code in the prime.
// Runs that can run in parallel do.
// Only a task whose packages overlap a running task is held back, through its board `depends_on`.
// Every status report on a running run carries its `lym reed attach` command.
//
// # The run loop, per task
//
//  1. Create the task worktree with `lym fabric add <slug>`, then start its run from inside it with `lym loom start`.
//     The run's status file is `_lym/shed/<slug>/status.json` under the worktree.
//     Its `state` is one of running, awaiting, blocked, failed or done.
//  2. Keep no watch loop of your own.
//     A batten or dobby notice arrives in full as its own quoted `[creel] #<id> from "<sender>": …` line.
//     The orch session's hooks deliver it, whichever claim it was posted under.
//     Read a stop it names with `lym batten status <slug>`.
//  3. The run's own tail merges, reviews and lands without an approval step.
//     The tail is Parent-Align and its fixer, the Code-Review ring and Describe.
//     Finalize then takes the parent lock first and lands on the parent.
//     No approval step follows; a landing halt at Parent-Align or Finalize is the orchestrator's to read.
//  4. On blocked or failed, read `error` in the status file.
//     Rerun the failing tests to tell a flaky failure from a real one.
//     A landing halt at Parent-Align or Finalize reaches the orchestrator as it stands.
//     No driver, batten or arbiter routes it to another row, and the status marks it handed back.
//     A real regression in the task is fixed in the task worktree by a background subagent, with code commits only:
//     no `lym fabric`, no `_lym` or `.lym` edits and no push.
//     The run is started again.
//     A lym bug is fixed through a task of its own, never in the prime, and the run is started again once it lands.
//     Never merge the parent into a task while webster is mid-batch, because record-batch then refuses the batch;
//     a merge-in is safe only once every batch is recorded, for example at a blocked integration verify.
//  5. Batten's Land row lands the run:
//     it pulls the prime, runs the hub's landing `deploy` command and prunes the board,
//     and Worktree-Teardown then removes the pair.
//     The orchestrator acts on a Land or teardown failure notice and on a ready-task notice.
//     It starts the tasks a ready notice names.
//     While the hub's landing `deploy` key is empty, it deploys by hand with `update-plugins.sh`.
//     It does so on a ready notice or at its next wake after a landing.
//     It reads the drive report only when the run had trouble, and it records findings.
//     Friction findings are filed as LoomYard issues.
//     They reach the board through intake when the operator asks for it.
//
// # Priorities
//
// In order:
// fewer and recoverable stops, since a false stop costs as much as a wrong landing and every refusal needs a way out;
// an automated loop, through batten, the driver's own repairs and the orchestrator's context cycling;
// the cycling uses `/compact` or `/clear` as orch.yaml's `cycle_mode` says;
// records that survive teardown; and the operator's surface, meaning panes, the IDE workspace and the launch line.
//
// # Config after a binary change
//
// No orch verb reconciles the hub's config after a binary change;
// `lym beam start` does, through `hubreconcile.Ensure`, before it calls `BeamUp`.
//
// # The origin reconcile at bring-up
//
// With `BeamUpOptions.Prepare`, `BeamUp` runs the hub-scope origin reconcile from the prime.
// The reconcile runs once `BeamUp` holds the start lock and before it decides anything about the strand,
// so the orch never acts on a stale view of origin.
// A caller that already judged origin leaves `Prepare` false.
// Adopt does so after its own reconcile, and so does the pawl run's Beam-Up.
// The refusals still run.
// The reconcile never refuses the bring-up:
// a stop, an unreachable origin and a reconcile error are reported for the orch to act on,
// and a run already driving is reported live or stopped when origin moved its branch.
// With `Prepare`, the `BeamUpReport` carries `Reconcile` and `FailedPushes`, the hub's outstanding failed pushes.
// It also carries the `Mutations` and `Partial` pair of a mutating step.
// The reconcile can remove pairs, write snapshot tags and move branches.
// An unreachable origin or a reconcile error is a warning.
// A failure after the reconcile ran carries the report's mutations, with `Partial` true when the record is non-empty.
// A fresh launch's prompt carries a line summarising the report and the outstanding failed pushes.
// The summary includes the owned-elsewhere lines.
// Under an operator's live hub pause the reconcile's network calls wait at the network gate until the lift.
//
// # The prime claim and the owner pause at bring-up
//
// After the reconcile, `Prepare` claims the prime for this machine through `claimPrime`.
// The claim runs in the prime and in a beam alike, and writes only when the board records no prime owner.
// A claim, a claim lost to another machine and a failure are reported under the warnings, and none refuses the bring-up.
// Before it touches any strand, `Prepare` then pauses each owned_elsewhere entry of the report through `pauseOwned`.
// `InstallOwnerPause` stores `pauseOwned` for the process;
// the root pre-run installs `tendercli.PauseOwnedElsewhere`.
// `tendercli` imports this package, so the reverse import would cycle.
// The paused slugs with their owners are the report's `OwnerPauses`.
// A failed or unwired pause is a warning naming that `lym beam start` pauses it again.
// A nil `claimPrime` or `pauseOwned`, as a hand-built start harness has, is skipped.
//
// # Withdrawing around a fresh launch
//
// Wherever a bring-up resets the orch state onto a session, it signs creel calls as the watcher's program.
// That covers a relaunch, a resume and an adopted live strand alike.
// It runs two rounds on the orch strand's name, the listed strand's name or else the fixed `<shortname>:orch`.
// A hub with no shortname record fails the bring-up before any round when no orch strand is listed.
// The first round runs before the bring-up starts or binds a session and before it removes the old strand.
// It is a withdraw of the old cycle's queued steps, and a release only when an orch strand is listed.
// A failed withdraw, a refusal or an unreachable daemon alike, fails the bring-up.
// The failure carries creel's error and a way forward, and nothing is started, bound or removed.
// Creel's refusals for a program claim from a non-orch pane or for an unknown pane process carry no code.
// That way forward is therefore one fixed text naming both causes.
// The second round runs after the reset state is saved.
// It is a withdraw and a release that catch a step an old watcher still running queued in between.
// A failed first release or a failure in the second round does not fail the bring-up;
// the error text is recorded in the orch state's `fresh_launch_failure`, which the watcher retries on every tick.
// A withdrawn resume line goes to `orchengine.ClearResumeNoticed`, so a resumed session gets the line posted again.
// The call carries the orch GUID the state records at that call.
//
// # The session-start hook
//
// The orch run's spec is the one that installs the provider's context-after-compaction hook.
// Its command is built through the POSIX shell dialect, which the provider runs hooks under on every OS.
// The command changes directory to the acting orch's own anchor, the beam's in a beam.
// It then runs the bare hidden verb `lym orch resume-context`.
// The verb therefore runs in that worktree whatever directory the session has moved to.
// Its own prime-or-beam refusal bounds a failed change of directory.
// It prints the resume pointer as the provider's additional-context JSON and nothing else.
// It also writes the delivery mark the watcher reads.
// A failure prints the JSON error envelope and writes no mark, so the watcher types the pointer itself.
//
// # Reaching the orch from another module
//
// `PrimePaths` is the one accessor other modules use for the orch's told paths,
// so the orch state is reached without re-deriving it.
// A module sends the orch a notice as creel mail and never types into the orch session itself.
// `NotifyPrime` is the way a task-worktree module does that.
// It takes the task worktree's location, the module's program name and a line.
// It resolves the prime itself and posts the line to the prime's orch strand through creel.
// It reports whether the line was posted, and a prime with no orch strand recorded posts nothing and is no error.
// `PrimeOrchStrand` answers the name of that strand.
// That is the name reed's state holds for the recorded GUID.
// For a GUID reed no longer holds it is the fixed `<shortname>:orch` name, which creel then accepts or refuses.
// `OrchStrandOf` answers the same for any worktree's orch, the fixed name being `<shortname>:<slug>:orch` in a beam.
// A notice of a run goes to the nearest live orch up its pair's beam chain, the prime's when none is live:
// `NotifyParent` resolves it from the pair's recorded parent worktree at each call.
// `NotifyFrom` starts the walk at the given worktree itself, for a run whose pair is absent.
// `ParentOrchRecorded` answers whether that destination records a strand.
// A driver reads it as the watched-or-silent decision.
// `NotifyAsker` is the hold notice's route:
// it resolves the orch with `orchroute.Asker`, the rule `lym creel post --parent` uses,
// so a held agent's notice reaches the orch its own question reached.
// The walk starts at the worktree itself from a beam or the prime.
// Callers that act on no run's pair keep `NotifyPrime`.
// `HubOrchs` lists the worktrees an orch may run in, for the readers that must cover every orch of the hub.
// They are the prime's location and each local pair whose origin record carries the beam mark.
// It skips a pair still being created, in its staging directory or without its `_lym` junction.
// It reads nothing through such a pair, so a poll during `lym fabric add` leaves nothing the wiring refuses.
// A beam's `watch` tells its watcher the orch's own agent name,
// so the pause checks match an orch caller or network operation marker against that name.
// The prime's watcher keeps matching an orch name with no slug.
// `InstallPushNotifier` routes fabric's failed-push notice to the prime orch as mail signed `FABRIC`.
// The root pre-run installs it for every lym process.
// It resolves the hub from the working directory as the network gate does.
// A detached push child starting in the board worktree is therefore covered, and nothing is installed outside a hub.
// `InstallReconcileNotifier` routes the hub config reconcile's reinterpretation notice to the prime orch.
// The mail is signed `RECONCILE`.
// The notice says what a binary's change of a config value's meaning rewrote or flagged in the hub's files.
// It resolves the hub from the working directory, as the network gate does.
package orchcli
