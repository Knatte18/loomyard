# Re-parenting a running task: what a verb has to write

2026-11-12, board task `reparent-running-task`.
A read-only study of the code at the commit this branch forked from; nothing was run on a live task and no code changed.
Source for [issue #48](https://github.com/Knatte18/loomyard/issues/48).
A claim marked *not read* names code the study did not open, so the verb's own plan has to settle it.

## Short answer

The operator's idea holds: pause the task, move every pointer, resume.
The parent of a task is not one value.
It is written in two durable places, the board entry's `project` and the pair's `_lym/fabric/origin.json`, and copied into a few run records.
Everything else (members of a beam, the dobby's view, the parent lock, the orch a session reports to) is derived from those two at read time.
Two stores written by two modules means no atomic change, so the verb must be idempotent and ordered so that a crash between the writes is harmless.
The hard part is git, not bookkeeping: the new parent has to hold the commits the task started from.
This study recommends refusing a re-parent when it does not.

## 1. Where a parent or a membership is recorded

"Writer" is the code that writes the value, "Readers" the code that decides on it.
Reader lists name the load-bearing ones; a rename search on the symbol finds the rest.

| # | Place | Holds | Writer | Readers |
|---|---|---|---|---|
| 1 | Board entry, `boardengine.Task.Project` (`internal/boardengine/task.go`) | the entry's project, hence its beam | `lym board upsert` through the store's write path | `boardengine.ParentBeam`; `battencli.beamForkParent`; `battencli.beamMembers`; `loomcli.projectRecordPaths`; dobby `opensReadyBeam` (`task.Project == slug`) |
| 2 | Project entry, `Task.Order` | member slugs in plan order | board upsert of the project | not read by the study; a verb has to treat it as a second place the slug sits |
| 3 | Origin record, `fabricengine.Origin` in `_lym/fabric/origin.json` of the task's weft worktree | `ParentBranch`, `ParentWorktree`, `Beam` | `Topology.Add` step 10c (`add.go`) through `WriteOrigin` and `CommitRecordsPaths`, commit subject `fabric: record parent branch for <slug>`; `MarkBeam`; the legacy repair of `lym loom start --parent` (`sharedbootstrap.go`) | `ReadOrigin`, `ReadOriginFor`; `hubgeom.ParentOf`; `hubgeom` parent agent name; `loomcli.landingBranches`; `battenshed` land seam; `fabricengine.parentHoldsTaskWork`; `ReconcileOrigin`'s run scope; `beamcli` pull-request mode and child-pair blockers; `hubworkspace.go`; `pairteardown` |
| 4 | Warp and weft branch ancestry | the commit the task forked from; the weft branch forks from the parent's records branch | `Topology.Add` (`worktree add -b <branch> <path> <parent head>`; `createWeftWorktreeDroppingRuns`) | `MergeBase`, `HoldsWork`, `HeadHoldsCommitsNotIn`, every landing row |
| 5 | Loom status product, `loomengine.Status.Parent` in the run's status file | the parent branch the run was seeded with | `loomshed.Seed`, once; it refuses to overwrite | `loomStatusExtras` (`parent` key of the status envelope), `EntryObservation.Parent` |
| 6 | Merge record, `landingshed.MergeRecord` in the run's scratch dir | `ParentBranch`, `MergedTip`, `TaskBase`, `AlignedTip` of the last merge-in | Parent-Align, Finalize | Finalize's `readParentMergeBefore` and `mergedTipRead`, which measure the parent side from the earlier record's merged tip |
| 7 | Parent lock, `parentlock` records | one record per parent branch, its owner a run | Finalize and the verify marker, through `parentGuard` | Parent-Align and Finalize queueing; `HeldBy(slug)` |
| 8 | In-memory `landingshed.Deps.ParentBranch` | the branch this process lands on | built at process start from the origin record (`landingBranches`) | every landing row of that process |
| 9 | Pull request of the task, on the old base | head task branch, base the old parent | Publish, PR-Review, Finalize | `ResolvePullRequest`, `pullrequest.Client.FindOpen(head, base)` |
| 10 | Pause record (`pauserecord`) | per-slug entries | `lym tender pause` and `lym adopt` | `batten run`'s `refuseCovered`, the parent lock queue, dobby |

Findings that shape the verb:

- **The batten run record names no parent.**
  `internal/battenshed` reaches the parent through seams (`ChildParentLock`, the land row's `ParentBranch` func) that read row 3.
  The issue's "the batten run and the fabric pair record the parent" is therefore one record (3) plus the loom status copy (5).
- **Row 3 is the durable truth and is written once.**
  `resolveParentBranch` refuses a `--parent` that disagrees with the record and says "there is deliberately no override flag".
  `Add` over an adopted weft worktree keeps an existing record's parent fields, so a resume or a pair re-made on another machine will not undo a re-parent.
- **Membership has no list of its own.**
  A beam's members are derived: open entries whose `ParentOf` is the beam, through grouping projects, plus local pairs whose origin names the beam as `ParentWorktree` (`ReadBeamLandBlockers`).
  `ParentOf` reads row 3 first and falls back to row 1 only for a slug with no origin record.
  So rows 1 and 3 can disagree, and row 3 wins for landing, ownership and blockers, row 1 for project directives and the dobby's `task.Project` test.
- **Branch names do not encode the parent.**
  A pair is `<branch_prefix><slug>` and `<that>-weft`, so no branch is renamed.
- **The ownership rule follows the parent.**
  A task's effective owner machine is read through `Owners.Effective(slug, ParentOf)` (`PATTERN-owner-machine`), so a re-parent can change which machine may run it.
- **Live sessions keep stale text.**
  `lym creel post --parent` resolves at call time through row 3, so it follows the move.
  The prompt a running session was rendered with ("Your parent is `ly:orch`", the project directive) is fixed at spawn.
  The next spawn reads the new beams' records live (`projectRecordPaths`), not the copy the task inherited when it forked.

## 2. Pause, atomicity and half-done changes

| Place | Does a pause hold its writer still? | One verb, atomic with the others? | A half-done change leaves |
|---|---|---|---|
| 1 `project` | No. A pause is not consulted by board writes; the refusal is the run lock: `checkRunLocks` compares `Project` and `Order` in `scopeChanged`, and returns `RunLockedError`. | Not by `upsert`. Needs a narrow board write that the lock lets through for this one field. Board writes are atomic alone, under the board lock. | Board under the old beam while landing goes to the new parent: the new beam's blocker check does not see the task (row 3 wins), so the new beam can land first, and the task then lands into a parent that is not the one the board shows. |
| 2 `Order` | Same as 1. | Same write as 1; a project with a hold-free run (`holdFreeProducers`) is compared on `Project`, `Kind`, `Recipe` and `Beam` only, so `Order` is free there. | A slug listed under a project it no longer belongs to. *Consumer not read.* |
| 3 origin record | Nothing writes it after creation, so there is no writer to hold. Its commit and detached push are the only moving parts. | A write plus one commit on the weft branch (`CommitRecordsPaths`), under the weft write lock, then an async push. No cross-repo transaction exists (`Fabric.Commit`: "no rollback"). | Written but unpushed: the owner machine is right and any other machine is stale until the push lands; owner-machine rules make other machines read-only for a run, so this is tolerable. Not written: nothing moved. |
| 4 ancestry | Immutable; nothing to hold. | Not writable. The verb checks it. | Not applicable. |
| 5 status `Parent` | `Seed` runs once. A rewrite goes through `state.UpdateJSON` (`PATTERN-tolerant-records`). | A separate file; not atomic with 3. | A status envelope that reports the old parent. No reader found that lands on it. |
| 6 merge record | Its writers are Parent-Align and Finalize. A pause stops a new `batten run` pass (`refuseCovered`) but not a driver already running. The driver must have ended. | A delete of one scratch file. | A record naming the old parent: Finalize would measure the parent side from a tip of the wrong branch and report a wrong overlap. |
| 7 parent lock | Covered runs yield the queue, but a held lock is released under a pause only for `OwnerCaller` entries, so an adopt-style pause leaves it held. | Not rewritten; keyed by branch, so the new parent has its own record. | A stale record under the old branch until its owner process dies; a live holder means a landing is in flight and the verb must refuse. |
| 8 in-memory `ParentBranch` | A paused run's processes are gone after the driver ends; the next pass rebuilds `Deps` from row 3. | Nothing to write. | A driver that survived the pause lands on the old branch. |
| 9 pull request | No. Only PR-Review, Finalize, `lym beam pr` and pair teardown open, reopen, merge or close a pull request (`PATTERN-github-auth`). | Not by this verb. | An open PR on the old base next to a new one: Publish finds none for the new base (`FindOpen(head, base)`) and opens a second. |
| 10 pause entry | The pause is the tool. `AdoptCaller` entries lift only by hand; `OwnerCaller` entries lift by the dobby. A re-parent needs a caller of its own that the verb lifts itself. | One write under the record's lock. | A stranded pause: the run stays held until `lym tender resume <slug>`. |

Everything else is derived and needs no write: the beam's blockers (`ReadBeamLandBlockers`), its dobby's member test, the dobby's slot count and run list (`DobbyRuns` takes the parent from `ParentOf`), the project directive of the next spawn, `pairteardown`'s dobby lookup, and `hubgeom.Parent` for the creel route.
Two derived views are cached outside the code the study read: the generated workspace file (`hubworkspace.go` reads each origin's `ParentWorktree`) and the reed window grouping (`ideengine/spawn.go`).
*The triggers that regenerate them were not traced.*

A crash between the two durable writes is safe when row 3 is written first.
Row 3 decides landing, ownership and blockers, so after it the old beam stops counting the task and the new parent starts counting it.
Rows 1 and 2 then lag, which costs a stale project directive and a wrong dobby member test until the verb is re-run.
The verb is idempotent: each step reads its current value and writes only a difference.

## 3. The git side

A task forked from beam `B` holds `B`'s commits as ancestors.
Let `T` be the merge base of the task with its old parent branch, read after any Parent-Align merge-in, so it covers what `B` gave the task.

Landing merges the task branch into the parent (Finalize).
Whatever of `B` is below the task is carried in with it.
That gives three cases for a new parent `N`:

| Case | `N` holds `T` | What a landing does |
|---|---|---|
| `B` already landed in `N` (the issue's case: the beam landed in `main`) | yes | Parent-Align merges `N` in as usual; Finalize lands only the task's own commits. |
| `N` is a new beam that took `B` in with `lym fabric merge-in` (the issue's workaround) | yes | Same. |
| `N` lacks `B`'s unlanded commits | no | Parent-Align merges `N` into the task, which does not remove `B`'s commits. Finalize then lands `B`'s unreviewed work into `N` along with the task. |

The note names two answers.

- **Refuse.** The verb checks that `N`'s tip holds `T` and refuses otherwise, naming the way forward: land `B` first, or `lym fabric merge-in` `B` into `N`, then re-run.
- **Merge `N` at the next Parent-Align.** This is what the landing does anyway when `N` moved on.
  It changes nothing in the third case, because merging `N` in never takes `B`'s commits out.

**Recommendation: refuse.**
The second answer is not an alternative for the third case; it is the unchecked path to landing someone else's work.
The refuse check costs one ancestry test with existing primitives (`MergeBase`, `HoldsWork`, `HeadHoldsCommitsNotIn`).
It leaves the first two cases, which are the reasons for the verb, on the ordinary Parent-Align path with no new merge logic.

Open risk, *not read*: `HoldsWork` was not opened.
If it judges ancestry, a beam landed by a squash merge on a pull request reads as not held even though its content is in `N`.
Then the first case needs either a content-based test or a `--force` that the operator names.
`ResolvePullRequest` mentions a `TrueMerge` mark, so a hub's pull-request mode decides how often this bites.

What the weft side carries is harmless: the task's records branch was forked from `B`'s records branch with the run records dropped.
Records do not merge on landing (`main-weft` does not advance); a landed pair's records live in its archive tag.
The inherited copy of `B`'s project record in the task's weft is never read, since readers go to the live beam records.
The origin-record commit subject marks the pair's own history, and its doc comment says everything below it is the parent's; that stays true of the old parent.
A re-parent commit must use another subject, so a reader that finds the record's first commit by `OriginRecordSlug` still finds the original.

## 4. Beam-Await, the old beam's pull request, and the new parent

**Old beam.**
Beam-Await accepts only a land request naming its own run, writes the landing hold, then re-runs the checks, `BeamLandRefusal` among them, before anything lands.
Blockers are read at that moment from the board and the origin records, so a member whose rows 1 and 3 moved is simply not counted.
No stored member list needs an edit.
The beam's pull request has the beam branch as head.
The task's branch was never part of it, so the pull request, its ready record and `BeamCommitsPast` are untouched.
The old beam's dobby loses the run from its view on the next tick and its slot frees.

**New parent, a beam.**
The target must pass what a fork from it passes: `beamForkPrecondition` (pair present on this machine, run not holding its entry from Finalize on, ready record not flagged).
The pair-present and held cases are refusals with ways forward; a flagged-ready beam takes no member because the member could not land into it.
So a task cannot be moved into a beam that is already ready, the same way it cannot be forked from one.
Once moved, the task is a blocker of the new beam from the next read: the board, `ParentOf` and the origin's `ParentWorktree` all name it.
The new beam's dobby starts no new member for it and counts its slot; whether a run beyond the dobby's `max_runs` is tolerated was *not read*.

**New parent, the prime (`root`).**
`Add` run from the prime records `ParentBranch` as the prime's checked-out branch and `ParentWorktree` as the prime's worktree name.
`ParentOf` reads that as the prime.
The empty/empty record is the root-pair case, whose landing is refused, so the verb must not write it.
Landing then follows the prime's `beam.yaml` `pull_request` and the branch rule of the receiving branch, which can differ from the beam's, so the task may now go through a pull request where it did not.

**Ownership.**
`Owners.Effective(slug, ParentOf)` follows the parent.
The verb requires this machine to own the task both before and after.
Moving between parents owned by two machines is refused, naming `lym adopt`.

## 5. What `lym batten reparent <slug> --to <beam|root>` writes

Scope: tasks only.
A beam project, a grouping project, a front task and a done entry are refused.

### Refusals, in order, each naming its way forward

1. The entry is not a task of this hub, or has no run (nothing to move; `lym board upsert` does it).
2. This machine does not own the task, or would not own it under the new parent (`lym adopt`).
3. The target is the current parent (no-op, exit 0 with `changed: false`).
4. The target is not an open beam, is not marked, has no pair on this machine, is held from Finalize on, or is flagged ready (the `beamForkPrecondition` set; `lym beam create` or `lym adopt`).
5. The task's landing is in flight: a parent lock held by its run, Finalize past its parent-side merge (`ParentHoldsTaskWork`), or a pull request recorded.
   The way forward is to let the landing finish, or `lym batten cancel`.
6. A driver of the run is still alive after the pause.
7. `N` does not hold `T` (section 3).
8. A merge parked in the task worktree.

### Order

1. Pause the slug with a caller of its own, so `refuseCovered` stops new passes and the parent lock queue yields.
   End the run's driver (the pause's `--grace`, *engine not read*).
   Re-check refusals 5 and 6 after the pause, since a landing can start between the first check and the pause.
2. Origin record: write `ParentBranch` and `ParentWorktree` (keeping `Beam`), commit it on the task's records branch with its own subject, and queue the pair's detached push.
   This is the commit point.
3. Merge record: delete it, so Parent-Align measures against `N`.
4. Run position: a run at or past Parent-Align goes back to Parent-Align through the existing `lym batten goto` mechanism; an earlier row needs nothing.
5. Loom status product: rewrite `Parent`.
6. Board: set `project` (the beam's slug, or empty for root) on the task, and move the slug between the two projects' `Order`.
   The write bypasses the run lock for these fields only.
7. Lift the verb's own pause.
   The run resumes with `lym batten run`, which reads `Deps` from the new record.
8. Report the old and new parent, the ancestry check, and what stayed stale: the running session's prompt, and the workspace file until its next regeneration.

A crash after 2 is repaired by re-running the verb: step 2 finds the target already recorded and skips, and the rest are written from their current values.
A crash before 2 changed nothing durable but the pause, which the re-run lifts.

### Tests

- `boardengine`: the narrow write passes the run lock for `project` and `Order` only, and `upsert` of a claimed entry still returns `RunLockedError`; refusals of a missing target, a non-project, a done project, and the task itself.
- `fabricengine`: the origin rewrite changes only the parent fields, keeps `Beam`, uses a subject other than the creation subject, is idempotent, and survives an `AdoptOnly` re-make.
- `battencli`, one table over the refusals in the order above, each asserting no write happened; the no-op case; root versus beam targets.
- Integration (one hub from `hubforge`): a task in beam `B` moved to root and to a second beam; after each, `ParentOf`, `ReadBeamLandBlockers` of `B` and of the target, the creel parent, the merge record and the status `Parent` are checked.
- Crash points: a fault after the origin commit and one after the board write, each followed by a re-run that converges.
- Ancestry: `N` holding `T` passes; `N` lacking it is refused with the `fabric merge-in` way forward.
- Pause: the verb's pause is lifted on success and on every refusal after it is taken.
- `PATTERN-owner-machine` and the sole-writer tests: the origin record has one new writer, and the pattern entry for `ParentOf` states that only the reparent verb changes a recorded parent.

### Size

A cross-module change: `boardengine`, `fabricengine`, `battencli`, `pauserecord`, `loomshed`, `landingshed` (for the merge record's owner), `loomcli`, the docs of each, and a `PATTERN` entry.
It sets a new invariant (one verb may change a recorded parent) and decides several behaviors above, so it wants a Discussion and a plan.
Recipe `tapestry`, profile `solid` (the default for tapestry).
If this report is accepted as its Sketch, `darn` on profile `light` is the cheaper route, at the cost of the plan's review of the crash-point order.

## Not covered

- Consumers of `Task.Order`, the triggers that regenerate the workspace file and the reed window grouping, and the engine behind `lym tender pause --grace` were not read.
- Whether a dobby tolerates a run beyond its `max_runs` was not read.
- `HoldsWork`'s test (ancestry or content) was not read; section 3 names the consequence.
- A nested beam (a beam inside a beam) as a target or as a moved entry was not studied; the verb refuses a beam project.
