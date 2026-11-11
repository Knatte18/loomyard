// Package adoptcli is the adopt module.
// Its verbs are `lym adopt <beam>...`, `lym adopt root` and `lym adopt --all`.
// It also has `lym adopt --here` and `lym adopt --archives <subject>...`.
// They take the flags `--keep-local`, `--drop-local`, `--no-orch`, `--discard`, `--here` and `--archives`.
// It takes over beam projects, or root, and the runs under them from the other machine of the operator.
// It calls each owning module's takeover function in-process, so no module learns another's state,
// and it has no engine package of its own.
//
// # The verb
//
// The verb runs from the hub's prime and from any beam, and acts on the prime's location in both;
// `battencli.ResolveBattenPrime` resolves the prime.
// `lym adopt` runs no step itself.
// `runAdopt` validates the request, starts a detached worker, `lym adopt worker`, and follows the adopt record.
// The worker runs the steps in order under `adopt.lock` and records each one, so a killed worker resumes.
// `request` holds the slugs, `Root`, `All`, `KeepLocal`, `DropLocal` and `NoOrch`;
// the word `root` among the slugs sets `Root`.
// `adoptDeps` holds every step's seam, which `hubDepsAs` fills over the hub and `TestAdopt` fakes.
// An entry whose slug is `root` is shadowed by that word and cannot be named.
// The word `worker` shadows a beam so named in the same way as `check` does, and `--all` still adopts such a beam.
//
// The arguments are beam projects (a project entry whose `beam` is true) and `root`, which names the prime.
// `--all` names `root` and every beam project that is not done.
// Only a beam project and the prime record an owner, so an adopt moves the named projects alone:
// each one's owner (the prime owner for `root`), its pair, and the runs whose parent is its worktree.
// A nested beam keeps its own owner.
// `lym adopt root` moves root's tasks and the tasks of the grouping projects below root.
// It never moves a beam's members, whose parent is that beam.
//
// `validateRequest` reads the local Bolt through `namedEntries` and refuses what cannot be adopted.
// An unknown slug is refused naming `lym board list`, and a note naming `lym board promote`.
// A done entry is refused naming that it already landed.
// To let a run that landed there go on to Land under its owner pause, the refusal names `lym tender resume <slug>`.
// A task is refused naming `lym adopt <beam>` or `lym adopt root`, the `AdoptSubject` that moves it.
// A grouping project is refused naming the same adopt, which moves its tasks.
// The refusal also names `lym beam create <slug>`, which makes it a beam.
// `--keep-local` with `--drop-local` is refused, and so is no argument without `--all`.
// The caller runs the validation before it spawns anything, and the worker runs it again before it takes the lock.
//
// `movedRuns` lists, sorted, the runs an adopt moves.
// A run is moved when its task or beam project is not done and holds a batten run record that is not done,
// and it is a child of an adopted beam, or of root when root is adopted.
// Each adopted beam's own pawl run is moved too.
// A beam below an adopted beam or root is listed and keeps its own owner.
//
// # The caller and the worker
//
// `runAdopt` takes these branches in order:
//
//  1. `--discard` stands alone and runs `discard`, which prints whether it removed a record.
//  2. `--here` resolves the call's request through `hereRequest`, described under the here call below,
//     or ends the call there.
//  3. It sets `NoOrch` when the caller's strand name, `agentname.StrandNameEnv`, parses as an orch role.
//     A numbered orch counts, and an empty or unparsable name sets nothing.
//  4. It validates the request, reads the record and judges it with `judgeStanding`:
//     a running worker of the same subjects is followed and nothing is spawned;
//     an unfinished record of other subjects is refused, naming the recorded subjects, the step, the re-run and `--discard`;
//     any other case spawns a worker.
//  5. It follows the record through `follow` and prints the end:
//     the record's envelope for a `done` adopt, and an error envelope with the stop's way forward for a `stopped` one.
//
// `spawnWorker` resolves the binary through `selfexec.LymBinary`.
// It runs `workerArgs` as a detached process in the prime.
// The worker's stdout and stderr are appended to `adopt.out` beside the record.
// So the envelope of a worker that lost the lock, and a panic, stay on disk.
// The spawn is logged, the starter waits on the worker in a goroutine,
// and the worker is registered under `runproc.HubOwner` with the role `runproc.RoleAdopt`.
// The worker outlives the call: `proc.Detach` gives it a session of its own.
// A test binary starts nothing.
//
// `follow` reads the record on each wake and prints `adopt: <step> done` on stderr for each step that ended.
// It returns the record of the followed worker once that reads `done` or `stopped`.
// An `interrupted` record, whose worker is gone, is an error naming the re-run and `lym adopt --discard`.
// When the spawned worker ended and the record names another pid, that worker lost `adopt.lock`.
// The follow then takes the other record when it started after the caller did and names the same subjects.
// Other subjects are refused, and a record not started later is not followed.
// When the call followed a worker that already ran, there is no spawned worker to watch.
// A record that names another pid, or none, then ends the follow with an error.
// The error names `lym beam status` and the re-run.
// An exited worker that left no record is read once more at the next wake, and then ends the follow with an error.
// That error names the re-run, `lym beam status` and `adopt.out`.
// The wake starts at `followWakeFloor` and doubles to `followWakeCeiling` while the record is unchanged.
// It returns to the floor on a change.
// A plain shell follows until the worker ends.
// An agent session's follow is capped by `shuttleengines.KeepWarm` for its role, through `followCap`.
// At the cap it returns `still running`, naming `lym beam status` and the re-run that follows again.
//
// The hidden `lym adopt worker [<subject>...]` has audience internal.
// It takes `--all`, `--keep-local`, `--drop-local` and `--no-orch`.
// It is one more entry into the steps, callable from any shell.
// The same validation, the subject match and the lock bound it.
// `runWorker` is the whole worker.
// It prints the end envelope: ok for `done`, an error envelope for a stop or a refusal.
//
// # The here call
//
// `lym adopt --here` adopts the subject of the worktree it runs in:
// `root` in the prime, the beam in an open beam project.
// In any other worktree it ends ok with `outcome` `no_subject` and does nothing else.
// A subject, `root` or `--all` beside it is refused.
//
// It first reads the adopt record.
// An unfinished record takes the worktree up when its `check` step is not done.
// Its subjects must also list the subject the worktree reads its owner from, through `adoptrecord.Record.TakesUp`.
// That is the worktree's beam when the record lists it or it records an owner of its own.
// Otherwise it is the subject above it through `hubgeom.ParentOf`, `root` at the top.
// The call then runs the record's request with its own flags merged in by `mergeRequest`,
// so it follows a live worker or resumes the record, whoever the owner reads as.
// A record past `check`, such as one that stopped on a held-back project, takes up no worktree.
//
// With no record taking the worktree up, it pulls Bolt through `fabricengine.Bolt.Pull` and reads the subject's owner.
// A pull that cannot bring Bolt up to date, an unreachable origin among them, reads the local Bolt,
// and its detail is a line of `warnings`.
// When no other machine owns the subject, the call ends ok with `outcome` `owned_here` and `subject`.
// It spawns no worker and writes no record.
// Otherwise it runs as `lym adopt <subject>`.
// The `lym s` shortcut runs `lym adopt --here --no-orch` before `lym beam start`, which starts the orch.
// So adopt starts from a plain shell on the machine taking over, and an adopt that stops starts nothing.
//
// Bound: every `--here` with no record taking its worktree up pays one Bolt pull.
// A stale local Bolt that reads this machine as the owner skips the adopt;
// the next Bolt pull that reaches origin hands the subject to the old owner's path.
//
// # The archives list
//
// `lym adopt --archives <subject>...` lists the `archive-owner/` tags the old owner of each subject pushed.
// It lists only those whose commit the slug's branch on origin does not contain.
// It reads them through `fabricengine.Topology.ReadOwnerArchives`.
// It reads each repository's tags against that repository's branch of the slug:
// the code branch for a code tag, the records branch for a records tag.
// A tag's slug belongs to the subject `boardengine.Owners.AdoptSubject` names for it;
// a slug the board no longer holds belongs to the subject of its parent.
// The envelope's `archives` lists the tags per slug with its subject.
// Each tag carries its side, commit and branch, and a `git -C <dir> log <tag>` that shows it.
// Teardown and `--drop-local` archives carry the `archive/` prefix, so it lists none of them.
// It runs no step, reads and writes no adopt record and brings nothing in;
// it fetches each listed tag as a local tag, so its log shows.
// A subject that is neither `root` nor a beam project is refused,
// and the refusal of a task or grouping project names the subject that moves it.
// Any other flag beside it is refused.
//
// # The record
//
// `adoptrecord` holds the adopt record, `adopt.json`, and its lock, `adopt.lock`.
// Both sit directly under `fabricengine.AdoptScratchDir`, beside `adopt.out` and the per-subject directories.
// `ReadCheckViews` and `primeMark` skip non-directories, so those files change no check view.
// The worker alone writes the record, atomically.
// A read takes no lock and creates nothing.
// A record holds the request, the resolved subjects, and the worker's pid and start ticks.
// It also holds the steps that ended with their parts and the step in flight.
// Last it holds the inputs the later steps need, the state and the end envelope.
// The state is `running`, `stopped` or `done`.
// A `running` record whose worker is gone reads `interrupted`, through `Record.ReadState` over `adoptrecord.WorkerAlive`.
//
// `runWorker` takes `adopt.lock` without blocking.
// A held lock returns at once, naming `lym beam status` and writing nothing.
// Holding the lock, it reads the record and judges it:
//
//   - No record, or a `done` one, is a fresh adopt.
//     The worker writes the record `running` at `reconcile` before any step runs.
//     The record holds the request and the resolved subjects.
//   - An unfinished record of the same subjects whose worker reads alive is a transient refusal naming `lym beam status`
//     and `lym adopt --discard`, since this worker holds the lock while that one is recorded as running.
//   - An unfinished record of the same subjects whose worker is gone or that stopped is resumed.
//     `mergeRequest` keeps the record's flags and adds the call's.
//     A call's `--keep-local` or `--drop-local` replaces the record's other one and restarts the adopt at `reconcile`.
//     The worker rewrites only its pid, start ticks, start and state, and runs from `Record.ResumeStep`.
//     It takes the subjects from the record, so a beam made since does not join an `--all` adopt.
//     It keeps the steps listed, each replaced as it runs again, unless the call restarted the adopt.
//   - An unfinished record of other subjects is refused, writing nothing.
//
// Steps up to `owner` run in one process.
// So a resume whose first step not done is `owner` or earlier restarts at `reconcile`.
// Only the inputs of the steps from `deploy` on persist in the record, with the slugs the pause step paused.
// Every step is safe to run again.
// A re-run adds only the pairs that are missing, pauses only the slugs not listed, and writes the same owners.
// It judges the deploy again, leaves a live pair to the beam steps, and starts no second live check.
// An anchor change that stands in a worktree the steps act under stops the record at the step it would run.
//
// `lym adopt --discard` takes `adopt.lock` without blocking and removes an unfinished record or one that does not decode.
// It leaves a done record, or none, and refuses naming `lym beam status` while a worker holds the lock.
// It lifts no pause and undoes no owner write.
// `lym beam status` shows an unfinished record as its `adopt` section.
//
// # The steps
//
// The worker runs `adoptrecord.Steps` in order: reconcile, pairs, pause, owner, deploy, beam, check and done.
// Each step writes the part of the outcome it set.
// The record stores that part with the step when it ends.
// A step that fails stores the part it gathered before the error.
// The adopt pause is set before the owner write, and the owner write comes before the deploy.
// So an adopted run is paused, never running, from the moment its owner changes.
//
// # Step reconcile
//
// A moving hub-scope reconcile runs first.
// Its `pairteardown` seams take a `HubPaused` from `hubOnlyPaused`, which covers a slug only by a live hub entry.
// So a pair whose strands are alive under any slug-scope pause, an owner pause included, reads live.
// An unreachable origin stops the record naming a re-run once origin answers.
// A reconcile error stops it naming the error and the re-run, and the envelope keeps the report and its mutation record.
// A stop on Bolt or the prime's records branch stops it with the report and that branch's way forward.
// Nothing below runs after a stop.
//
// An adopted beam or moved run that the report reads owned_elsewhere stops in either case:
// a branch of it was judged `needs_replay`, `ahead`, `rewound`, `rewritten`, `diverged` or `anchor_stopped`;
// adopt's own `LivePair` call, made for every such slug with a local pair, reads the pair live.
// The slug's envelope entry names each branch and its state, and the two ways forward:
// `lym adopt <project> --keep-local` and `lym adopt <project> --drop-local`.
// `<project>` is the adopted beam or `root` that moves the slug, never a task.
// `wouldStop` answers that from the read-only part of this step, before any slug is touched.
// A project one of whose entries stops is held back whole:
// no owner is written for it and no other entry under it is taken over.
// The other projects go on.
// The moved runs are read once more after the reconcile, which may bring the run records of the other machine.
//
// A beam that is a member of an adopted beam, or of root, and records another machine as its owner keeps that owner.
// It is reported on its own outcome as `owned elsewhere` with `owned_by`, and it is neither taken over nor paused.
//
// # Step pairs
//
// Adopt adds a pair through `Topology.Add` from the prime.
// A pair is added for each adopted beam or moved run that holds a run status or is an owned beam.
// Only a slug this machine has no pair for gets one.
// `Add` adopts the live branches and their origin record, the beam mark included.
// A refusal stops that slug, and the other slugs go on.
// The refusal carries fabric's own message and way forward, except in one case.
// That case is `*fabricengine.ErrBranchExists`, the leftover local branch's.
// Dropping that branch would lose this machine's unpushed commits with no archive, so adopt rewords the refusal.
// The reworded refusal names tagging the leftover tip under `archive/<slug>/<tip>`.
// It then names `git branch -D <branch>` and a re-run of the adopt that moved the slug.
//
// `--keep-local` records the owner over local commits origin lacks and over a live pair.
// `--drop-local` refuses a live pair naming `--keep-local`.
// Otherwise it calls `fabricengine.DropLocal`, which records into adopt's mutation record.
// `DropLocal` keeps each local tip origin lacks under an `archive/<slug>/<tip>` tag before it moves the branch.
//
// The step decides which beams are adoptable and whether the prime goes, and stores them as inputs.
//
// # Step pause
//
// The step reads the owners again through `ReadOwners`.
// The owners the reconcile left in Bolt decide the pause.
// `pauseTargets` returns, sorted, the slugs to pause.
// A moved run is paused when it went through `takeOverSlug` with no stop, error or warning,
// and its effective owner in those owners is another machine,
// and the write of this call will make it this machine's:
// its project is a beam being written, or `root` while the prime is written.
// An adopted beam's own pawl run also needs its parent owned here or written.
// A run whose pair `LivePair` reads live here is paused too.
// A run that will still read owned elsewhere, such as a nested beam keeping its own owner, is not.
// A task entry under a subject being written is paused too, though no run exists yet:
// it is not done, has an empty status and is not a front task.
// Then one `pauserecord.Update` through `UpdatePause` applies `pauseEntries`:
// a slug whose own scope holds an `OwnerCaller` entry has that entry turned into an `AdoptCaller` entry in place,
// and the entry keeps its pause time;
// a slug already holding an `AdoptCaller` entry keeps it;
// a slug holding an entry of any other caller, a legacy `HoldCaller` entry included, keeps it and is not paused;
// any other slug gets a new `AdoptCaller` entry at now.
// A slug the record lists as paused is skipped, so a slug an operator resumed stays resumed.
// The envelope still reports the pause of every listed slug, since a re-run replaces the part of the earlier run.
// The pause is written straight through the record, never through `tenderengine.Pause`.
// So it waits no grace and sends no Esc.
// `pauserecord.Coverage.Held` reads an `AdoptCaller` entry as it reads a `HoldCaller` one,
// so the dobby treats an adopt pause as a hold.
// Only an operator lifts it, by `lym tender resume` or `lym batten cancel`.
// Neither the adopt check, the dobby's owner lift nor a hibernate wake lifts it.
// A failure to pause stops the record before the owner write, naming the re-run.
// A paused slug's outcome reads `run` `paused by adopt` with `adopt_paused` true.
// The outcome's `pause_standing` line names `lym tender resume <slug>`.
//
// # Step owner
//
// One `boardengine.SetOwners` call with `Takeover` records the owners.
// It records this machine as the owner of every adopted beam the earlier steps did not stop.
// It records the prime too when `root` is adopted or when the board records no prime owner.
// A stop that holds root back keeps the prime unrecorded.
// A refused push stops the record naming its class's way forward and then the re-run of the same `lym adopt`.
// The pause stays, so the re-run finds it in place.
// A beam the re-read shows owned by another machine stops, and its pause stays.
// Its outcome names that owner and says a re-run of `lym adopt <beam>` takes it over again.
//
// After the write the owners are read back, and each adopted beam and moved run is settled over them.
// An entry that reads owned by another machine is left as it stands.
// It is reported as `owned elsewhere` with its `owned_by`, never as an error.
// A slug still covered by a pause of another caller gets a line naming that caller and the way to lift it:
// `lym tender resume <slug>` for an entry of its own scope, or `lym tender resume` for a hub entry.
// A beam with no run record reads `none`.
// Adopt lifts no pause and starts no run.
//
// Bound: until the same machine owns a beam and its parent, the beam's pawl run gets no pause and no pass here.
// It is reported owned elsewhere.
// `lym adopt <beam>` on the parent's owner, or `lym adopt <parent>` on the beam's owner, takes it over.
//
// The step ends with the warnings for the branches the reconcile left unmoved.
// `reconcileWarnings` returns a warning line for a branch the reconcile report holds when all of these hold:
// its outcome reads `Prime`, or its `Slug` is an adopted beam;
// its state is `rewritten` or `diverged`, or `ahead` with `Moved` unset;
// no stop of that slug already reported it, and the shared-stop refusal did not judge it.
// The line names the branch and its state.
// It also names `lym fabric pull` in the branch's worktree and the re-run of `lym adopt` with the same arguments.
// The worktree is the `CodeWorktree` for a records branch, where its fabric verbs run;
// for a code branch it is the `Worktree`.
// The reconcile's own way forward follows in parentheses when it set one.
// A records branch the reconcile replayed reads `ahead` with `Moved` set, so it gets no warning.
// Adopt goes on, carries the lines as the envelope's `warnings` and runs no `lym fabric pull`.
//
// # Step deploy
//
// The reconcile may move the prime past the commit the running lym was built from,
// and a run let go under that lym fails on code it does not know, such as a stencil marker it never fills.
// So when the owner write recorded the prime, `deployStale` runs.
// `readDeployState` reads the hub's `landing` config.
// Nothing more is read and nothing runs when `deploy` is empty or `deploy_branches` omits the prime's branch.
// That is the same rule as at the Land row.
// Otherwise it reads the VCS stamp of the running lym through `buildvcs.OfFile`, and the prime's head.
// `installedBehind` reads a build as behind when its revision is a proper ancestor of the prime's head.
// A build of a commit the prime lacks, as of unpushed local work, is not behind.
// A behind build runs the `deploy` command in the prime through `battencli.RunDeploy`, under `battenshed.DeployTimeout`.
// The command's output goes to `deploy.log` in root's adopt scratch directory.
// The envelope's `deploy` names the build it replaced, the prime's head and the log.
// A deploy that fails, times out or cannot start stops the record at `deploy`, naming the log and the re-run.
// The owner stays written and the adopted runs stay paused, so no run goes on under the old lym.
// The re-run resumes at `deploy` and judges the build again.
// A state that does not read, as of a lym with no VCS stamp, is a line in `warnings` naming the hand deploy.
// The adopt goes on.
// The deploy runs before the beam steps and the adopt checks.
// So the dobby, the orch and the check this call starts run the new lym.
// Bound: the deploy takes no land lock, so it can run beside a landing's own deploy.
// The deploy command is expected to install by rename, as the hub's own does.
//
// # Step beam
//
// `BeamUp` runs the beam steps for the prime when its owner was recorded.
// It then runs them for each adopted beam whose owner write stood.
// The steps start the beam's dobby, its beam window and its orch.
// So the pair, the dobby and the orch move with the project.
// `--no-orch` starts the dobby and leaves the orch down.
// A call from an orch session passes it by default, since that orch is the session the operator is already in.
// `BeamUp` calls `orchcli.BeamUp` with `Prepare` false, since the first step already reconciled origin.
// A beam whose pawl run holds the landing hold gets no beam steps, and its owner is still written.
// `LandingHeld` reads that hold over `boardengine.RunHolds`, since `ReadOwners` carries no run status.
// The beam's `way_forward` names `lym batten status <slug>`.
// When this machine does not also own the beam's parent, it names `lym adopt <parent subject>` too.
// For the prime that is `lym adopt root`.
// A nil `LandingHeld` reads no hold, and a failed read is the slug's warning naming `lym beam start <slug>`.
// A failed `BeamUp` is the slug's own warning naming `lym beam start <slug>`.
// For the prime it is `prime_beam_warning`, naming `lym beam start root`.
// The other worktrees go on.
// What each step did is the slug's `beam_steps` and the envelope's `prime_beam_steps`.
//
// # Step check
//
// Machines differ in OS and toolchain, so code landed on one can break on another.
// After the beam steps, the step starts one hidden `lym adopt check <subject>` per subject, detached.
// The order is `root` first when the prime's owner was recorded, then each adopted beam whose owner write stood.
// Before each start it writes the subject's `fix-tries.json` count to 0 through `ResetTries`.
// A hand adopt resets the try count, and a check started otherwise does not.
// The spawn is logged through `logger`.
// `startCheckWithTiers` first calls `resolve`, the subject's worktree resolve.
// For a worktree that does not resolve it spawns nothing and returns an error.
// The error names `lym beam start <subject>` and the re-run of `lym adopt <subject>`.
// `ensureCheck` spawns nothing when `check.json` reads `running` and `checkAlive` finds the process alive.
// The check's stdout and stderr go to `check.out`, created for append, and never to `suite.log`.
// So the gate's output alone reaches `suite.log`.
// A losing check's envelope goes to `check.out`, and a panic of the check is on disk.
// A tier the gate's green record holds green for a worktree's tree is carried and runs nothing.
// `checkAlive` reads a pid as alive when it is alive and its start ticks equal the record's.
// It reads a pid with other ticks, as after a reboot that reused it, as dead.
// It reads a record without ticks, or a platform where the start time is unreadable, as alive on the pid alone.
// Each item of the envelope's `checks` holds the `subject`, the check's pid, `suite.log` and `check.json`.
// The tiers are those of the hub's `adopt` gate key as the gate config reads them before the check's own reconcile.
// They are none when the config does not read.
// `already_running` marks a check found running.
// The envelope's `message` is one line per check.
// Each line names the subject, whether the check started or was already running, and those tiers.
// It also says that the prime's reed status bar shows the check's progress and result.
// A spawn that fails is one of the envelope's `check_warnings`.
// The warning names its subject, the error and the re-run of `lym adopt <subject>`.
// It changes no slug's outcome and stops no other subject's check.
// The step ends by building the per-run report over the runs the takeover moved and paused.
// Bound: between a spawn and the child's lock, `lym beam status` can show the previous record,
// and a second `lym adopt` can spawn a check that loses the lock and exits.
// So either envelope's pid can name the check that lost, both naming the same log and record.
//
// # Step done
//
// The step stops the record when a local-commit or live-pair stop held a project back.
// The stop names the re-run that records the owner over the local state, `--keep-local`,
// and the one that keeps each local tip under an archive tag and moves the branches to origin's, `--drop-local`.
// Both are rendered by `adoptrecord.Request.Command` over the adopt's own request.
// Only the local-commit flag differs, so the standing record is resumed under either.
// An adopt that held nothing back ends `done`.
//
// # The check verb
//
// The hidden `lym adopt check <subject>` has audience internal and takes exactly one argument, a beam slug or `root`.
// It runs the hub's `adopt` gate in-process in the subject's worktree through `gatecli.RunGate`,
// and it reports each tier and its end to that worktree's nearest live orch.
// `hubCheckDeps` resolves the subject's worktree: the prime for `root`, the beam's pair for a beam.
// The pair resolves through `lymcwd.ResolveWorktree`.
// An unresolved worktree returns an error naming `lym beam start <subject>`.
// The verb resolves the prime from the working directory as the adopt does, so it runs from anywhere in the hub.
// It prints an ok envelope with the record's `state`.
// The envelope has `already_running: true` when another check of the subject holds the lock.
// `adopt` itself takes arbitrary arguments, so a standalone `RunCLI` still reaches `adopt <subject>`;
// a beam named `check` cannot be named to `lym adopt` and is adopted through `--all`.
//
// Before it takes the lock, `runCheck` reads the subject's owner through `checkDeps.OwnedElsewhere`.
// `hubCheckDeps` fills it from `boardengine.ReadOwners`:
// the beam's effective owner, or the prime owner for `root`, against this machine's name.
// A subject another machine owns posts this text and returns it as its error, writing no record:
// `adopt check of <subject>: <subject> is owned by <owner>, so no check ran; way forward: run lym adopt <subject> on this machine`.
// A hand `lym adopt check` on a machine that does not own the subject reaches that case.
// A read error is returned likewise, writing nothing.
//
// Each subject's files sit under `<fabricengine.AdoptScratchDir>/<subject>/` and are named only here:
// `check.json`, the progress record, written through `internal/state` under its own state lock;
// `check.lock`, the verb's own lock;
// `check.out`, which the spawn of the check points its stdout and stderr at;
// `suite.log`, the gate's log;
// `fix-tries.json`, the count of fix tasks the subject's check has filed.
// The last is read and rewritten through `internal/state` under its own lock.
// It is kept apart from the check record, so no write of the record touches it.
// `CheckRecord` holds the check's subject, pid and start ticks, its start and the gate key.
// It holds each `CheckTier` as it ended, with the failed tests the gate's counter named, and the tier in flight.
// It holds its state (`running`, `green` or `failed`), its end time, the first failed tier and an error.
// A record with an unknown key still reads, so a newer writer's record never fails an older reader.
// `ReadCheck` stats the record first and reports false when it is absent, taking no lock and creating no directory.
// A record that does not decode reads as absent with one Warn, so a damaged file never blocks the next check.
//
// `runCheck` takes `check.lock` with a non-blocking acquire.
// When the lock is held, it returns at once, having written and posted nothing.
// Holding the lock, it truncates `suite.log` and writes a running record.
// It then reconciles the hub through `hubreconcile.Ensure`.
// A hub whose build stamp is stale thereby seeds `adopt` from its tuned `finalize` before the gate reads it.
// A failing reconcile ends the check failed with the way forward and runs no tier.
// So no check goes green on the template's tiers.
// The gate's observer rewrites the record as each tier starts and ends.
// It posts `adopt check of <subject>: tier<N> <green|failed>, passed <p> failed <f> skipped <s>, <wall>s`.
// Every notice opens `adopt check of <subject>:`, so the orch tells two subjects' checks apart.
// A failed tier's notice adds one line per failed test, `<package> <test>: <first line>`.
// It lists at most `maxListedFailures` of them and then `and <n> more`, and ends with a line naming `suite.log`.
// A green gate ends the record green and posts the tiers and `suite.log`.
// Any other exit ends it failed.
// The record names the first failed tier, or the gate's exit code as the error when no tier ended.
// A failed tier posts the failed tier and `suite.log`, and the lines of the fix loop follow it.
// A reconcile failure and a gate that exits before any tier ended are no verdict.
// They start no fix loop and name no worktree.
// They name the fix of what stopped them and the re-run `lym adopt <subject>`.
// A tier that refuses after it started returns without ending, so the end clears the tier in flight.
// `runCheck` also sets the check mark on the prime's reed session through `reedengine.Engine.SetCheckMark`.
// It does so at its start, as each tier starts and ends, and at its end.
// `checkMarkOf` builds the mark from the record, so the mark names its subject.
// At the start it reads `adopt check of <subject>: starting`.
// Later it shows the done tiers with their outcomes and wall seconds, then the tier in flight timed from its start.
// At the end it reads `adopt check of <subject> green` or `adopt check of <subject> failed` with every tier it ran.
// A failure before any tier ended reads as a failure naming `lym beam status`.
// `primeMark` shows the mark joined, by ` | `, with the mark of each other running subject.
// The other marks follow the subject's own, sorted by subject.
// The state is running while any joined record runs.
// A post or mark that fails is logged and changes nothing.
// Bound: a check killed before its end leaves its last running mark, with its seconds still growing.
// The mark stays until the next check sets one or the prime's session ends.
//
// Bound: the gate slots do not serialize two checks; the lock does.
// So one check of a subject at a time writes its record, its log and its posts.
//
// # The green end
//
// Once the gate exits 0 and the green record is written, `runCheck` lets go of `check.lock`.
// It then calls `checkDeps.Green` once and posts each line it returns as one notice after the green notice.
// A failed end, an end before any tier and a subject owned by another machine never call it.
// `hubCheckDeps` sets `Green` to `greenEnd` over `hubGreenDeps`.
// A build error of the deps is the one line it returns.
// A green check only reports.
// `greenEnd` lifts no pause, brings no dobby or orch up and starts no run.
// It takes two steps, each returning a line that names its way forward when it fails, and the other step goes on:
//
//  1. It removes the subject's fix task, which `fronttask.FrontOf` finds over the subject's project.
//     The task is removed through `fronttask.Remove` in any status but done, and its cancel step ends a running one.
//     A failure carries the step's error and the hand removal.
//  2. It writes the subject's `fix-tries.json` count to 0.
//
// An adopt pause stands until an operator lifts it with `lym tender resume`, or with `lym batten cancel`.
// The run report names that command for each paused run.
//
// # The fix loop
//
// When the gate exits non-zero with a failed tier recorded, `runCheck` writes the failed record.
// It then lets go of `check.lock` and calls `checkDeps.Failed` once.
// It posts each line `Failed` returns after the failed notice.
// `hubCheckDeps` sets `Failed` to `failedEnd` over `hubFixDeps`.
// A build error of the deps is the one line it returns, naming its cause and the re-run of `lym adopt <subject>`.
// The pause stands meanwhile.
// `fixDeps` holds the try limit, which is `adopt_fix_tries` of the subject worktree's `beam.yaml`.
// It also holds the board's vocabulary, read through `boardengine.LoadConfig`.
// `failedEnd` takes the first case that applies, each returning the lines of its notice:
//
//  1. The subject's own fix task stands open and healthy.
//     `fronttask.FrontOf` finds it over the subject's project and the `adopt-check` action.
//     It reads open and healthy in one of these states:
//     released with the empty status;
//     a run status with no readable batten run record (starting);
//     a run record `running` or `paused`;
//     a run record `awaiting` with an admitted escalation.
//     Nothing is filed or removed.
//     The notice names the task, its state, and that its landing starts the next check.
//     The hand steps are `lym batten run <slug>` from the prime for a paused one.
//     To drop the task they are `lym batten cancel <slug> --remove` then `lym board remove '{"slug":"<slug>"}'`.
//  2. Another open front task stands in the subject's worktree.
//     `fronttask.OpenFront` finds it over the board without the own fix task.
//     Nothing is filed or removed.
//     The notice names it and the re-run of `lym adopt <subject>` once it lands.
//  3. The try count in `fix-tries.json` is at the limit.
//     Nothing is filed or removed.
//     The notice names the failing tiers and tests, `suite.log`, and a halted own fix task when one stands.
//     It also names the re-run of `lym adopt <subject>`, whose check removes that task.
//  4. The own fix task is a draft or halted.
//     `fixRunHalted` reads the run record as halted in state `blocked` or `failed`.
//     It also reads `awaiting` as halted when no escalation is admitted.
//     The task is removed through `fronttask.Remove` right before the filing.
//     A failed removal files nothing, and its line carries the step's error and hand removal.
//     The line adds the re-run only when the entry's removal failed.
//  5. A task is filed through `fronttask.File`.
//     Its slug is the one `nextFixSlug` gives, and its kind is task.
//     Its title is `Fix the adopt check of <subject> on <machine>`, and its body is what `fixBody` renders.
//     Its labels are `bug` and, when the board's vocabulary knows it, `gate`.
//     Its recipe is `darn` and its profile `light`.
//     Its project is the beam's, none for `root`, and its `on_land` is `adopt-check`.
//     The try count is written one higher between the entry's creation and its release,
//     so a filing that dies between the writes still counts and one the create refused does not.
//     A filing error files nothing more and names the re-run of `lym adopt <subject>`.
//     That re-run resets the count and checks again.
//
// `failedEnd` never starts a pass of the task: its parent's dobby starts it from the empty status.
// `fixSlugPrefix` is `adopt-fix-<subject>` when that and `-t999` fit `boardengine.MaxSlugLength`;
// otherwise it is `adopt-fix-`, the subject cut short, a hyphen and the first six hex digits of the subject's SHA-256.
// `nextFixSlug` returns `<prefix>-t<N>`, where N is one more than the highest N already in use.
// It looks at the board's slugs, the batten run directories and the prefix's task branches.
// `fabricengine.TaskBranchSlugs` lists those branches.
// So a filing meets neither a landed slug nor a leftover branch.
// `fixBody` holds the failing tiers and the failed tests the gate named.
// It holds an excerpt of `suite.log` of at most `fixExcerptLines` lines around each failed test's first line.
// It holds the instruction to diagnose first, and the stop condition for a machine failure.
// That condition names `lym adopt <subject>` as the way forward and never `lym loom resume`.
//
// Bound: at most `adopt_fix_tries` fix tasks per subject between two hand adopts, and at most one at a time.
//
// # The check view
//
// `ReadCheckViews` reads the `check.json` of each subject directory under `fabricengine.AdoptScratchDir`.
// It reads through `ReadCheck`, which on an absent record reports false and creates no directory or lock file.
// It returns the views sorted by subject, none when no directory holds a record.
// `lym beam status` shows them as the `adopt_checks` section, one item per subject.
// `checkView` builds the `CheckView` from the record.
// A running record shows the tier in flight and the tiers done.
// A green or failed record shows its end time, and a failed one also its failing tier.
// Every view names its subject, and its lines open `adopt check of <subject>:` and name `suite.log`.
// A running record shows `ended without a verdict` when its pid is not alive.
// It shows the same when the pid is alive with start ticks that differ from the record's.
// Differing ticks arise after a reboot that reused the pid.
// The view names `suite.log`, `check.out` and a re-run of `lym adopt <subject>`.
// The record clears at the next `lym adopt` with an owner write, which starts a fresh check;
// the re-run writes no record.
//
// # The envelope
//
// The worker's end envelope is merged from the steps' parts by `envelopeOf`, not gathered in one process.
// It takes the part of every step the record lists as done, an earlier process's steps included, in step order.
// It then takes the part a stopped step gathered before its error, without listing that step as done.
// A scalar field of a later part replaces an earlier one.
// Slugs merge per slug and per field, and list fields concatenate.
// So the envelope of a stopped adopt keeps the reconcile report, the mutation record and the pauses already set.
// The record stores the envelope at `done` and at a stop, and the caller prints it.
//
// The envelope carries `reconcile`, `mutations` and `partial`.
// The `reconcile` object holds no mutations of its own: the top-level `mutations` holds the whole record.
// `deploy` reports a deploy the call ran.
// `warnings` covers branches the reconcile left unmoved and a deploy state that did not read.
// `prime_beam_steps` and `prime_beam_warning` report the prime's beam steps.
// `checks` and `message`, or `check_warnings`, report the adopt checks.
// `archives` names `lym adopt --archives` over the subjects whose owner write stood.
// The old owner archives its unpushed tips only once it reads that write.
// Per slug it carries what happened: a stop with its branches, a pair added and an owner recorded.
// It carries the pause as `adopt_paused` and `pause_standing`.
// It carries the run as paused by adopt, none, running here or owned elsewhere by `owned_by`.
// It carries the `beam_steps` run and the error.
// It carries a warning when this machine does not own the slug's parent, since that run halts at Finalize here.
// The warning names `lym adopt <project of the parent>`, or `lym adopt root` for the prime.
//
// `runs` is always present, an empty array for none, beside the per-slug outcomes.
// It holds one `runReport` for each run under the named subjects, sorted by slug.
// A `runReport` holds its `slug` and `state`, and a plain `line`.
// Its `row` is the batten run record's current producer, omitted for an entry with no record.
// It holds `owned_by` when owned elsewhere and `caller` for `paused`.
// The runs are the moved runs before and after the reconcile.
// They also include the slugs the adopt paused, released tasks included, and each subject's open front task.
// `runReports` gives each the first state that applies, in this order:
//
//   - `owned_elsewhere`: its entry's effective owner after the owner write is another machine.
//   - `landed`: it was a moved run before the reconcile, and its board entry is absent or done after it.
//   - `paused_by_adopt`: a live pause entry of its own scope carries the adopt caller or the legacy hold caller.
//     A paused released task has no row.
//     The line names `lym tender resume <slug>` and, for a run blocked at Finalize's owner check, `lym loom resume`.
//   - `paused`: a live entry of its own scope of another caller stands, naming that caller.
//   - `halted`: its run record reads halted through `fixRunHalted`.
//     The escalation is read through the `EscalationEligible` seam.
//     The line names the halt reason, and for the subject's fix task the re-run of `lym adopt <subject>`.
//   - `ended`: it was a moved run before the reconcile, and its run record is gone.
//     Its entry after the reconcile reads a status that is neither empty, a run status nor done,
//     such as the draft a cancel on the other machine writes.
//   - `running_here`: any other run this machine owns, the fix task included.
//     An unstarted fix task has no row.
//
// A read that fails leaves `runs` empty and adds a line to `warnings` naming the re-run of the same `lym adopt`;
// the steps already taken stand.
//
// # The bounds
//
// Adopt writes owners without the old owner's help.
// It stops no driver live elsewhere; such a driver pauses at its next owner check.
// Under a hub pause `LivePair` reads not live,
// so `--drop-local` can move a branch under interrupted strands, as the reconcile of any hub-paused pair does.
// A slug whose pair has no batten run record never reads owned_elsewhere.
// The reconcile judges only recorded pairs, so such a slug's local commits do not stop it before the owner write.
// Whether origin holds an entry's branches is read from the board, not from origin.
// A run status, or an owner on a beam, marks an entry that was started, and the add then adopts its live pair.
// A worker killed with `lym fabric anchor`'s `runproc.EndSparing` leaves its record `running` with a dead pid.
// That reads `interrupted`, and a re-run resumes it.
package adoptcli
