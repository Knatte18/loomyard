// Package beamcli is the beam module: `lyx beam create <slug>`, the making of a beam from a board project entry, `lyx beam promote <slug>`, the turning of a spawned task into one, and `lyx beam land <slug>`, the landing of a beam into its parent.
// It only calls each owning module's function in-process, so no module learns another's state, and it has no engine package of its own.
//
// # The verbs
//
// Every verb resolves its location through `battencli.ResolveBattenPrime`, so it runs from the hub's prime and from any beam, and acts on the prime's location.
// The module is hub-only, so it declares no `cliwire.Module`.
// A verb's seams are a deps struct that `hubCreateDeps` fills over the hub and `TestCreate` fakes.
//
// # create
//
// `create <slug>` takes an existing project entry that is not done, whose `beam` is false, or whose `beam` is true and whose effective owner is this machine or none.
// The project's title, brief and labels are board writing, so the verb makes no entry.
// It refuses an absent entry or a note (naming `lyx board upsert` of a project entry), a task (naming `lyx beam promote`), a done project, and a beam another machine owns (naming the owner, `lyx beam create` there and `lyx adopt`).
//
// The steps run in order, each skipped when its result is already there, so a re-run finishes a stopped create:
//
//  1. `boardengine.SetOwners` records this machine as the owner with the beam flag.
//  2. `Topology.Add` makes the pair from the prime, or from the parent beam `boardengine.ParentBeam` names for a nested beam, with the beam mark in the same origin commit; an existing pair gets the mark through `fabricengine.MarkBeam`.
//     A parent beam whose pair is absent here is refused naming `lyx beam create <parent>`, or `lyx adopt <parent>` when its branch is on origin.
//  3. `fabricengine.SpawnPairPush` spawns the pair's push.
//  4. `fabricengine.RegenerateHubWorkspace` rewrites the workspace files.
//
// Writing the flag first makes a member started mid-create stop at Worktree-Create and resume into the beam rather than fork from the prime for good.
// Members already spawned under the grouping project keep the prime as their parent.
// The envelope names the beam's worktree and `lyx orch start` there as the way to give it an orch; the verb starts none.
//
// # promote
//
// `promote <slug>` turns a spawned task into a beam, keeping its pair and the commits on its branch.
// On a project whose `beam` is false it runs create's steps, and on a project whose `beam` is true and whose pair carries the mark it finishes the workspace step.
// It refuses, each with its way forward:
//   - a task never started, holding no owner and no run status, naming a new project entry through `lyx board upsert` and `lyx beam create` on it;
//   - a started task with no pair on this machine, and a task another machine owns, naming `lyx adopt`;
//   - a task whose run has passed Finalize's parent-side merge, naming `lyx batten run`, which the end of the run's own refusal says;
//   - a task an open task depends on, naming each dependent and `lyx board set-deps`;
//   - a task whose pair's origin record has an empty parent branch, a root pair made by `lyx fabric add --adopt-local` over the default branch, naming a new project entry and `lyx beam create`.
//
// The steps run in order, each skipped when its result is already there, so a re-run after any step finishes the promote:
//
//  1. `battencli.EndRunForPromote` ends the task's run processes and strands and keeps its pair, and removes the cancel marker when it ends; an unreachable origin makes it end-only, and promote stops there naming a re-run once origin answers, with no mark written.
//     The end repeats while the task's claim stands and its pair is unmarked, since ending an ended run changes nothing.
//  2. `fabricengine.MarkBeam` writes the beam mark into the pair's origin record in one records commit, and the pair's push is spawned.
//  3. `battencli.ClearRunForPromote` removes the run's records and clears its claim, the rest of a cancel without `--remove`.
//  4. `fabricengine.ResetRecordsToParent` resets the pair's records content to a fresh fork of its parent's, and the pair's push is spawned when it committed.
//     Its refusal of uncommitted content outside the record pathspec names the paths and a re-run; promote takes no force flag.
//  5. `boardengine.SetOwners` records this machine as the owner with `Beam` and `ToProject` in one Bolt write, turning the entry into a beam project and clearing its `depends_on`, `recipe`, `profile` and `run_settings`.
//  6. `fabricengine.RegenerateHubWorkspace` rewrites the workspace files.
//
// Bound: promote ends a running run for good; nothing of the loom run survives in the pair's checkouts but its code commits, and its discussion, plan and run records stay reachable in the records history behind step 4's commit, on the beam's records branch and every member's forked from it.
// A promote stopped before step 2 leaves the run claimed with its processes ended, as an interrupted cancel does: a resume would restart it, and a re-run of promote ends it again.
// Batten's start guard refuses a task whose pair carries the mark, naming `lyx beam promote`, so an entry a promote left an open unclaimed task is never started.
// A hand `lyx loom resume` inside the pair between steps 1 and 4, while its loom seed still stands, restarts the ended run, and a re-run of promote ends it again.
//
// # land
//
// `land <slug>` ends a beam with a short landing run over batten and the `beam` recipe.
// It runs from the prime or a beam, and refuses when the acting worktree is the beam itself, since the landing removes the pair, naming the beam's parent worktree as where to run it.
// It refuses an entry that is not a beam project of this machine with a pair that carries the mark, then applies `battencli.BeamLandRefusal`, then starts the landing with `battencli.ResumeInWindow`, whose window opens in the beam's parent worktree's session.
//
// `BeamLandRefusal` refuses, each with its way forward:
//   - any open entry whose `hubgeom.ParentOf` is the beam, through any depth of grouping projects, naming landing it, `lyx batten cancel <slug> --remove`, or, for an entry with no pair, moving its `project` off the beam;
//   - any local pair whose origin record names the beam as its parent worktree, naming `lyx batten run` and `lyx batten cancel --remove`;
//   - the beam's orch strand live, or its watcher live, naming `lyx orch stop` in the beam's worktree and a re-run once `lyx orch status` there reports the watcher gone.
//
// Batten's start guard applies the same refusal to a project entry, and Worktree-Create applies it again after its claim, so a resume or a direct `lyx batten run` cannot skip it and a member added before the claim is caught.
// The guard also refuses a run of any project entry before the claim unless it is a landable beam: a grouping project, naming `lyx beam create`, and a beam with no local pair, naming `lyx beam create` and `lyx adopt`.
// A marked beam pair passes the guard's dirty-source and left-pair checks, so the landing reaches Worktree-Create, which finds the pair complete and skips; a landing forks no pair, and Worktree-Create never calls `Add` for a project.
//
// Bound: a pair on another machine whose origin names the beam is not seen when its entry no longer names the beam, as for a pair made by hand there with `--parent <beam>` and no board entry.
package beamcli
