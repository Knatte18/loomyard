# PATTERN-owner-machine

Starting, resuming, pushing or landing a run, or picking a pair's parent: only on the entry's owner machine, read through hubgeom's one `ParentOf`.

## Owner machine

Rule: Run start, resume, push or landing: only if no other machine owns its entry; a loom push reconciles run-scope origin first; owners only on roots and beams, set only by `boardengine`'s setter; others read their `hubgeom.ParentOf` owner; a dobby continue is no resume.

A run starts, resumes, pushes or lands only on a machine that does not read its entry as owned by another.
The same holds for acting on an entry after a wake.
The rest of a Finalize already past its parent-side merge is exempt.

- A start, a resume, a claim and a landing proceed only when the entry does not read as owned by another machine.
- A loom run's commit and push of its pair proceed only after a run-scope origin reconcile reads its entry as active.
  The exceptions are a run with git or pushes off, a run in the prime, Finalize, which judges itself, and a paused run.
  The reconcile judges.
  The push also goes when the check itself fails.
  That covers an unreachable origin, a failed reconcile, and a failed read of whether the check applies.
  It covers a failed read of whether the parent holds the run's work too.
  An ended entry whose parent already holds the run's work goes on too, as after its own landing.
  An entry owned elsewhere is paused under the owner, naming `lym adopt`.
  An entry a branch stops, or origin ended while the parent lacks its work, is paused naming what moved.
- The rest of a Finalize already past its parent-side merge finishes on either machine.
  A landing in flight is therefore never stranded.
- An owner is recorded for a beam project as `owner_machine` and for the root as `prime_owner`.
  Only `boardengine`'s owner setter writes either.
  It takes a project another machine owns only when `lym adopt` asks for it.
  A task and a grouping project record none, and a load drops the key an older machine recorded on one.
- Every other entry reads the owner of its parent, and so does a beam project with no owner recorded.
  The root of a hub with no prime owner recorded reads as owned by no other machine.
  A run status on an entry does not change whose owner it reads.
  The parent comes through `hubgeom.ParentOf` alone, the one resolver every owner reader is handed.
  It reads the entry's origin record and, with none, the local board's beam chain.
- Adopting a beam moves the beam only: a member reads its beam's owner through that one resolver.
- The verdict that gates a resume, a wake, a push or a landing is judged against origin inside `ReconcileOrigin`.
  A start reads it from the Bolt it just pulled.
  The re-read after an owner write's push reads the Bolt it just pushed.
  The landing's parent-owner read uses the Bolt its reconcile left.
  A resume with origin unreachable reads the local Bolt.
- This machine's name comes only from `internal/machinename`.
  It reads the machine file in the per-user lym config directory before the hostname rule,
  and a machine file that yields no valid name is an error, never a fallback to the hostname.

### Origin reconcile first

Acting on a run or a hub after an absence starts with `fabricengine`'s origin reconcile.
Every verdict above therefore reads the state origin holds.
A loom run's commit and push of its pair start with the run-scope origin reconcile too.
The one exception is a resume at Worktree-Teardown whose task worktree is gone.
It runs no origin reconcile, judges the owner on the local board alone and writes no origin-owed marker.
The teardown only finishes what is left of the pair.

### Passes

A run is driven in passes, one `lym batten run` each, and a pass is either full or a continue pass.

- A continue pass is the one dobby schedules into a run it already drives.
  It is not a resume, and it needs no origin verdict.
  It skips the resume's origin reconcile, the Open-Window re-run and the pre-run pause refusal.
- The first pass of a run after a dobby start or a machine wake is a resume.
  So is every pass while a full pass is owed, and so is every hand pass.
  A resume gets the origin reconcile, which every full pass keeps first.
  The exception is a resume at Worktree-Teardown whose task worktree is gone.
- A continue pass is refused unless batten's status reads `running` at Run-Shed;
  the parent worktree's dobby state names the run as one it drives with no full pass owed;
  and no origin-owed marker stands.
  The refusal reads to dobby as a full pass owed.
- A resume that judged its owner without origin leaves the origin-owed marker.
  A Worktree-Teardown resume whose task worktree is gone writes none.
  The marker stays until a pass that reached origin exits 0, so the passes stay full until origin has been reached.

### Enforcement

- `internal/machinename`'s sole-source scan fails on a production file outside the package that reads the hostname.
- The reconcile, batten, loom, `InnerRun` and Finalize tests reach each check.
  They cover `ReconcileOrigin`'s owned-elsewhere outcome and batten's start, resume and claim refusals.
  They cover loom's `owned_elsewhere` start refusal and loom's pre-push stops.
  They cover `InnerRun`'s owner pause and Finalize's `owner check:` halts.
- Review covers the rest: who writes an owner, and which Bolt each verdict reads.

## Pair parent

Rule: Picking a new pair's parent for a board entry or reading an entry's owner through its parent: only `ParentBeam` and hubgeom's one `ParentOf`, never the acting worktree.
