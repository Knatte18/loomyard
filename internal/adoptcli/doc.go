// Package adoptcli is the adopt module: `lyx adopt <beam>...`, `lyx adopt root` and `lyx adopt --all`, with `--keep-local` and `--drop-local`, the takeover of beam projects, or of root, and of the runs under them from the other machine of the operator.
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
// A nested beam keeps its own owner: `lyx adopt root` moves root's tasks and the tasks of the grouping projects below root, never a beam's members, whose parent is that beam.
//
// Before anything runs, `namedEntries` reads the local Bolt and refuses an unknown slug (naming `lyx board list`), a note (naming `lyx board promote`), a done entry (naming that it already landed, and `lyx tender resume <slug>` to let a run that landed there go on to Land under its owner pause), a task (naming `lyx adopt <beam>` or `lyx adopt root`, the `AdoptSubject` that moves it), a grouping project (naming the same adopt to move its tasks and `lyx beam create <slug>` to make it a beam), `--keep-local` with `--drop-local`, and no argument without `--all`.
//
// `movedRuns` lists, sorted, every task and beam project that is not done, holds a batten run record that is not done, and is a child of an adopted beam, or of root when root is adopted, plus each adopted beam's own pawl run.
// A beam below an adopted beam or root is listed too, and keeps its own owner.
//
// It then asks `adoptDeps.MoveInProgress`, filled from `fabricengine.MoveInProgress`, for the prime and for the own worktree of each adopted beam and moved run this machine holds a pair for and each one's parent worktree, and refuses with an empty outcome when an anchor change stands in any of them.
// The refusal carries the record's way forward, a change in progress to wait for or `lyx fabric anchor <worktree> --set <anchor>` to finish an interrupted one, and then the same `lyx adopt` again, so no pair is added, no run resumed and no gate suite started under a moving anchor.
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
// The slug's envelope entry names each branch and its state and the two ways forward, `lyx adopt <project> --keep-local` and `lyx adopt <project> --drop-local`, where `<project>` is the adopted beam or `root` that moves the slug and never a task.
// `wouldStop` answers that from the read-only part of step 1, before any slug is touched, and a project one of whose entries stops is held back whole: no owner is written for it and no other entry under it is taken over, so the named adopt answers the stop before it moves anything.
// The other projects go on.
// `--keep-local` records the owner over those local commits and over a live pair.
// `--drop-local` refuses a live pair naming `--keep-local`, and otherwise calls `fabricengine.DropLocal`, which keeps each local tip origin lacks under an `archive/<slug>/<tip>` tag before it moves the branch, and records into adopt's mutation record; when the pair's anchor differs from origin's, its refusal ends with re-running the same `lyx adopt` under `--drop-local`, since the pair may be a moved run that no adopt names.
//
// A beam that is a member of an adopted beam, or of root, and records another machine as its owner keeps it: it is reported on its own outcome as `owned elsewhere` with `owned_by`, and neither taken over nor resumed.
//
// # Step 2: the pair
//
// Through `Topology.Add` from the prime, adopt adds the pair of each adopted beam or moved run that holds a run status or is an owned beam, and that this machine has no pair for.
// Add adopts the live branches and their origin record, the beam mark included.
// A refusal stops that slug and the other slugs go on.
// It carries fabric's own message and way forward, bar `*fabricengine.ErrBranchExists`, the leftover local branch's: dropping that branch would lose this machine's unpushed commits with no archive, so adopt rewords it to name tagging the leftover tip under `archive/<slug>/<tip>`, then `git branch -D <branch>` and a re-run of the adopt that moved the slug.
//
// # Step 3: the owner write
//
// One `boardengine.SetOwners` call with `Takeover` records this machine as the owner of every adopted beam the earlier steps did not stop, and of the prime when `root` is adopted or when the board records no prime owner, and not while a stop holds root back.
// A refused push refuses the verb, resumes nothing and names its class's way forward and then a re-run of `lyx adopt` with the same arguments.
// A beam the re-read shows owned by another machine stops, naming that owner and that a re-run of `lyx adopt <beam>` takes it over again, and starts no run.
//
// # Steps 4 and 5: the pause and the run
//
// Step 4 lifts, through `tenderengine.Lift`, each live entry carrying `pauserecord.OwnerCaller` whose scope is an adopted beam or a moved run, by its scope, caller and pause time, whenever the entry's effective owner, read from the re-read board, is this machine.
// A slug still covered by another entry gets an envelope line naming it and `lyx tender resume <slug>` for an entry of its own scope or `lyx tender resume` for a hub entry.
//
// Step 5 resumes each moved run whose batten run record is not done, and whose entry reads as this machine's, as a detached first pass through `battencli.StartFirstPass`, driven on by the parent's dobby, so the run's own driver lock refuses a second driver.
// A beam's pawl run is resumed only when this machine also owns the beam's parent, whichever of the two the adopt named, so adopting root then a beam and a beam then root both leave its pawl run resumed here.
// A run whose entry reads owned by another machine, such as a nested beam's pawl run under an adopted root, is not resumed and is reported on its own outcome as `owned elsewhere` with its `owned_by`, never as an error.
// A resume failure is the slug's error and the other slugs go on; a beam with no run record gets its owner and no run.
//
// Bound: until the same machine owns a beam and its parent, the beam's pawl run gets no batten pass here, reported owned elsewhere; `lyx adopt <beam>` on the parent's owner, or `lyx adopt <parent>` on the beam's owner, resumes it.
//
// After the resumes, `BeamUp` runs the beam steps of the prime when root is adopted, then of each adopted beam whose owner write stood, through `orchcli.BeamUp` with `Prepare` false, since the adopt's own first step already reconciled origin: the beam's dobby, its beam window and its orch, so the pair, the dobby and the orch move with the project.
// A beam whose pawl run holds the landing hold, read through `LandingHeld` over `boardengine.RunHolds` since `ReadOwners` carries no run status, gets no beam steps: its owner is still written, and its `way_forward` names `lyx batten status <slug>` and, when this machine does not also own the beam's parent, `lyx adopt <parent subject>`, `lyx adopt root` for the prime, which resumes the landing here.
// A nil `LandingHeld` reads no hold, and a failed read is the slug's warning naming `lyx beam start <slug>`.
// A failed `BeamUp` is the slug's own warning naming `lyx beam start <slug>`, `prime_beam_warning` naming `lyx beam start root` for the prime, and the other worktrees go on; what each step did is the slug's `beam_steps` and the envelope's `prime_beam_steps`.
//
// # Step 6: the test suite
//
// Machines differ in OS and toolchain, so code landed on one can break on another.
// After the beam steps, a call that recorded this machine as the owner of an entry or of the prime starts `lyx gate test --gate finalize --wait` once, detached, in the prime, the spawn logged through `logger`.
// One call is one adoption session: adopting the prime and two beams together starts one suite, and a tier the gate's green record holds green for the prime's tree is carried and runs nothing.
// The suite's output goes to `suite.log` under `fabricengine.AdoptScratchDir` in the hub's board-level `.lyx` directory, truncated at each start, and the envelope's `suite` names its pid and that log.
// A red suite only reports there, since the owner write and the resumes have already taken effect.
// A spawn that fails is the envelope's `suite_warning`, naming `lyx gate test --gate finalize` in the prime, and changes no slug's outcome.
// Bound: two separate adopt calls each start a suite, which the gate slots serialize.
//
// A child halted at Finalize's owner halt, read as a blocked child status whose error opens with `landingshed.OwnerHaltPrefix`, is resumed with `lyx loom resume` spawned in its task worktree, the spawn logged through `logger`.
// That holds when `root`, or the beam above it, is adopted while this machine owns the child's run; a child halted for any other reason stays halted.
//
// # The envelope
//
// It carries `reconcile`, `mutations` and `partial`, `prime_beam_steps` and `prime_beam_warning` for the prime's beam steps, `suite` or `suite_warning` for the test suite, and per slug what happened: a stop with its branches, pair added, owner recorded, pause lifted or standing, run resumed, none or owned elsewhere by `owned_by`, the `beam_steps` run, the error, and a warning when this machine does not own the slug's parent, since that run halts at Finalize here; the warning names `lyx adopt <project of the parent>`, `lyx adopt root` for the prime.
//
// # The bounds
//
// Adopt writes owners without the old owner's help and stops no driver live elsewhere, which pauses at its next owner check.
// Under a hub pause `LivePair` reads not live, so `--drop-local` can move a branch under interrupted strands, as any hub-paused pair's reconcile does today.
// A slug whose pair has no batten run record never reads owned_elsewhere, since the reconcile judges only recorded pairs, so its local commits do not stop it before the owner write.
// Whether origin holds an entry's branches is read from the board, not from origin: a run status, or an owner on a beam, marks an entry that was started, and the add then adopts its live pair.
package adoptcli
