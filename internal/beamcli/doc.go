// Package beamcli is the beam module: `lym beam create <slug>`, the making of a beam from a board project entry, `lym beam describe <slug>`, the writing of the beam's change description, `lym beam pr open <slug>`, `update`, `status` and `close`, the opening, updating, reading and closing of its pull request, `lym beam ready <slug>`, the flag that a beam is ready to merge, `lym beam land <slug>`, the landing of a beam into its parent, `lym beam merge-parent <slug>`, the filing of a task that merges a beam's parent branch into it, `lym beam start [slug] [--all]`, the bring-up of a beam's dobby and orch, `lym beam stop [slug]`, their bring-down, `lym beam status`, the view of every local dobby and orch, and the hidden `lym beam dobby`, the dobby process itself.
// The verbs only call each owning module's function in-process, so no module learns another's state; the daemon's engine is `internal/dobbyengine`, and this package wires its seams over the hub.
//
// # The verbs
//
// Every verb but `start` and `stop` resolves its location through `battencli.ResolveBattenPrime`, so it runs from the hub's prime and from any beam, and acts on the prime's location.
// The module is hub-only, so it declares no `cliwire.Module`.
// A verb's seams are a deps struct that `hubCreateDeps` fills over the hub and `TestCreate` fakes.
//
// # create
//
// `create <slug>` takes an existing project entry that is not done, whose `beam` is false, or whose `beam` is true and whose own owner is this machine or none, and whose parent does not read as owned by another machine.
// The project's title, brief and labels are board writing, so the verb makes no entry.
// It runs first from the beam's parent worktree only, the prime or the parent beam `boardengine.ParentBeam` names, and `refuseCreateHere` refuses elsewhere naming that worktree's path and the same command there;
// a parent beam whose pair is absent here is named instead by `lym beam create <parent>`, or `lym adopt <parent>` when its branch is on origin, then the same command from the parent's worktree.
// It refuses an absent entry or a note (naming `lym board upsert` of a project entry), a task (naming `lym board upsert` of a project entry under a slug of its own, then `lym beam create` on it), a done project, a beam another machine owns (naming the owner, `lym beam create` there and `lym adopt <slug>`), and a parent another machine owns, through `Owners.Effective`, naming `lym adopt` of `Owners.AdoptSubject` of the parent: the nearest beam above it, else `root`.
//
// The steps run in order, each skipped when its result is already there, so a re-run finishes a stopped create:
//
//  1. `boardengine.SetOwners` with `Prime` claims the root for this machine when the board records no root owner.
//     A claim another machine wins reads the board again, and a parent that now reads as theirs is refused naming `lym adopt root`.
//  2. `boardengine.SetOwners` records this machine as the beam's owner with the beam flag and `recipe: pawl`, and the beam's colour when it has none; a write lost to another machine is refused naming `lym adopt <slug>`.
//     A beam recorded here with no colour runs the step again.
//  3. `battencli.StartFirstPass` starts the beam's batten run as a detached first pass recording the anchor, driven on by the parent's dobby.
//     A run in flight, a batten status file not done, is reported live and starts nothing; a flagged beam with no run status, or a legacy beam with a marked pair and no pawl run, gets its pawl run.
//     A done run whose pair is gone is first passed to `battencli.ClearMotifRun`, which removes it when its child ran the motif recipe, so a project split after its motif run starts its pawl run under the same slug; the envelope says so with `motif_run_cleared`.
//     Any other done run stays, and the first pass refuses it naming `lym batten cleanup --apply`.
//     A first pass whose envelope is not ok, such as one a covering pause refused, is the verb's error carrying the envelope's error and leaves no run.
//
// The verb makes, marks and pushes no pair itself: the run's Worktree-Create makes the pair from the parent beam, unmarked, at the anchor the run's seed records, its Record-Write writes the project record from the entry's body, and its Beam-Up marks the pair and starts the beam's dobby and orch.
// `--anchor <dir>` is recorded in the seed's `anchor` param on the first pass only, so it rides in the run record and a resume keeps it.
// A first pass a covering pause refused leaves no run, and the same `lym beam create <slug> [--anchor <dir>]` after the lift starts it; a flagged beam left by a first pass that died takes the same `--anchor` again, since its anchor reached no run record.
// Dobby never starts a beam project, so only this verb, a hand pass or the green end of the adopt check that `lym adopt` starts starts or resumes its run.
//
// Writing the beam flag first makes a member started mid-create stop at Worktree-Create and resume into the beam rather than fork from the prime for good.
// Members already spawned under the grouping project keep the prime as their parent.
// The envelope names `lym batten status <slug>` as the way to follow the run.
//
// # ready
//
// `ready <slug>` flags the beam ready to be merged into its parent and is final: it writes a `pawlengine.ReadyRecord` naming the beam at `loomengine.BeamReadyPath` of the beam's pair, beside the change description, and commits the landing directory through `fabricengine.CommitAnchoredPaths`.
// No verb removes the record; Finalize's landing, Beam-Down and the retired-row move end it.
// The flag does not say the project is finished; once it stands and nothing works under the beam, the beam's dobby writes the land request itself.
// It runs anywhere in the hub, as `land` does, refuses what `land`'s entry check refuses, and reports a beam already flagged without judging or rewriting it, though a record left uncommitted is committed.
// For a beam not yet flagged, `judgeReadyLanding` refuses, in order, each naming its way forward:
//  1. a run not at Beam-Await, in any state;
//  2. open members and child pairs, as `landDeps.Refusal` words them;
//  3. a parent that holds commits the beam lacks (`ParentUnmerged`), naming `lym beam merge-parent <slug>`, or `lym batten status` of the merge-in task already open;
//  4. a landing record with no `DescribedAt`, or a description that fails validation (`DescriptionFindings`), naming `lym beam describe <slug>`;
//  5. with no pull request recorded, `landingshed.ResolvePullRequest` over the parent's mode and GitHub's rules (`PullRequestMode`): a missing origin fact the decision demands a pull request over, naming the fact, and a yes for a `required` parent or GitHub's rules, naming `lym beam pr open <slug>`;
//  6. with one recorded, `pullrequest.ReadMergeState` at the beam's head: a closed pull request, one open at another head, one that cannot merge now with each cause's action, and one merged without every beam commit (`MergedLacks`), each naming `lym beam pr` verbs.
//
// A `merge` parent passes with none recorded, since Finalize opens its own, and so does a beam with no commits beyond its parent, since Finalize lands it with no pull request.
// The parent's tips are read after a fetch under the remote-refs lock when the parent has an upstream and the hub's push skip is unset, and the local tip alone otherwise.
// The record names its beam because a pair forked from the beam inherits the beam's `_lym` records: `pawlengine.ReadyFor` reads a record naming another slug as no flag.
// The beam's orch or the operator runs it; dobby never does.
//
// # describe
//
// `describe <slug>` writes the beam's change description and returns its path.
// It runs anywhere in the hub, refuses what `land`'s entry check refuses, and refuses a beam with no pawl run (naming `lym beam create <slug>`), a beam whose run is not at Beam-Await (naming `lym batten status <slug>`), whatever its state, and a beam whose ready record stands, since `ready` is final.
// A run at Beam-Await passes in every state, so a halted run is served: Beam-Await halts by an error stop the status records as failed.
// `judgeAwaitingRun` and `judgeNotReady` are those two refusals, which every orch verb of the landing reuses.
// The description is `landingshed.WriteBeamDescription`'s: it owns `summary.md` in the beam's landing directory and writes it whole each run from the entry's title and brief, the beam's line from `fabricengine.BeamLineCommits` without its records-only commits, and the project record's added decisions.
// The stamp of `pawlengine.LandingRecord.DescribedAt` and the commit of the landing directory share one commit with the description, made after the description validated, so a refused description stamps nothing.
// A finding is refused naming `lym board upsert` for the entry's title or brief.
// The envelope names the description's path and `lym beam pr open <slug>`, or `lym beam pr update <slug>` when a pull request is recorded.
//
// # pr
//
// `pr open <slug>` and `pr update <slug>` are thin verbs over `internal/pullrequest` that act only on the pull request from the beam's own branch into its parent.
// They open, edit and reopen it, read a token only through `internal/githubclient`, never merge, and change nothing once the ready record stands.
// `judgePullRequestVerb` runs, in order, the entry check (an open beam with a pair here), the owner check (`judgeLandOwners`, which takes the verb so its refusals name it), `judgeAwaitingRun` and `judgeNotReady`.
// A step that fails is refused naming its cause and the way forward of fixing it and running the same verb again.
//
// `open` refuses a landing record with no `DescribedAt` for the beam (naming `lym beam describe <slug>`), a beam with no commit beyond its parent (naming landing a member into the beam first, or `lym beam ready <slug>` to land without a pull request) and an origin that `landingshed.OriginMissing` finds a fact missing from.
// A recorded pull request that GitHub reports open answers `already` and commits the landing directory, one reported closed is reopened and answered `reopened`, and one reported merged changes nothing and names `lym beam ready <slug>`, after which Finalize lands it.
// With none recorded it pushes the beam branch through `fabricengine.PushPairAnchored`, finds an open pull request from the branch into the parent or creates one with the description's title and body, marks a draft it found ready for review and leaves its text, and records `Number`, `URL` and the pushed head as both `PushedHead` and `AnnounceHead`.
// A record write that fails after a pull request stands is refused naming its URL and `lym beam pr open <slug>`, which finds it open and records it.
//
// `update` refuses a beam with no pull request recorded, naming `lym beam pr open <slug>`; it pushes the branch, sets the title and body from the description, marks a draft ready for review and records the pushed head as `PushedHead`.
//
// `status <slug>` is read-only and answers at any row up to Beam-Down: it runs the entry check alone and, with no pull request recorded, answers `recorded: false` naming `lym beam pr open <slug>` at Beam-Await and Finalize's decision at a later row.
// With one recorded it prints `pullrequest.ReadMergeState`'s reading of it at the beam's head: state, draft, head, mergeable state, approval, each required check with its conclusion, `mergeable_now` and, when false, each cause.
//
// `close <slug>` runs `judgePullRequestVerb`, refuses a beam with no pull request recorded (naming `lym beam pr open <slug>`), closes an open recorded pull request with one comment and drops it from the landing record, committing the landing directory; a pull request GitHub reports closed or merged is only dropped.
// The comment and the answer come from `landingshed.ResolvePullRequest` for the beam with none recorded: a no comments that the operator chose to land without a pull request, a yes comments only the close and answers that a `merge` parent's Finalize still lands through a pull request, or that `lym beam ready <slug>` refuses until `lym beam pr open <slug>` records one again.
// A failed read of the parent's mode or of the beam's commits still closes and drops, with the close-only comment, and the answer reports the read's error.
// The orch closes a pull request only to land without one, never to add work, since work lands into the beam while it stays open.
//
// The seams the verbs share are `landDeps`' `PullRequestSide`, `PushBeam`, `BeamHasCommits`, `Landing`, `WriteLanding`, `Description`, `BeamHead`, `PullRequestMode` and `Sleep`; `hubLandDeps` fills them over the beam's pair.
// `BeamHasCommits` is whether the pair's HEAD holds a commit the local parent branch lacks, the read Finalize makes of a task pair, so every verb that judges an empty beam judges it alike.
// `PullRequestMode` is the parent's beam.yaml `pull_request`, read from the parent pair the beam's origin record names, with whether GitHub's rules for the parent branch require a pull request, which `parentRuleRequired` reads only when the origin allows a pull request and reads as no rule when the token is unusable or the call fails for a reason that is not transient.
//
// # land
//
// `land <slug>` asks the beam's pawl run to land the beam by writing a land request, and starts no run of its own.
// It runs anywhere in the hub, a task pair and the beam itself included, resolving the prime from the cwd's worktree and never through `battencli.ResolveBattenPrime`, which refuses a worktree that is neither the prime nor a beam.
// The request is a `pawlengine.LandRequest` naming the pawl run by its child seed's `started_at`, written through `pawlengine.WriteLandRequest` under the beam worktree's `.lym/beam/` directory, where Beam-Await consumes it; the beam's pair is removed only at the run's teardown.
// It answers the request's path and `lym batten status <slug>` to follow the landing.
//
// It refuses, in order, each naming its way forward:
//   - an entry that is not an open beam project of this machine with a pair that carries the mark;
//   - a beam with no pawl run, naming `lym beam create <slug>`, and a beam whose run is not running at Beam-Await, read from the beam pair's own child status, naming `lym batten status <slug>`;
//   - a beam this machine does not own, naming `lym adopt <slug>`, and a beam whose parent another machine owns, naming the parent's adopt subject (`lym adopt root` for the prime);
//   - a beam whose ready record does not flag it, naming `lym beam ready <slug>`;
//   - `battencli.BeamLandRefusal`.
//
// The request is only a file: Beam-Await accepts only one naming its own run, writes the landing hold, a status naming Finalize, first, stops the dobby and re-runs every check above but the run-at-Beam-Await one before anything lands.
//
// `BeamLandRefusal` judges members and member pairs alone, each refused with its way forward:
//   - any open entry whose `hubgeom.ParentOf` is the beam, through any depth of grouping projects, naming landing it, `lym batten cancel <slug> --remove`, or, for an entry with no pair, moving its `project` off the beam;
//   - any local pair whose origin record names the beam as its parent worktree, naming `lym batten run` and `lym batten cancel --remove`.
//
// The beam's orch is no blocker, since it runs until Beam-Down.
// `battencli.ReadBeamLandBlockers` reads the same facts as data, `BeamLandBlockers` with each member's `Slug`, `HasPair` and `Running`, for a caller that words its own ways forward.
//
// Batten's start guard applies the same refusal to a project entry, and Worktree-Create applies it again after its claim, so a resume or a direct `lym batten run` cannot skip it and a member added before the claim is caught.
// The guard also refuses a run of any project entry before the claim unless it is a landable beam: a grouping project, naming `lym beam create`, and a beam with no local pair, naming `lym beam create` and `lym adopt`.
// A marked beam pair passes the guard's dirty-source and left-pair checks, so the landing reaches Worktree-Create, which finds the pair complete and skips; a landing forks no pair, and Worktree-Create never calls `Add` for a project.
//
// Bound: a pair on another machine whose origin names the beam is not seen when its entry no longer names the beam, as for a pair made by hand there with `--parent <beam>` and no board entry.
//
// # merge-parent
//
// `merge-parent <slug>` files the beam's merge task: a released darn task in the beam's project whose run merges the beam's parent branch, the `ParentBranch` of the beam pair's origin record, into the task's branch, which its landing brings into the beam.
// It runs anywhere in the hub, as `land` does, and refuses what `land`'s entry check refuses, a beam whose origin record names no parent branch, naming `lym fabric reconcile`, and a beam that holds an open merge task, naming `lym batten status` and `lym batten cancel --remove` of that task.
// A merge task is found by its project and its slug's `-merge-in` suffix, with an optional number; the new one takes the first free slug of that shape, the beam's slug cut to fit the board's slug length.
// It carries the beam's labels, so the board's type-label rule holds, and a body the decision record carries as the goal of both the darn writer and the Code-Review perch: the writer runs `lym fabric merge-in`, settles the conflicts, makes the gate pass and changes nothing else, and the perch reviews the merge alone.
// Its `run_settings` turn the perch on: loom's `code_max_bounces` is the hub's resolved Code-Review budget, at least one round, and `code_hard_max_bounces` stays above it.
// The beam's dobby starts it as any member, so it runs at any time; pawl neither starts one nor waits for one.
// The orch runs it before `ready`, which refuses while the parent holds commits the beam lacks, and Finalize brings the parent in again at the landing.
//
// # start
//
// `start [slug] [--all]` brings a beam up through `orchcli.BeamUp`: its dobby, its runs strand and its orch.
// `resolveBeamTarget` first resolves the beam, before any reconcile:
// no slug names the acting worktree's beam, the prime or a marked beam pair; the slug `root` names the prime; any other slug names that beam from anywhere in the hub.
// A task pair, a worktree whose origin record carries no beam mark and a slug with no local beam pair are refused, the first two naming the beam the pair belongs to, its worktree path and `lym beam start` there, the last naming `lym beam create <slug>` from its parent worktree, or `lym adopt <slug>` when its task branch is on origin.
// The verb does not resolve through `battencli.ResolveBattenPrime`, which would refuse first inside a task pair with no path and inside a held beam with no way forward, leaving these refusals and `BeamUp`'s hold refusal unreachable.
// A slug and `--all` together are refused naming both ways forward.
//
// After the target resolves, `hubreconcile.Ensure` runs once over the first target's `hubgeom.ReconcileGeometry`, the one config reconcile after a binary change any beam verb makes, and its failure is the verb's error with the reconcile's own way forward.
// `BeamUp` then runs for each target.
// `--all` names the prime and every local beam this machine owns, from `dobbyengine.OwnedWorktrees` over `hubgeom.DobbyStart`, and `startBeams` runs `BeamUp` for each, the first with `Prepare` set so the hub's origin reconcile, failed-push read, prime claim and owner pauses run once.
// The envelope holds one entry per beam: the dobby's outcome and detail, the orch's action, prompt source and abandoned session, the warnings and the error, and for the prepared beam the reconcile, failed pushes, owner pauses and mutations.
// A beam that fails to come up records its error on its entry, the others still start, and the verb exits non-zero naming each.
// Every `BeamUp` refusal, an owner's another machine or a hold of a run other than the beam's own landing among them, applies to the beam it acts on.
// A beam held by its own pawl run's landing, a halted Finalize for one, gets its orch back and its dobby stays down; `lym beam start` never starts the dobby of a held beam.
// The verb is idempotent.
//
// # stop
//
// `stop [slug]` resolves its target as `start` does and calls `orchcli.BeamDown`, which removes the orch strand and ends the beam's dobby through `runproc.EndRole`, each only when live.
// It prints what it stopped and takes no `--all`.
//
// # status
//
// `status` prints, once, the dobby section a beam's window draws, for the prime and for each local beam `hubgeom.DobbyStart` lists, the prime's first.
// Each entry carries the worktree's label, the structured `dobbyengine.View` that `battencli.DobbyViewReader` reads, and the lines `battencli.DobbyLines` renders from it with each run's cells from `battencli.DobbyRunCells`.
// It also carries the beam's orch, `orchView`: whether the orch strand is live (`orchcli.OrchLive`), whether its watcher is live (`orchengine.WatcherLive` over `orchcli.PrimePaths`) and the session id its state records.
// The envelope also carries `adopt_checks`, one `adoptcli.CheckView` per subject directory of the hub's adopt scratch directory that holds a record, sorted by subject, as `adoptcli.ReadCheckViews` reads them: the subject, its adopt check's state, the tier in flight, the tiers done and the paths of its log and output, absent when the hub holds no record of one, and a read error joins the returned error like a dobby's.
// A view that does not read carries its error in its entry, the others still read, and the verb fails naming each.
// It starts and changes nothing; its one write is the empty orch directory the watcher probe creates.
//
// # dobby
//
// `dobby <worktree root>` is hidden and internal: `dobbyengine.SpawnDetached` launches it, with the hub root as its working directory.
// Before it starts anything it reaps every child it already has through `proc.ReapInheritedChildren`: a dobby re-exec'd onto a new binary keeps the batten passes its old image started, with no wait left on them.
// It resolves the prime and runs `dobbyengine.Run` over `hubDobbyDeps` under a SIGINT/SIGTERM context.
// `registerDobby` runs only once `Run` holds the lock, through `Deps.Locked`: it writes `runproc.SelfRecord(runproc.RoleDobby)` under the beam's slug, or `runproc.HubOwner` for the prime, so the pair teardown of a landed beam ends that beam's dobby.
// It prints `already_running` when the lock stays held past the lock wait, with nothing registered.
//
// Each exit it takes before `Run` leaves a trace, through `runDobbyWith`, which takes the wiring and the notice as seams:
//   - A refusal of a worktree that resolved appends `start refused: <error>` to that worktree's `dobby.log`, where its `.lym/beam` directory exists, since no append creates one; a refusal before the worktree resolved keeps only its envelope.
//   - A wiring failure appends `start failed: wire the dobby over the hub: <error>` to the beam's `dobby.log`.
//   - Either of them in a process that reads as a re-exec of the dobby there, by `dobbyengine.ReadsAsReexec` over the state record, also posts a notice naming the cause and `lym beam start`; the wiring failure posts none when the worktree no longer resolves.
//
// Where `selfexec` execs, off Windows and outside a test binary, the process moves its standard output and error onto `dobby.out` through `reopenStdio` before anything else writes, so a dobby whose descriptors an older binary pointed at `dobby.log` leaves that file to its own lines.
// The holder of the dobby lock also rotates `dobby.out` once it holds the lock, and moves its output onto the fresh file; a process that loses the lock renames nothing.
//
// A path that is neither the prime nor a local pair whose origin record carries the beam mark, read as `hubgeom.DobbyStart` reads it, is refused naming `lym beam start`, before anything is registered or locked.
// `hubgeom.DobbyGeometry` alone places any worktree with a directory, a task pair's included, so a hand invocation can do no more than `lym beam start` does: a second invocation exits on the held lock, and one on a worktree another machine owns exits on the loop's owner read.
//
// The dobby's settle seam, `Deps.SettleFronts`, is `fronttask.Settle` over the worktree's front tasks, filled by `fronttask.HubSettleDeps` for the beam's slug, empty for the prime.
//
// A beam's dobby also gets `Deps.PrimeTidyShuttle`, a `runnerTidyShuttle` over the prime's location, with which it stops its board-tidy agent when its worktree is gone; the removed worktree's own config no longer exists, and the prime's dobby is given none.
// `hubDobbyDeps` still runs before the dobby holds its lock, so a fresh dobby is unregistered from its spawn until it does, at most the wiring time plus `lock.ProbeGrace`, and a beam dobby spawned inside a teardown of its own beam escapes that teardown; the worktree-gone exit ends it.
//
// Bound: `runproc.End` of the beam's own slug, from a cancel or an owner pause of the beam's landing run, also ends its dobby; by then `BeamLandRefusal` has left the beam no members, so it drives nothing, and `lym beam start` brings it back if the beam stays.
//
// # The dobby's seams
//
// `hubDobbyDeps` fills every seam of the engine from told paths, the dobby's working directory being the hub root and never a worktree:
//   - the local batten status files of the prime and the board's tasks through `battencli.DobbyRuns`, `battencli.DobbyLandings`, which reads the landings of this machine's name only, and `battencli.DobbyTasks`, which a beam's window reads too, and each run's parent through `hubgeom.ParentOf`;
//   - `battencli.DobbyRuns` also marks each run `NoSlot` through the run's board entry, so a pawl run without its landing hold takes no slot under `max_runs`;
//   - `battencli.DobbyRuns` also reads each run's place against its Land row through `battenrecipe.LandOutcomeOf`, with the landing commit through `shedrun.ReadLandedCommit`, and for a paused run whether its work already reached its parent by the judgment `lym batten cancel` refuses on, and `ClearNotifyOnLand` clears a landed entry's request through `boardengine`;
//   - the landing step of a beam's dobby: `CanLand` through `beamLandReading`, where the beam's ready record says whether it is flagged ready, `battencli.ReadBeamLandBlockers` whether nothing works under it, and the parent through `hubgeom.ParentOf` with `Owners.Effective` and `Owners.AdoptSubject` says which other machine owns it and what `lym adopt` takes, and `Land`, the `land` sequence of `lym beam land` itself; the prime's dobby is given neither;
//   - the start hold of a beam's dobby: `BeamReady`, the beam's ready record read through the same `hubLandDeps(prime).Ready` as `CanLand`, under which the dobby starts no member; the prime's dobby is given none;
//   - the member landing announcement of a beam's dobby: `LandingPullRequest`, the landing record's pull request announce head read through `pawlengine.ReadLanding`, and `BeamLandingsSince`, the beam pair's HEAD with the landings of `fabricengine.BeamLineCommits` past a head, records-only commits dropped and a head not on the line listing the whole line; the prime's dobby is given neither;
//   - the board and the owners through `boardengine`, and the pause record through `pauserecord`;
//   - the file watch through `internal/fswatch`, one watcher per directory merged into the loop's one channel;
//   - the pass spawn of `lym batten run`, started in the prime's anchor;
//   - the owner pause and the parent lock release through `tendercli` and `battencli`;
//   - the notice through `orchcli.NotifyFromScoped`, signed `ly:DOBBY` for the prime and `ly:<slug>:DOBBY` for a beam;
//   - the wait reminders of every dobby through `waitReminderDeps`: the rows PR-Review and Sampler-Session of `loomshed`, the latest resume through `pauserecord`, the presence and `remind_after_min` through `slackengine`, and the post as creel mail to `<shortname>:SLACK`, signed as the notice is;
//   - `beam.yaml` through `dobbyengine.LoadConfig`, and the binary check through `internal/selfexec`;
//   - each log line appended to `dobby.log` in the worktree's `.lym/beam/` directory by its own open, through `dobbyengine.AppendLog`, while the process's raw standard output and error, which `dobbyengine.SpawnDetached` points at `dobby.out` there, go to that other file;
//   - the board-tidy agent's shuttle runner from the worktree's own reed geometry, its model `sonnet[medium]` resolved through the model registry, and the stencils directory;
//   - for the prime's dobby alone, the hub's hibernate request and Slack relay: `Deps.Tick` runs the tick of the `hibernateengine.Carrier` that `hibernatecli.NewDobbyCarrier` builds over the prime, which takes the pending request up and carries it in a goroutine of its own and never reports a run in flight, and then `slackengine.EnsureRelayWhileAway`, which brings back a relay that died while the presence is slack, starts nothing while it is terminal or the hub has no Slack setup, and logs a start that gave up at Warn once per episode without failing the tick, the binary check is wrapped by `holdReexec`, which skips it while `Carrier.HoldsReexec` reports a sequence past its trigger wait, until it has sent its notice, and the watch also covers the request file at `hibernatecli.RequestPath`, so a written request wakes the tick that takes it up.
//     No beam's dobby carries or watches the request or starts the relay.
//
// Bound: `Deps.Tick` runs after the tick's other steps, so while a tick fails before it no pending request is taken up and no relay is started; a sequence already in flight runs on.
// The relay start runs after the carrier's tick on the same tick, and each failure of the two is joined into the tick's error.
package beamcli
