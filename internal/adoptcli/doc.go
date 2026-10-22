// Package adoptcli is the adopt module: `lyx adopt <slug>...`, `lyx adopt --prime [<slug>...]` and `lyx adopt --all`, with `--keep-local` and `--drop-local`, the takeover of tasks or the prime from the other machine of the operator.
// It only calls each owning module's takeover function in-process, so no module learns another's state, and it has no engine package of its own.
//
// # The verb
//
// It runs from the hub's prime only, by `battencli.RefuseUnlessPrime`.
// `request` holds the slugs, `Prime`, `All`, `KeepLocal` and `DropLocal`; `adoptDeps` holds every step's seam, which `hubDeps` fills over the hub and `TestAdopt` fakes.
//
// Before anything runs, `adopt` reads the local Bolt and refuses an unknown slug (naming `lyx board list`), a note (naming `lyx board promote`), a done task (naming that it already landed, and `lyx resume <slug>` to let a run that landed there go on to Land under its owner pause), `--keep-local` with `--drop-local`, and no slug without `--prime` or `--all`.
// `--all` names the prime and every task that is not done and has an owner or a run status, passing over a done task.
//
// # Step 1: the origin reconcile
//
// A moving hub-scope reconcile runs first.
// Its `pairteardown` seams take a `HubPaused` from `hubOnlyPaused`, which covers a slug only by a live hub entry, so a pair whose strands are alive under any slug-scope pause, an owner pause included, reads live.
// An unreachable origin refuses naming a re-run once origin answers, a reconcile error refuses naming it and the re-run, and a stop on Bolt or the prime's records branch refuses with the report and that branch's way forward; nothing below runs then.
//
// A named slug the report reads owned_elsewhere stops before its owner write when a branch of it was judged `needs_replay`, `ahead`, `rewound`, `rewritten` or `diverged`, or when adopt's own `LivePair` call, made for every such slug with a local pair, reads it live.
// The slug's envelope entry names each branch and its state and the two ways forward, `lyx adopt <slug> --keep-local` and `lyx adopt <slug> --drop-local`, and the other slugs go on.
// `--keep-local` records the owner over those local commits and over a live pair.
// `--drop-local` refuses a live pair naming `--keep-local`, and otherwise calls `fabricengine.DropLocal`, which keeps each local tip origin lacks under an `archive/<slug>/<tip>` tag before it moves the branch, and records into adopt's mutation record.
//
// # Step 2: the pair
//
// Through `Topology.Add` from the prime, adopt adds the pair of each named task that holds an owner or a run status and that this machine has no pair for.
// A refusal stops that slug and the other slugs go on.
// It carries fabric's own message and way forward, bar `*fabricengine.ErrBranchExists`, the leftover local branch's: dropping that branch would lose this machine's unpushed commits with no archive, so adopt rewords it to name tagging the leftover tip under `archive/<slug>/<tip>`, then `git branch -D <branch>` and a re-run of `lyx adopt <slug>`.
//
// # Step 3: the owner write
//
// One `boardengine.SetOwners` call with `Takeover` records this machine as the owner of every named entry the earlier steps did not stop, and of the prime when it is named or when the board records no prime owner.
// A refused push refuses the verb, resumes nothing and names its class's way forward and then a re-run of `lyx adopt` with the same arguments.
// A slug the re-read shows owned by another machine stops, naming that owner and that a re-run of `lyx adopt <slug>` takes it over again, and starts no run.
//
// # Steps 4 and 5: the pause and the run
//
// Step 4 lifts, through `tenderengine.Lift`, each live entry carrying `pauserecord.OwnerCaller` whose scope is an adopted slug, by its scope, caller and pause time.
// A slug still covered by another entry gets an envelope line naming it and `lyx resume <slug>` for an entry of its own scope or `lyx resume` for a hub entry.
//
// Step 5 resumes each adopted task whose batten run record is not done through `battencli.ResumeInWindow`, so the run's own driver lock refuses a second driver.
// A resume failure is the slug's error and the other slugs go on; a task with no run record gets its owner and no run.
//
// A child halted at Finalize's owner halt, read as a blocked child status whose error opens with `landingshed.OwnerHaltPrefix`, is resumed with `lyx loom resume` spawned in its task worktree, the spawn logged through `logger`.
// That holds both when the child's own slug is adopted and when `--prime` or a parent slug adopts its parent while this machine owns the child's run; a child halted for any other reason stays halted.
//
// # The envelope
//
// It carries `reconcile`, `mutations` and `partial`, and per slug what happened: a stop with its branches, pair added, owner recorded, pause lifted or standing, run resumed or none, the error, and a warning when this machine does not own the slug's parent, since that run halts at Finalize here.
//
// # The bounds
//
// Adopt writes owners without the old owner's help and stops no driver live elsewhere, which pauses at its next owner check.
// Under a hub pause `LivePair` reads not live, so `--drop-local` can move a branch under interrupted strands, as any hub-paused pair's reconcile does today.
// A named slug whose pair has no batten run record never reads owned_elsewhere, since the reconcile judges only recorded pairs, so its local commits do not stop it before the owner write.
// Whether origin holds the task's branches is read from the board, not from origin: an owner or a run status marks a task that was started, and the add then adopts its live pair.
package adoptcli
