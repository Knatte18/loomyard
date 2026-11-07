// Package adoptcli is the adopt module for the verbs `lym adopt <beam>...`, `lym adopt root` and `lym adopt --all`.
// They take the flags `--keep-local` and `--drop-local`.
// It takes over beam projects, or root, and the runs under them from the other machine of the operator.
// It calls each owning module's takeover function in-process, so no module learns another's state,
// and it has no engine package of its own.
//
// # The verb
//
// The verb runs from the hub's prime and from any beam, and acts on the prime's location in both;
// `battencli.ResolveBattenPrime` resolves the prime.
// `request` holds the slugs, `Root`, `All`, `KeepLocal` and `DropLocal`;
// the word `root` among the slugs sets `Root`.
// `adoptDeps` holds every step's seam, which `hubDeps` fills over the hub and `TestAdopt` fakes.
// An entry whose slug is `root` is shadowed by that word and cannot be named.
//
// The arguments are beam projects (a project entry whose `beam` is true) and `root`, which names the prime.
// `--all` names `root` and every beam project that is not done.
// Only a beam project and the prime record an owner, so an adopt moves the named projects alone:
// each one's owner (the prime owner for `root`), its pair, and the runs whose parent is its worktree.
// A nested beam keeps its own owner.
// `lym adopt root` moves root's tasks and the tasks of the grouping projects below root.
// It never moves a beam's members, whose parent is that beam.
//
// Before anything runs, `namedEntries` reads the local Bolt and refuses what cannot be adopted.
// An unknown slug is refused naming `lym board list`, and a note naming `lym board promote`.
// A done entry is refused naming that it already landed.
// To let a run that landed there go on to Land under its owner pause, the refusal names `lym tender resume <slug>`.
// A task is refused naming `lym adopt <beam>` or `lym adopt root`, the `AdoptSubject` that moves it.
// A grouping project is refused naming the same adopt, which moves its tasks.
// The refusal also names `lym beam create <slug>`, which makes it a beam.
// `--keep-local` with `--drop-local` is refused, and so is no argument without `--all`.
//
// `movedRuns` lists, sorted, the runs an adopt moves.
// A run is moved when its task or beam project is not done and holds a batten run record that is not done,
// and it is a child of an adopted beam, or of root when root is adopted.
// Each adopted beam's own pawl run is moved too.
// A beam below an adopted beam or root is listed and keeps its own owner.
//
// Adopt then asks `adoptDeps.MoveInProgress` whether an anchor change stands.
// `hubDeps` fills it from `fabricengine.MoveInProgress`.
// It asks for the prime.
// It also asks for each adopted beam and moved run this machine holds a pair for.
// For each of those it asks at its own worktree and at its parent's.
// When an anchor change stands in any of them, adopt refuses with an empty outcome.
// The refusal carries the record's way forward, followed by a re-run of the same `lym adopt`:
// a change in progress to wait for, or `lym fabric anchor <worktree> --set <anchor>` to finish an interrupted one.
// No pair is added, no run held and no gate suite started under a moving anchor.
// A nil `MoveInProgress` reads as no record standing.
// The moved runs are read once more after the reconcile, which may bring the run records of the other machine.
//
// # Step 1: the origin reconcile
//
// A moving hub-scope reconcile runs first.
// Its `pairteardown` seams take a `HubPaused` from `hubOnlyPaused`, which covers a slug only by a live hub entry.
// So a pair whose strands are alive under any slug-scope pause, an owner pause included, reads live.
// An unreachable origin refuses naming a re-run once origin answers.
// A reconcile error refuses naming the error and the re-run.
// A stop on Bolt or the prime's records branch refuses with the report and that branch's way forward.
// Nothing below runs after a refusal.
//
// An adopted beam or moved run that the report reads owned_elsewhere stops before its owner write in either case:
// a branch of it was judged `needs_replay`, `ahead`, `rewound`, `rewritten`, `diverged` or `anchor_stopped`;
// adopt's own `LivePair` call, made for every such slug with a local pair, reads the pair live.
// The slug's envelope entry names each branch and its state, and the two ways forward:
// `lym adopt <project> --keep-local` and `lym adopt <project> --drop-local`.
// `<project>` is the adopted beam or `root` that moves the slug, never a task.
// `wouldStop` answers that from the read-only part of step 1, before any slug is touched.
// A project one of whose entries stops is held back whole:
// no owner is written for it and no other entry under it is taken over.
// The named adopt answers the stop before it moves anything.
// The other projects go on.
// `--keep-local` records the owner over those local commits and over a live pair.
// `--drop-local` refuses a live pair naming `--keep-local`.
// Otherwise it calls `fabricengine.DropLocal`, which records into adopt's mutation record.
// `DropLocal` keeps each local tip origin lacks under an `archive/<slug>/<tip>` tag before it moves the branch.
// When the pair's anchor differs from origin's, the refusal ends with a re-run under `--drop-local`.
// The re-run is of the same `lym adopt`, since the pair may be a moved run that no adopt names.
//
// A beam that is a member of an adopted beam, or of root, and records another machine as its owner keeps that owner.
// It is reported on its own outcome as `owned elsewhere` with `owned_by`, and it is neither taken over nor held.
//
// # Step 2: the pair
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
// # Step 3: the hold
//
// Before the owner write, adopt reads the owners again through `ReadOwners`;
// a hold is decided by the owners the reconcile left in Bolt.
// `holdTargets` returns, per subject, the slugs to hold.
// A moved run is held when it went through `takeOverSlug` with no stop, error or warning,
// and its effective owner in those owners is another machine,
// and the write of this call will make it this machine's:
// its project is a beam being written, or `root` while the prime is written.
// An adopted beam's own pawl run also needs its parent owned here or written.
// A run whose pair `LivePair` reads live here is not held and reports `running here`;
// a run that will still read owned elsewhere, such as a nested beam keeping its own owner, is not held.
// A task entry under a subject being written is held too, though no run exists yet:
// it is not done, has an empty status and is not a front task.
// `appendHolds` lists the slugs in each subject's `holds.json`.
// Then one `pauserecord.Update` through `UpdatePause` applies `holdEntries`:
// a slug whose own scope holds an `OwnerCaller` entry has that entry turned into a `HoldCaller` entry in place,
// and the entry keeps its pause time;
// a slug already holding a `HoldCaller` entry keeps it;
// a slug holding an entry of any other caller keeps it and gets no hold,
// and `removeHolds` drops it from `holds.json` again;
// any other slug gets a new `HoldCaller` entry at now.
// The hold is written straight through the record, never through `tenderengine.Pause`.
// So it waits no grace and sends no Esc.
// Only the green adopt check of the slug's subject lifts it.
// A failure to hold refuses the verb before the owner write, naming the re-run.
// A held slug's outcome reads `run` `held` with `held` true.
// The outcome's `pause_standing` line says the run starts by itself once the subject's check is green.
// It also says that a re-run of `lym adopt <subject>` checks again.
// A re-run reports a slug as held when its subject's `holds.json` lists it,
// and a live `HoldCaller` entry of its own scope still covers it, whether or not this call held it.
//
// # The deploy
//
// The reconcile may move the prime past the commit the running lym was built from,
// and a run let go under that lym fails on code it does not know, such as a stencil marker it never fills.
// So when the owner write below records the prime, `deployStale` runs between the hold and the owner write.
// `readDeployState` reads the hub's `landing` config.
// Nothing more is read and nothing runs when `deploy` is empty or `deploy_branches` omits the prime's branch.
// That is the same rule as at the Land row.
// Otherwise it reads the VCS stamp of the running lym through `buildvcs.OfFile`, and the prime's head.
// `installedBehind` reads a build as behind when its revision is a proper ancestor of the prime's head.
// A build of a commit the prime lacks, as of unpushed local work, is not behind.
// A behind build runs the `deploy` command in the prime through `battencli.RunDeploy`, under `battenshed.DeployTimeout`.
// The command's output goes to `deploy.log` in root's adopt scratch directory.
// The envelope's `deploy` names the build it replaced, the prime's head and the log.
// A deploy that fails, times out or cannot start refuses the verb before the owner write, naming the log and the re-run.
// The holds stay, so no run goes on under the old lym, and the re-run judges the build again.
// A state that does not read, as of a lym with no VCS stamp, is a line in `warnings` naming the hand deploy.
// The adopt goes on.
// The deploy runs before the beam steps and the adopt checks.
// So the dobby, the orch and the check this call starts run the new lym.
// Bound: the deploy takes no land lock, so it can run beside a landing's own deploy.
// The deploy command is expected to install by rename, as the hub's own does.
//
// # Step 4: the owner write
//
// One `boardengine.SetOwners` call with `Takeover` records the owners.
// It records this machine as the owner of every adopted beam the earlier steps did not stop.
// It records the prime too when `root` is adopted or when the board records no prime owner.
// A stop that holds root back keeps the prime unrecorded.
// A refused push refuses the verb and starts no run.
// The refusal names its class's way forward and then a re-run of `lym adopt` with the same arguments.
// The holds stay, so the re-run finds them in place.
// A beam the re-read shows owned by another machine stops, and its holds stay.
// Its outcome names that owner and says a re-run of `lym adopt <beam>` takes it over again.
//
// # Step 5: the settled runs
//
// After the owner write the owners are read back, and each adopted beam and moved run is settled over them.
// An entry that reads owned by another machine is left as it stands.
// It is reported as `owned elsewhere` with its `owned_by`, never as an error.
// A held slug keeps its `held` outcome.
// A slug adopt did not hold has its live owner entry lifted through `tenderengine.Lift`.
// The lift is by the entry's scope, caller and pause time.
// A slug still covered by another entry gets a line naming that caller and the way to lift it:
// `lym tender resume <slug>` for an entry of its own scope, or `lym tender resume` for a hub entry.
// A beam with no run record reads `none`.
// Adopt resumes no run and no halted child: a run a hold keeps starts at the green end of its subject's check.
//
// Bound: until the same machine owns a beam and its parent, the beam's pawl run gets no hold and no pass here.
// It is reported owned elsewhere.
// `lym adopt <beam>` on the parent's owner, or `lym adopt <parent>` on the beam's owner, takes it over;
// the adopt check of that subject then resumes it at its green end.
//
// After the owner write, `BeamUp` runs the beam steps for the prime when root is adopted.
// It then runs them for each adopted beam whose owner write stood.
// The steps start the beam's dobby, its beam window and its orch.
// So the pair, the dobby and the orch move with the project.
// `BeamUp` calls `orchcli.BeamUp` with `Prepare` false, since the first step already reconciled origin.
// A beam whose pawl run holds the landing hold gets no beam steps, and its owner is still written.
// `LandingHeld` reads that hold over `boardengine.RunHolds`, since `ReadOwners` carries no run status.
// The beam's `way_forward` names `lym batten status <slug>`.
// When this machine does not also own the beam's parent, it names `lym adopt <parent subject>` too.
// For the prime that is `lym adopt root`.
// The parent's check resumes the landing here at its green end.
// A nil `LandingHeld` reads no hold, and a failed read is the slug's warning naming `lym beam start <slug>`.
// A failed `BeamUp` is the slug's own warning naming `lym beam start <slug>`.
// For the prime it is `prime_beam_warning`, naming `lym beam start root`.
// The other worktrees go on.
// What each step did is the slug's `beam_steps` and the envelope's `prime_beam_steps`.
//
// # Warnings
//
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
// # Step 6: the adopt check
//
// Machines differ in OS and toolchain, so code landed on one can break on another.
// After the beam steps, a call starts one hidden `lym adopt check <subject>` per subject, detached.
// The order is `root` first, then each adopted beam whose owner write stood.
// Last come the other named subjects, `root` included, that still have a held slug:
// their `holds.json` lists a slug that a live `HoldCaller` entry of its own scope still covers.
// So a re-run after a failure starts a new check even when no owner changes.
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
// A hold stands until its subject's check ends green; a check that fails lifts none.
// A spawn that fails is one of the envelope's `check_warnings`.
// The warning names its subject, the error and the re-run of `lym adopt <subject>`.
// It changes no slug's outcome and stops no other subject's check.
// Bound: between a spawn and the child's lock, `lym beam status` can show the previous record,
// and a second `lym adopt` can spawn a check that loses the lock and exits.
// So either envelope's pid can name the check that lost, both naming the same log and record.
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
// `holds.json`, the slugs adopt holds for the subject's check;
// `fix-tries.json`, the count of fix tasks the subject's check has filed.
// Both of the last two are read and rewritten through `internal/state` under their own locks.
// They are kept apart from the check record, so no write of the record touches them.
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
// The marks are sorted by subject, and the state is running while any joined record runs.
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
// `greenEnd` takes these steps in order.
// Each failure is a returned line naming its way forward, and the other steps go on:
//
//  1. It reads the subject's `holds.json`.
//     It lifts the live `HoldCaller` entry of each listed slug, by scope, caller and pause time.
//     `removeHolds` then drops every slug it read in one update,
//     so a slug a second adopt appended meanwhile stays listed for the check of that adopt.
//     A failed lift names `lym tender resume <slug>`.
//  2. It brings the subject's dobby and orch up through `orchcli.BeamUpWorktree`, which leaves a live pair alone.
//     A failure names `lym beam start <subject>`.
//  3. It starts the detached first pass of each lifted slug with a batten run record that is not done.
//     The start goes through `battencli.StartFirstPass` from the prime.
//     A refusal envelope names `lym batten run <slug>` from the prime.
//     A lifted released task with no record is left to the dobby, which starts it once its pause is gone.
//     A start that races the dobby's own pass reads ok, since the pass that loses the driver lock exits on its own.
//  4. It runs `lym loom resume` in the pair of each qualifying task entry.
//     A qualifying entry is not done and runs directly under the subject, under the prime for `root`.
//     Its run is blocked with a reason opening `landingshed.OwnerHaltPrefix`, and its effective owner is this machine.
//     A child halted for another reason, one under another beam and one owned elsewhere are left alone.
//  5. It removes the subject's fix task, which `fronttask.FrontOf` finds over the subject's project.
//     The task is removed through `fronttask.Remove` in any status but done, and its cancel step ends a running one.
//     A failure carries the step's error and the hand removal.
//  6. It writes the subject's `fix-tries.json` count to 0.
//
// Bound: a hold lifted by hand (`lym tender resume`, or `lym batten cancel <slug>`) is gone at the green end.
// The green end then lifts nothing for it.
// `--keep-local` over a live pair leaves that run unheld.
// Bound: a second adopt's reconcile may have moved the tree under a check that is already in its green end.
//
// # The fix loop
//
// When the gate exits non-zero with a failed tier recorded, `runCheck` writes the failed record.
// It then lets go of `check.lock` and calls `checkDeps.Failed` once.
// It posts each line `Failed` returns after the failed notice.
// `hubCheckDeps` sets `Failed` to `failedEnd` over `hubFixDeps`.
// A build error of the deps is the one line it returns, naming its cause and the re-run of `lym adopt <subject>`.
// The holds stand meanwhile.
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
// The envelope carries `reconcile`, `mutations` and `partial`.
// `deploy` reports a deploy the call ran.
// `warnings` covers branches the reconcile left unmoved and a deploy state that did not read.
// `prime_beam_steps` and `prime_beam_warning` report the prime's beam steps.
// `checks` and `message`, or `check_warnings`, report the adopt checks.
// Per slug it carries what happened: a stop with its branches, a pair added and an owner recorded.
// It carries the pause as lifted or standing.
// It carries the run as held for its subject's check, none, running here or owned elsewhere by `owned_by`.
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
// They also include the slugs held for each subject, released tasks included, and each subject's open front task.
// `runReports` gives each the first state that applies, in this order:
//
//   - `owned_elsewhere`: its entry's effective owner after the owner write is another machine.
//   - `landed`: it was a moved run before the reconcile, and its board entry is absent or done after it.
//   - `held_failed_check`: a `HoldCaller` entry of its own scope stands,
//     and its subject's check record reads failed or running with its process gone.
//     This call started no check of the subject, as when that start failed.
//   - `waiting_for_check`: a `HoldCaller` entry of its own scope stands otherwise.
//     A held released task has no row.
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
// `CheckState` reads the subject's check record through `ReadCheck` and `checkAlive`.
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
package adoptcli
