// Package beamcli is the beam module: `lym beam create <slug>`, the making of a beam from a board project entry, `lym beam land <slug>`, the landing of a beam into its parent, `lym beam start [slug] [--all]`, the bring-up of a beam's dobby and orch, `lym beam stop [slug]`, their bring-down, `lym beam status`, the view of every local dobby and orch, and the hidden `lym beam dobby`, the dobby process itself.
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
//     A first pass whose envelope is not ok, such as one a covering pause refused, is the verb's error carrying the envelope's error and leaves no run.
//
// The verb makes, marks and pushes no pair itself: the run's Worktree-Create makes the pair from the parent beam, unmarked, at the anchor the run's seed records, and its Beam-Up marks it and starts the beam's dobby and orch.
// `--anchor <dir>` is recorded in the seed's `anchor` param on the first pass only, so it rides in the run record and a resume keeps it.
// A first pass a covering pause refused leaves no run, and the same `lym beam create <slug> [--anchor <dir>]` after the lift starts it; a flagged beam left by a first pass that died takes the same `--anchor` again, since its anchor reached no run record.
// Dobby never starts a beam project, so only this verb, a hand pass or `lym adopt` starts or resumes its run.
//
// Writing the beam flag first makes a member started mid-create stop at Worktree-Create and resume into the beam rather than fork from the prime for good.
// Members already spawned under the grouping project keep the prime as their parent.
// The envelope names `lym batten status <slug>` as the way to follow the run.
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
//   - `battencli.BeamLandRefusal`.
//
// The request is only a file: Beam-Await accepts only one naming its own run, writes the landing hold first, stops the dobby and re-runs every check above but the run-at-Beam-Await one before anything lands.
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
// Every `BeamUp` refusal, an owner's another machine or a landing hold among them, applies to the beam it acts on.
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
// The envelope also carries `adopt_check`, the `adoptcli.CheckView` that `adoptcli.ReadCheckView` reads from the hub's adopt scratch directory: the adopt check's state, the tier in flight, the tiers done and the paths of its log and output, absent when the hub holds no record of one, and a read error joins the returned error like a dobby's.
// A view that does not read carries its error in its entry, the others still read, and the verb fails naming each.
// It starts and changes nothing; its one write is the empty orch directory the watcher probe creates.
//
// # dobby
//
// `dobby <worktree root>` is hidden and internal: `dobbyengine.SpawnDetached` launches it, with the hub root as its working directory.
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
//   - the "can land" reading of a beam's dobby, `CanLand`, through `beamLandReading`: `battencli.ReadBeamLandBlockers` says whether nothing works under the beam, and the parent through `hubgeom.ParentOf` with `Owners.Effective` and `Owners.AdoptSubject` says which other machine owns it and what `lym adopt` takes; the prime's dobby is given none;
//   - the board and the owners through `boardengine`, and the pause record through `pauserecord`;
//   - the file watch through `internal/fswatch`, one watcher per directory merged into the loop's one channel;
//   - the pass spawn of `lym batten run`, started in the prime's anchor;
//   - the owner pause and the parent lock release through `tendercli` and `battencli`;
//   - the notice through `orchcli.NotifyFromScoped`, signed `ly:DOBBY` for the prime and `ly:<slug>:DOBBY` for a beam;
//   - `beam.yaml` through `dobbyengine.LoadConfig`, and the binary check through `internal/selfexec`;
//   - each log line appended to `dobby.log` in the worktree's `.lym/beam/` directory by its own open, through `dobbyengine.AppendLog`, while the process's raw standard output and error, which `dobbyengine.SpawnDetached` points at `dobby.out` there, go to that other file;
//   - the board-tidy agent's shuttle runner from the worktree's own reed geometry, its model `sonnet[medium]` resolved through the model registry, and the stencils directory.
package beamcli
