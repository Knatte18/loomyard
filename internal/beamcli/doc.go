// Package beamcli is the beam module, whose verbs are:
//   - `lym beam create <slug>` makes a beam from a board project entry;
//   - `lym beam describe <slug>` records the change description the beam's orch wrote;
//   - `lym beam pr open <slug>`, `update`, `status` and `close` open, update, read and close its pull request;
//   - `lym beam ready <slug>` flags that a beam is ready to merge;
//   - `lym beam land <slug>` lands a beam into its parent;
//   - `lym beam merge-parent <slug>` files a task that merges a beam's parent branch into it;
//   - `lym beam start [slug] [--all]` brings up a beam's dobby and orch;
//   - `lym beam stop [slug] [--all]` brings them down, and with `--all` the whole hub on this machine;
//   - `lym beam status` shows every local dobby and orch;
//   - the hidden `lym beam dobby` is the dobby process itself.
//
// The verbs only call each owning module's function in-process, so no module learns another's state.
// The daemon's engine is `internal/dobbyengine`, and this package wires its seams over the hub.
// The module is hub-only, so it declares no `cliwire.Module`.
// Every verb but `start` and `stop` resolves its location through `battencli.ResolveBattenPrime`,
// so it runs from the hub's prime and from any beam, and acts on the prime's location.
//
// # create
//
// `create <slug>` takes an existing project entry that is not done.
// Its `beam` is false, or true with an owner that is this machine or none.
// Its parent must not read as owned by another machine.
// It first runs from the beam's parent worktree only: the prime, or the parent beam `boardengine.ParentBeam` names.
// It refuses, each naming its way forward:
//   - an absent entry, a note or a task;
//   - a done project;
//   - a beam another machine owns, naming `lym adopt <slug>`;
//   - a parent another machine owns, naming `lym adopt` of the nearest beam above it, else `root`;
//   - a run from any worktree but the parent's, naming that worktree and the same command there.
//
// The steps skip what is already there, so a re-run finishes a stopped create:
// the root claim, the beam's owner record, then the beam's batten run as a detached first pass that records the anchor.
// A run in flight is reported live and starts nothing.
// A done run whose pair is gone is cleared by `battencli.ClearMotifRun` when its child ran the motif or arras recipe;
// any other done run is refused, naming `lym batten cleanup --apply`.
// The verb makes, marks and pushes no pair itself: the run's Worktree-Create, Record-Write and Beam-Up do.
// `--anchor <dir>` is recorded in the run's seed on the first pass only, and a resume keeps it.
// Dobby never starts a beam project; only this verb or a hand pass starts or resumes its run.
// The beam flag is written first so a member started mid-create resumes into the beam instead of forking from the prime.
//
// # ready
//
// `ready <slug>` flags the beam ready to be merged into its parent, and the flag is final.
// It writes a `pawlengine.ReadyRecord` at `loomengine.BeamReadyPath` of the beam's pair.
// It then commits the landing directory.
// No verb removes the record; Finalize's landing, Beam-Down and the retired-row move end it.
// The flag does not say the project is finished.
// Once it stands and nothing works under the beam, the beam's dobby writes the land request itself.
// It runs anywhere in the hub and refuses what `land`'s entry check refuses.
// For a beam not yet flagged it refuses, in order, each naming its way forward:
//  1. a run not at Beam-Await;
//  2. open members and child pairs;
//  3. a parent that holds commits the beam lacks, naming `lym beam merge-parent <slug>`;
//  4. a missing or invalid change description, naming `lym beam describe <slug>`;
//  5. a pull request GitHub's rules for the parent branch demand and none recorded, naming `lym beam pr open <slug>`;
//  6. a recorded pull request that is closed, open at another head, unmergeable or merged without every beam commit.
//
// A beam whose `pull_request` is `trail` passes with none recorded, since Finalize opens its own.
// That value is found by the per-task lookup from the beam's own worktree up, as for any run.
// A beam with no commits beyond its parent also passes, since Finalize lands it with no pull request.
// The record names its beam because a pair forked from the beam inherits the beam's `_lym` records.
// The beam's orch or the operator runs it; dobby never does.
//
// # describe
//
// `describe <slug>` records the beam's change description and returns its path.
// The beam's orch writes the description, `summary.md` in the beam's landing directory.
// Under a recorded pull request the beam's dobby also rewrites it after each member landing:
// its describer agent writes a draft, and the same sequence records the draft in place of the description.
// A refused draft leaves the recorded description standing.
// It runs anywhere in the hub and refuses what `land`'s entry check refuses.
// It refuses a beam with no pawl run, a run not at Beam-Await, and a beam whose ready record stands.
// A run at Beam-Await passes in every state, so a halted run is served.
// A refused description stamps nothing, since validation precedes the commit.
// With a pull request recorded, `describe` then runs the `pr update` sequence so the pull request carries the text.
//
// `describe <slug> --draft` records nothing and writes a starting point at `summary.md`.
// It refuses when `summary.md` already stands, so an edited or recorded description is never overwritten.
// The draft ends with a Merge danger section whose placeholders the orch must replace before recording succeeds.
//
// # pr
//
// `pr open <slug>` and `pr update <slug>` are thin verbs over `internal/pullrequest`.
// They act only on the pull request from the beam's own branch into its parent.
// They open, edit and reopen it, never merge, and change nothing once the ready record stands.
// They read a token only through `internal/githubclient`.
// Each refuses, naming the way forward:
//   - an entry that is not an open beam with a pair here;
//   - an owner of another machine;
//   - a run not at Beam-Await;
//   - a stood ready record.
//
// `open` refuses:
//   - a beam with no change description;
//   - a beam with no commit beyond its parent;
//   - an origin that `landingshed.OriginMissing` finds a fact missing from;
//   - a beam GitHub could not merge as it stands: one with a merge task not done, or one whose head conflicts.
//
// A beam only behind its parent opens, since GitHub merges it.
// A recorded pull request that GitHub reports open answers `already`, and a closed one is reopened.
// A merged one changes nothing and names `lym beam ready <slug>`.
// With none recorded it pushes the beam branch and finds or creates the pull request from the description.
// It then records `Number`, `URL` and `PushedHead`.
// A record write that fails after a pull request stands is refused, and a re-run of `open` records it.
//
// `update` refuses a beam with no pull request recorded.
// Otherwise it pushes the branch and sets the title and body from the description.
//
// `status <slug>` is read-only and answers at any row up to Beam-Down.
//
// `close <slug>` refuses a beam with no pull request recorded.
// It closes an open recorded pull request with one comment and drops it from the landing record.
// A pull request GitHub reports closed or merged is only dropped.
// The orch closes a pull request only to land without one, never to add work.
//
// # land
//
// `land <slug>` asks the beam's pawl run to land the beam by writing a land request, and starts no run of its own.
// It runs anywhere in the hub, a task pair and the beam itself included.
// It resolves the prime from the cwd's worktree.
// It never uses `battencli.ResolveBattenPrime`, which refuses a worktree that is neither the prime nor a beam.
// The request is a `pawlengine.LandRequest` that Beam-Await consumes.
// The beam's pair is removed only at the run's teardown.
//
// It refuses, in order, each naming its way forward:
//   - an entry that is not an open beam project of this machine with a pair that carries the mark;
//   - a beam with no pawl run;
//   - a beam whose run is not running at Beam-Await;
//   - a beam this machine does not own, or whose parent another machine owns;
//   - a beam whose ready record does not flag it;
//   - `battencli.BeamLandRefusal`.
//
// The request is only a file.
// Beam-Await accepts only a request naming its own run.
// It writes the landing hold first, stops the dobby.
// It then re-runs every check above but the run-at-Beam-Await one before anything lands.
//
// `BeamLandRefusal` judges members and member pairs alone:
// any open entry whose `hubgeom.ParentOf` is the beam, through any depth of grouping projects,
// and any local pair whose origin record names the beam as its parent worktree.
// The beam's orch is no blocker, since it runs until Beam-Down.
//
// Batten's start guard applies the same refusal to a project entry, and Worktree-Create applies it again after its claim.
// The guard also refuses a run of a grouping project or of a beam with no local pair.
// A landing forks no pair, and Worktree-Create never calls `Add` for a project.
//
// Bound: a pair on another machine whose origin names the beam is not seen when its entry does not name the beam.
//
// # merge-parent
//
// `merge-parent <slug>` files the beam's merge task, a released darn task in the beam's project.
// Its run merges the beam's parent branch into the task's branch, and the task's landing brings that into the beam.
// It runs anywhere in the hub and refuses what `land`'s entry check refuses.
// It refuses a beam whose origin record names no parent branch, naming `lym fabric reconcile`,
// and a beam that holds an open merge task.
// A merge task is found by its project and its slug's `-merge-in` suffix.
// The new one takes the first such slug that no board entry, batten run directory or task branch holds.
// Its run_settings set beam's `pull_request` to `merge-in`, so it lands with no pull request by a true merge.
// The beam's dobby starts it as any member, and the orch runs it before `ready`, which refuses while the parent is ahead.
// Finalize brings the parent in again at the landing.
//
// # start
//
// `start [slug] [--all]` brings a beam up through `orchcli.BeamUp`: its dobby, its runs strand and its orch.
// No slug names the acting worktree's beam, the prime or a marked beam pair.
// The slug `root` names the prime, and any other slug names that beam from anywhere in the hub.
// A task pair, a worktree whose origin record carries no beam mark and a slug with no local beam pair are refused.
// The verb does not resolve through `battencli.ResolveBattenPrime`.
// That would make these refusals and `BeamUp`'s hold refusal unreachable.
// A slug and `--all` together are refused.
//
// `hubreconcile.Ensure` runs once over the first target.
// `--all` names the prime and every local beam this machine owns.
// A beam that fails to come up records its error on its entry, the others still start, and the verb exits non-zero.
// Every `BeamUp` refusal applies to the beam it acts on.
// Among them are an owner of another machine and a hold of a run other than the beam's own landing.
// A beam held by its own pawl run's landing gets its orch back, and its dobby stays down.
// The verb is idempotent.
//
// # stop
//
// `stop [slug]` resolves its target as `start` does and calls `orchcli.BeamDown`.
// `BeamDown` ends the beam's orch, its dobby and the orch session's provider process.
//
// `stop --all` takes the hub down on this machine, every worktree whoever owns it, and writes no tracked record.
// A slug beside `--all` is refused.
// The caller's process re-starts the verb detached through `selfexec`,
// so the end of the session it ran from does not cut it.
// The detached process goes past every failed step and names each in its report, which goes to lym's log.
// A hub pause entry already standing covers the stop and stays.
// It writes no pause intent, and a Bolt hold stays as it stands.
// Bound: the stop ends agent steps mid-step; a run's transitions are pushed or owed,
// so the cut step runs again once `lym beam start --all` or `lym s` brings the hub back.
// A stop that dies midway leaves its pause entry, which `lym tender resume` lifts.
//
// # status
//
// `status` prints, once, the dobby section a beam's window draws.
// It does so for the prime and each local beam `hubgeom.DobbyStart` lists.
// The envelope also carries `adopt_checks` and `adopt`, each absent when the hub holds no such record.
// A view that does not read carries its error in its entry, the others still read, and the verb fails naming each.
// It starts and changes nothing.
//
// # dobby
//
// `dobby <worktree root>` is hidden and internal; `dobbyengine.SpawnDetached` launches it.
// It reaps every child it already has through `proc.ReapInheritedChildren`.
// A dobby re-exec'd onto a new binary keeps the batten passes its old image started.
// It runs `dobbyengine.Run` over `hubDobbyDeps` under a SIGINT/SIGTERM context.
// It registers its `runproc` record only once `Run` holds the lock,
// so the pair teardown of a landed beam can end that beam's dobby.
// It prints `already_running` when the lock stays held past the lock wait.
//
// A path that is neither the prime nor a local pair whose origin record carries the beam mark is refused.
// The refusal names `lym beam start` and comes before anything is registered or locked.
// A hand invocation therefore can do no more than `lym beam start` does.
//
// A fresh dobby is unregistered from its spawn until it holds its lock.
// A beam dobby spawned inside a teardown of its own beam escapes that teardown; the worktree-gone exit ends it.
// Bound: `runproc.End` of the beam's own slug also ends its dobby, as a cancel of the beam's landing run does.
// `lym beam start` brings it back if the beam stays.
//
// # The dobby's seams
//
// `hubDobbyDeps` fills every seam of the engine from told paths.
// The dobby's working directory is the hub root and never a worktree.
// Only the prime's dobby carries the hub's hibernate request and Slack relay.
// It also restarts the prime's pusher at the start of its `Tick`, the one hub-wide pusher restart.
// Only a beam's dobby is given the landing step, the start hold and the member flagging.
// Bound: a teardown in flight when the dobby stops ends half done;
// the next attempt or `lym cleanup --apply` finishes it.
// Bound: `Deps.Tick` runs after the tick's other steps.
// So while a tick fails before it, no pending request is taken up and no relay is started.
package beamcli
