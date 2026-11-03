// Package adoptcli is the adopt module: `lym adopt <beam>...`, `lym adopt root` and `lym adopt --all`, with `--keep-local` and `--drop-local`, the takeover of beam projects, or of root, and of the runs under them from the other machine of the operator.
// It only calls each owning module's takeover function in-process, so no module learns another's state, and it has no engine package of its own.
//
// # The verb
//
// It runs from the hub's prime and from any beam, by `battencli.ResolveBattenPrime`, and acts on the prime's location in both.
// `request` holds the slugs, `Root`, `All`, `KeepLocal` and `DropLocal`, where the word `root` among the slugs sets `Root`; `adoptDeps` holds every step's seam, which `hubDeps` fills over the hub and `TestAdopt` fakes.
// An entry whose slug is `root` is shadowed by that word and cannot be named.
//
// The arguments are beam projects (a project entry whose `beam` is true) and `root`, which names the prime.
// `--all` names `root` and every beam project that is not done.
// Only a beam project and the prime record an owner, so an adopt moves the named projects alone: each one's owner (the prime owner for `root`), its pair, and the runs whose parent is its worktree.
// A nested beam keeps its own owner: `lym adopt root` moves root's tasks and the tasks of the grouping projects below root, never a beam's members, whose parent is that beam.
//
// Before anything runs, `namedEntries` reads the local Bolt and refuses an unknown slug (naming `lym board list`), a note (naming `lym board promote`), a done entry (naming that it already landed, and `lym tender resume <slug>` to let a run that landed there go on to Land under its owner pause), a task (naming `lym adopt <beam>` or `lym adopt root`, the `AdoptSubject` that moves it), a grouping project (naming the same adopt to move its tasks and `lym beam create <slug>` to make it a beam), `--keep-local` with `--drop-local`, and no argument without `--all`.
//
// `movedRuns` lists, sorted, every task and beam project that is not done, holds a batten run record that is not done, and is a child of an adopted beam, or of root when root is adopted, plus each adopted beam's own pawl run.
// A beam below an adopted beam or root is listed too, and keeps its own owner.
//
// It then asks `adoptDeps.MoveInProgress`, filled from `fabricengine.MoveInProgress`, for the prime and for the own worktree of each adopted beam and moved run this machine holds a pair for and each one's parent worktree, and refuses with an empty outcome when an anchor change stands in any of them.
// The refusal carries the record's way forward, a change in progress to wait for or `lym fabric anchor <worktree> --set <anchor>` to finish an interrupted one, and then the same `lym adopt` again, so no pair is added, no run held and no gate suite started under a moving anchor.
// A nil `MoveInProgress` reads as no record standing.
// The moved runs are read once more after the reconcile, which may bring the run records of the other machine.
//
// # Step 1: the origin reconcile
//
// A moving hub-scope reconcile runs first.
// Its `pairteardown` seams take a `HubPaused` from `hubOnlyPaused`, which covers a slug only by a live hub entry, so a pair whose strands are alive under any slug-scope pause, an owner pause included, reads live.
// An unreachable origin refuses naming a re-run once origin answers, a reconcile error refuses naming it and the re-run, and a stop on Bolt or the prime's records branch refuses with the report and that branch's way forward; nothing below runs then.
//
// An adopted beam or a moved run the report reads owned_elsewhere stops before its owner write when a branch of it was judged `needs_replay`, `ahead`, `rewound`, `rewritten`, `diverged` or `anchor_stopped`, or when adopt's own `LivePair` call, made for every such slug with a local pair, reads it live.
// The slug's envelope entry names each branch and its state and the two ways forward, `lym adopt <project> --keep-local` and `lym adopt <project> --drop-local`, where `<project>` is the adopted beam or `root` that moves the slug and never a task.
// `wouldStop` answers that from the read-only part of step 1, before any slug is touched, and a project one of whose entries stops is held back whole: no owner is written for it and no other entry under it is taken over, so the named adopt answers the stop before it moves anything.
// The other projects go on.
// `--keep-local` records the owner over those local commits and over a live pair.
// `--drop-local` refuses a live pair naming `--keep-local`, and otherwise calls `fabricengine.DropLocal`, which keeps each local tip origin lacks under an `archive/<slug>/<tip>` tag before it moves the branch, and records into adopt's mutation record; when the pair's anchor differs from origin's, its refusal ends with re-running the same `lym adopt` under `--drop-local`, since the pair may be a moved run that no adopt names.
//
// A beam that is a member of an adopted beam, or of root, and records another machine as its owner keeps it: it is reported on its own outcome as `owned elsewhere` with `owned_by`, and neither taken over nor held.
//
// # Step 2: the pair
//
// Through `Topology.Add` from the prime, adopt adds the pair of each adopted beam or moved run that holds a run status or is an owned beam, and that this machine has no pair for.
// Add adopts the live branches and their origin record, the beam mark included.
// A refusal stops that slug and the other slugs go on.
// It carries fabric's own message and way forward, bar `*fabricengine.ErrBranchExists`, the leftover local branch's: dropping that branch would lose this machine's unpushed commits with no archive, so adopt rewords it to name tagging the leftover tip under `archive/<slug>/<tip>`, then `git branch -D <branch>` and a re-run of the adopt that moved the slug.
//
// # Step 3: the hold
//
// Before the owner write, adopt reads the owners again through `ReadOwners`, so the owner a hold is decided by is the Bolt the reconcile left, and `holdTargets` returns, per subject, the slugs to hold.
// A moved run that went through `takeOverSlug` with no stop, error or warning is held when its effective owner in those owners is another machine and the write of this call will make it this machine's: its project is a beam being written, or `root` while the prime is written, and an adopted beam's own pawl run also needs its parent owned here or written.
// A run whose pair `LivePair` reads live here is not held and reports `running here`; a run that will still read owned elsewhere, such as a nested beam keeping its own owner, is not held.
// Each task entry under a subject being written that is a task, not done, with an empty status and not a front task, is held too, though no run exists yet.
// `appendHolds` lists the slugs in each subject's `holds.json`, then one `pauserecord.Update` through `UpdatePause` applies `holdEntries`: a slug whose own scope holds an `OwnerCaller` entry has it turned into a `HoldCaller` entry in place, keeping its pause time; one already holding a `HoldCaller` entry keeps it; one holding an entry of any other caller keeps it, gets no hold and is dropped from `holds.json` again by `removeHolds`; any other gets a new `HoldCaller` entry at now.
// The hold is written straight through the record, never through `tenderengine.Pause`, so it waits no grace and sends no Esc, and only the green adopt check of the slug's subject lifts it.
// A failure to hold refuses the verb before the owner write, naming the re-run.
// A held slug's outcome reads `run` `held` with `held` true and a `pause_standing` line saying it starts by itself once the subject's check is green and that a re-run of `lym adopt <subject>` checks again.
// A re-run reports each slug its subject's `holds.json` lists and a live `HoldCaller` entry of its own scope still covers as held, whether or not this call held it.
//
// # The deploy
//
// The reconcile may move the prime past the commit the running lym was built from, and a run let go under that lym fails on code it does not know, such as a stencil marker it never fills.
// So when the owner write below records the prime, `deployStale` runs between the hold and the owner write.
// `readDeployState` reads the hub's `landing` config: with an empty `deploy`, or `deploy_branches` not listing the branch the prime is on, nothing more is read and nothing runs, as at the Land row.
// Otherwise it reads the VCS stamp of the running lym through `buildvcs.OfFile` and the prime's head, and `installedBehind` reads the build as behind when its revision is a commit the prime holds, is not the head and is an ancestor of it; a build of a commit the prime lacks, as of unpushed local work, is not behind.
// A behind build runs the `deploy` command in the prime through `battencli.RunDeploy` under `battenshed.DeployTimeout`, with its output in `deploy.log` in root's adopt scratch directory; the envelope's `deploy` names the build it replaced, the prime's head and the log.
// A deploy that fails, times out or cannot start refuses the verb before the owner write, naming the log and the re-run: the holds stay, so no run goes on under the old lym, and the re-run judges the build again.
// A state that does not read, as of a lym with no VCS stamp, is a line in `warnings` naming the hand deploy, and the adopt goes on.
// The deploy runs before the beam steps and the adopt checks, so the dobby, the orch and the check this call starts run the new lym.
// Bound: the deploy takes no land lock, so it can run beside a landing's own deploy; the deploy command is expected to install by rename, as the hub's own does.
//
// # Step 4: the owner write
//
// One `boardengine.SetOwners` call with `Takeover` records this machine as the owner of every adopted beam the earlier steps did not stop, and of the prime when `root` is adopted or when the board records no prime owner, and not while a stop holds root back.
// A refused push refuses the verb, starts no run and names its class's way forward and then a re-run of `lym adopt` with the same arguments; the holds stay, so the re-run finds them in place.
// A beam the re-read shows owned by another machine stops, naming that owner and that a re-run of `lym adopt <beam>` takes it over again, and its holds stay.
//
// # Step 5: the settled runs
//
// After the owner write the owners are read back, and each adopted beam and moved run is settled over them.
// An entry that reads owned by another machine is left as it stands and reported as `owned elsewhere` with its `owned_by`, never as an error; a held slug keeps its `held` outcome.
// A slug adopt did not hold has its live owner entry lifted through `tenderengine.Lift`, by its scope, caller and pause time, and a slug still covered by another entry gets a line naming that caller and `lym tender resume <slug>` for an entry of its own scope or `lym tender resume` for a hub entry.
// A beam with no run record reads `none`.
// Adopt resumes no run and no halted child: a run a hold keeps starts at the green end of its subject's check.
//
// Bound: until the same machine owns a beam and its parent, the beam's pawl run gets no hold and no pass here, reported owned elsewhere; `lym adopt <beam>` on the parent's owner, or `lym adopt <parent>` on the beam's owner, takes it over, and the adopt check of that subject resumes it at its green end.
//
// After the owner write, `BeamUp` runs the beam steps of the prime when root is adopted, then of each adopted beam whose owner write stood, through `orchcli.BeamUp` with `Prepare` false, since the adopt's own first step already reconciled origin: the beam's dobby, its beam window and its orch, so the pair, the dobby and the orch move with the project.
// A beam whose pawl run holds the landing hold, read through `LandingHeld` over `boardengine.RunHolds` since `ReadOwners` carries no run status, gets no beam steps: its owner is still written, and its `way_forward` names `lym batten status <slug>` and, when this machine does not also own the beam's parent, `lym adopt <parent subject>`, `lym adopt root` for the prime, whose check resumes the landing here at its green end.
// A nil `LandingHeld` reads no hold, and a failed read is the slug's warning naming `lym beam start <slug>`.
// A failed `BeamUp` is the slug's own warning naming `lym beam start <slug>`, `prime_beam_warning` naming `lym beam start root` for the prime, and the other worktrees go on; what each step did is the slug's `beam_steps` and the envelope's `prime_beam_steps`.
//
// # Warnings
//
// For each branch the reconcile report holds whose outcome reads `Prime`, or whose `Slug` is an adopted beam, in state `rewritten` or `diverged`, or `ahead` with `Moved` unset, that no stop of that slug already reported and that the shared-stop refusal did not judge, `reconcileWarnings` returns a line naming the branch and its state, `lym fabric pull` in its worktree and the re-run of `lym adopt` with the same arguments.
// The worktree is the `CodeWorktree` for a records branch, where its fabric verbs run, and the `Worktree` for a code branch; the reconcile's own way forward follows in parentheses when it set one.
// A records branch the reconcile replayed reads `ahead` with `Moved` set, so it gets no warning.
// Adopt goes on, carries the lines as the envelope's `warnings` and runs no `lym fabric pull`.
//
// # Step 6: the adopt check
//
// Machines differ in OS and toolchain, so code landed on one can break on another.
// After the beam steps, a call starts one hidden `lym adopt check <subject>` per subject, detached, `root` first and then each adopted beam whose owner write stood, then each other named subject, `root` included, whose `holds.json` lists a slug a live `HoldCaller` entry of its own scope still covers, so a re-run after a failure starts a new check even when no owner changes.
// Before each start it writes the subject's `fix-tries.json` count to 0 through `ResetTries`: a hand adopt resets the try count, and a check started otherwise does not.
// The spawn is logged through `logger`.
// `startCheckWithTiers` first calls `resolve`, the subject's worktree resolve, and spawns nothing for a worktree that does not resolve, returning an error naming `lym beam start <subject>` and the re-run of `lym adopt <subject>`;
// `ensureCheck` spawns nothing when the subject's `check.json` reads `running` and `checkAlive` finds its process still alive.
// The check's stdout and stderr go to `check.out`, created for append and never `suite.log`, so the gate's output alone reaches `suite.log`, a losing check's envelope goes to `check.out` and a panic of the check is on disk.
// A tier the gate's green record holds green for a worktree's tree is carried and runs nothing.
// `checkAlive` reads a pid that is alive and whose start ticks equal the record's as alive, a pid with other ticks, as after a reboot that reused it, as dead, and a record without ticks, or a platform where the start time is unreadable, as alive on the pid alone.
// The envelope's `checks` hold one item per subject: its `subject`, the check's pid, `suite.log`, `check.json`, the tiers of the hub's `adopt` gate key as the gate config reads before the check's own reconcile, none when it does not read, and `already_running` for a check found running;
// its `message` is one line per check naming its subject, started or already running, those tiers, and that the prime's reed status bar shows its progress and result.
// A hold stands until its subject's check ends green; a check that fails lifts none.
// A spawn that fails is one of the envelope's `check_warnings`, naming its subject, the error and the re-run of `lym adopt <subject>`, and it changes no slug's outcome and stops no other subject's check.
// Bound: between a spawn and the child's lock, `lym beam status` can show the previous record and a second `lym adopt` can spawn a check that loses the lock and exits, so either envelope's pid can name the check that lost, both naming the same log and record.
//
// # The check verb
//
// The hidden `lym adopt check <subject>`, audience internal and taking exactly one argument, a beam slug or `root`, runs the hub's `adopt` gate in-process in the subject's worktree through `gatecli.RunGate` and reports each tier and its end to that worktree's nearest live orch.
// `hubCheckDeps` resolves the subject's worktree, the prime for `root` and the beam's pair through `lymcwd.ResolveWorktree` for a beam, and returns an error naming `lym beam start <subject>` when it does not resolve.
// The verb resolves the prime from the working directory as the adopt does, so it runs from anywhere in the hub, and prints an ok envelope with the record's `state`, or `already_running: true` when another check of the subject holds the lock.
// `adopt` itself takes arbitrary arguments, so a standalone `RunCLI` still reaches `adopt <subject>`; a beam named `check` cannot be named to `lym adopt` and is adopted through `--all`.
//
// Before it takes the lock, `runCheck` reads the subject's owner through `checkDeps.OwnedElsewhere`, which `hubCheckDeps` fills from `boardengine.ReadOwners`: the beam's effective owner, or the prime owner for `root`, against this machine's name.
// A subject another machine owns posts `adopt check of <subject>: <subject> is owned by <owner>, so no check ran; way forward: run lym adopt <subject> on this machine`, returns that text as its error and writes no record;
// a hand `lym adopt check` on a machine that does not own the subject reaches it.
// A read error is returned likewise, writing nothing.
//
// Each subject's files sit under `<fabricengine.AdoptScratchDir>/<subject>/` and are named only here:
// `check.json`, the progress record, written through `internal/state` under its own state lock;
// `check.lock`, the verb's own lock;
// `check.out`, which the spawn of the check points its stdout and stderr at;
// `suite.log`, the gate's log;
// `holds.json`, the slugs adopt holds for the subject's check, and `fix-tries.json`, the count of fix tasks the subject's check has filed, each read and rewritten through `internal/state` under its own lock, kept apart from the check record so no write of the record touches them.
// A record left at the top of the scratch directory by an older binary is not read.
// `CheckRecord` holds the check's subject, pid and start ticks, its start, the gate key, each `CheckTier` as it ended with the failed tests the gate's counter named, the tier in flight, its state (`running`, `green` or `failed`), its end time, the first failed tier and an error, and an unknown key reads.
// `ReadCheck` stats the record first and reports false, taking no lock and creating no directory, when it is absent; a record that does not decode reads as absent with one Warn, so a damaged file never blocks the next check.
//
// `runCheck` takes `check.lock` with a non-blocking acquire and returns at once, having written and posted nothing, when it is held.
// Holding it, it truncates `suite.log`, writes a running record and reconciles the hub through `hubreconcile.Ensure`, so a hub whose build stamp is stale seeds `adopt` from its tuned `finalize` before the gate reads it.
// A failing reconcile ends the check failed with the way forward and runs no tier, so no check goes green on the template's tiers.
// The gate's `suite.log` is its log; its observer rewrites the record as each tier starts and ends and posts `adopt check of <subject>: tier<N> <green|failed>, passed <p> failed <f> skipped <s>, <wall>s`, every notice opening `adopt check of <subject>:` so the orch tells two subjects' checks apart;
// a failed tier's notice adds one line per failed test, `<package> <test>: <first line>`, at most `maxListedFailures` and then `and <n> more`, and a last line naming `suite.log`.
// A green gate ends the record green and posts the tiers and `suite.log`; any other exit ends it failed, with the first failed tier, or with the gate's exit code as the error when no tier ended.
// A failed tier posts the failed tier and `suite.log`, and the lines of the fix loop follow it.
// A reconcile failure and a gate that exits before any tier ended are no verdict: they start no fix loop, name no worktree, and name the fix of what stopped them and the re-run `lym adopt <subject>`.
// A tier that refuses after it started returns without ending, so the end clears the tier in flight.
// At its start, as each tier starts and ends, and at its end it also sets the check mark on the prime's reed session through `reedengine.Engine.SetCheckMark`, built by `checkMarkOf` from the record and so naming its subject: `adopt check of <subject>: starting`, the done tiers with their outcomes and wall seconds then the tier in flight timed from its start, and at the end `adopt check of <subject> green` or `adopt check of <subject> failed` with every tier it ran, or a failure before any tier ended naming `lym beam status`.
// `primeMark` then shows the mark joined, by ` | `, with the mark of each other subject whose record reads running, sorted by subject, and its state is running while any joined record runs.
// A post or mark that fails is logged and changes nothing.
// Bound: a check killed before its end leaves its last running mark, its seconds still growing, until the next check sets one or the prime's session ends.
//
// Bound: the gate slots do not serialize two checks, the lock does, so one check of a subject at a time writes its record, its log and its posts.
//
// # The green end
//
// Once the gate exits 0 and the green record is written, `runCheck` lets go of `check.lock`, calls `checkDeps.Green` once and posts each line it returns to the orch as one notice after the green notice.
// A failed end, an end before any tier and a subject owned by another machine never call it.
// `hubCheckDeps` sets `Green` to `greenEnd` over `hubGreenDeps`, and a build error of the deps is the one line it returns.
// `greenEnd` takes these steps in order, each failure a returned line naming its way forward while the other steps go on:
//
//  1. It reads the subject's `holds.json` and lifts, by scope, caller and pause time, the live `HoldCaller` entry of each listed slug, then `removeHolds` drops every slug it read in one update, so a slug a second adopt appended meanwhile stays listed for the check of that adopt. A failed lift names `lym tender resume <slug>`.
//  2. It brings the subject's dobby and orch up through `orchcli.BeamUpWorktree`, which leaves a live pair alone; a failure names `lym beam start <subject>`.
//  3. It starts the detached first pass, through `battencli.StartFirstPass` from the prime, of each lifted slug with a batten run record that is not done. A refusal envelope names `lym batten run <slug>` from the prime. A lifted released task with no record is left to the dobby, which starts it now its pause is gone, and a start that races the dobby's own pass reads ok, since the pass that loses the driver lock exits on its own.
//  4. It runs `lym loom resume` in the pair of each task entry not done that runs directly under the subject, under the prime for `root`, whose run is blocked with a reason opening `landingshed.OwnerHaltPrefix` and whose effective owner is this machine. A child halted for another reason, one under another beam and one owned elsewhere are left alone.
//  5. It removes the subject's fix task, `fronttask.FrontOf` over the subject's project, in whatever status it stands not done, through `fronttask.Remove`, whose cancel step ends a running one; a failure carries the step's error and the hand removal.
//  6. It writes the subject's `fix-tries.json` count to 0.
//
// Bound: a hold lifted by hand (`lym tender resume`, or `lym batten cancel <slug>`) is gone at the green end, which then lifts nothing for it; `--keep-local` over a live pair left that run unheld.
// Bound: a second adopt's reconcile may have moved the tree under a check that is already in its green end.
//
// # The fix loop
//
// Once the gate exits non-zero with a failed tier recorded and the failed record is written, `runCheck` lets go of `check.lock`, calls `checkDeps.Failed` once and posts each line it returns after the failed notice.
// `hubCheckDeps` sets `Failed` to `failedEnd` over `hubFixDeps`, and a build error of the deps is the one line it returns, naming its cause and the re-run of `lym adopt <subject>` with the holds standing meanwhile.
// `fixDeps` holds the try limit, `adopt_fix_tries` of the subject worktree's `beam.yaml`, and the board's vocabulary read through `boardengine.LoadConfig`.
// `failedEnd` takes the first case that applies, each returning the lines of its notice:
//
//  1. The subject's own fix task, `fronttask.FrontOf` over the subject's project and the `adopt-check` action, stands open and healthy: released with the empty status, a run status with no readable batten run record (starting), a run record `running` or `paused`, or `awaiting` with an admitted escalation. Nothing is filed or removed; the notice names the task, its state, that its landing starts the next check, and the hand steps: `lym batten run <slug>` from the prime for a paused one, and `lym batten cancel <slug> --remove` then `lym board remove '{"slug":"<slug>"}'` to drop it.
//  2. Another open front task, `fronttask.OpenFront` over the board without the own fix task, stands in the subject's worktree. Nothing is filed or removed; the notice names it and the re-run of `lym adopt <subject>` once it lands.
//  3. The try count in `fix-tries.json` is at the limit. Nothing is filed or removed; the notice names the failing tiers and tests, `suite.log`, a halted own fix task when one stands, and the re-run of `lym adopt <subject>`, whose check removes that task.
//  4. The own fix task is a draft or halted, `fixRunHalted` reading the run record's state `blocked`, `failed`, or `awaiting` with no admitted escalation. It is removed through `fronttask.Remove` right before the filing; a failed removal files nothing, and its line carries the step's error and hand removal, adding the re-run only when the entry's removal failed.
//  5. A task is filed through `fronttask.File` with the slug `nextFixSlug` gives, the kind task, the title `Fix the adopt check of <subject> on <machine>`, the body `fixBody` renders, the labels `bug` and, when the board's vocabulary knows it, `gate`, the recipe `darn`, the profile `light`, the beam's project (none for `root`) and `on_land` `adopt-check`. The try count is written one higher between the entry's creation and its release, so a filing that dies between the writes still counts and one the create refused does not. A filing error files nothing more and names the re-run of `lym adopt <subject>`, which resets the count and checks again.
//
// `failedEnd` never starts a pass of the task: its parent's dobby starts it from the empty status.
// `fixSlugPrefix` is `adopt-fix-<subject>` when that and `-t999` fit `boardengine.MaxSlugLength`; otherwise `adopt-fix-`, the subject cut short, a hyphen and the first six hex digits of the subject's SHA-256.
// `nextFixSlug` returns `<prefix>-t<N>` with N one more than the highest N among the board's slugs, the batten run directories and the task branches of the prefix, `fabricengine.TaskBranchSlugs`, so a filing meets neither a landed slug nor a leftover branch.
// `fixBody` holds the failing tiers, the failed tests the gate named, an excerpt of `suite.log` of at most `fixExcerptLines` lines around each failed test's first line, the instruction to diagnose first, and the stop condition for a machine failure, which names `lym adopt <subject>` as the way forward and never `lym loom resume`.
//
// Bound: at most `adopt_fix_tries` fix tasks per subject between two hand adopts, and at most one at a time.
//
// # The check view
//
// `ReadCheckViews` reads the `check.json` of each subject directory under `fabricengine.AdoptScratchDir` through `ReadCheck`, which on an absent record reports false having created no directory or lock file, and returns the views sorted by subject, none when no directory holds a record; `lym beam status` shows them as the `adopt_checks` section, one item per subject.
// `checkView` builds the `CheckView` from the record: a running record shows the tier in flight and the tiers done, a green or failed record its end time, a failed one also its failing tier, every view names its subject and every view's lines open `adopt check of <subject>:` and name `suite.log`.
// A running record whose pid is not alive, or is alive with start ticks other than the record's, as after a reboot that reused the pid, shows `ended without a verdict`, naming `suite.log`, `check.out` and a re-run of `lym adopt <subject>`, and that the record clears at the next `lym adopt` with an owner write, which starts a fresh check, since the rerun writes no record.
//
// # The envelope
//
// It carries `reconcile`, `mutations` and `partial`, `deploy` for a deploy the call ran, `warnings` for branches the reconcile left unmoved and a deploy state that did not read, `prime_beam_steps` and `prime_beam_warning` for the prime's beam steps, `checks` and `message`, or `check_warnings`, for the adopt checks, and per slug what happened: a stop with its branches, pair added, owner recorded, pause lifted or standing, run held for its subject's check, none, running here or owned elsewhere by `owned_by`, the `beam_steps` run, the error, and a warning when this machine does not own the slug's parent, since that run halts at Finalize here; the warning names `lym adopt <project of the parent>`, `lym adopt root` for the prime.
//
// `runs` is always present, an empty array for none, and holds one `runReport` for each run under the named subjects, sorted by slug, beside the per-slug outcomes:
// its `slug`, `state`, `row` (the batten run record's current producer, omitted for an entry with no record), `owned_by` when owned elsewhere, `caller` for `paused`, and a plain `line`.
// The runs are the union of the moved runs before the reconcile, the moved runs after it, the slugs held for each subject, released tasks included, and each subject's open front task.
// `runReports` gives each the first state that applies, in this order:
//
//   - `owned_elsewhere`: its entry's effective owner after the owner write is another machine.
//   - `landed`: it was a moved run before the reconcile and its board entry is absent or done after it.
//   - `held_failed_check`: a `HoldCaller` entry of its own scope stands, its subject's check record reads failed or running with its process gone, and this call started no check of the subject, as when that start failed.
//   - `waiting_for_check`: a `HoldCaller` entry of its own scope stands otherwise; a held released task has no row.
//   - `paused`: a live entry of its own scope of another caller stands, naming that caller.
//   - `halted`: its run record reads halted through `fixRunHalted`, the escalation read through the `EscalationEligible` seam; the line names the halt reason, and for the subject's fix task the re-run of `lym adopt <subject>`.
//   - `ended`: it was a moved run before the reconcile, its entry after it reads a status that is neither empty, a run status nor done, such as the draft a cancel on the other machine writes, and its run record is gone.
//   - `running_here`: any other run this machine owns, the fix task included; an unstarted fix task has no row.
//
// `CheckState` reads the subject's check record through `ReadCheck` and `checkAlive`.
// A read that fails leaves `runs` empty and adds a line to `warnings` naming the re-run of the same `lym adopt`; the steps already taken stand.
//
// # The bounds
//
// Adopt writes owners without the old owner's help and stops no driver live elsewhere, which pauses at its next owner check.
// Under a hub pause `LivePair` reads not live, so `--drop-local` can move a branch under interrupted strands, as any hub-paused pair's reconcile does today.
// A slug whose pair has no batten run record never reads owned_elsewhere, since the reconcile judges only recorded pairs, so its local commits do not stop it before the owner write.
// Whether origin holds an entry's branches is read from the board, not from origin: a run status, or an owner on a beam, marks an entry that was started, and the add then adopts its live pair.
package adoptcli
