# PATTERN-owner-machine

Starting, resuming, pushing or landing a run, a Finalize past its merge included, or picking a pair's parent: only on the entry's owner machine, read via hubgeom's `ParentOf` and Bolt's machine registry; a loom pusher reconciles first; after absence, a catch-up from origin first.

## Owner machine

Rule: Run start, resume, push or landing, a Finalize past its merge included: only if no other machine owns its entry, read through Bolt's machine registry; a loom pusher reconciles run-scope origin first; owners only on roots and beams, set only by `boardengine`'s setter; others read their `hubgeom.ParentOf` owner; a dobby continue is no resume; after an absence, a catch-up from origin first.

A run starts, resumes, pushes or lands only on a machine that does not read its entry as owned by another.
The same holds for acting on an entry after a wake.
The rest of a Finalize already past its parent-side merge finishes on the run's owner.

- Every comparison of a recorded machine name with this machine goes through `boardengine.OwnedElsewhere` over the registry.
  A former name of a registered machine reads as that machine.
- A start, a resume, a claim and a landing proceed only when the entry does not read as owned by another machine.
- A pair's pusher runs the run-scope origin reconcile before each push, and the push proceeds only once it reads the entry as active.
  A pause covering the run stops the pusher's push, and the commit runs no check.
  The exceptions are a run with git or pushes off, a run in the prime, and Finalize, which judges itself.
  The reconcile judges.
  The push also goes when the check itself fails.
  That covers an unreachable origin, a failed reconcile, and a failed read of whether the check applies.
  It covers a failed read of whether the parent holds the run's work too.
  An ended entry whose parent already holds the run's work goes on too, as after its own landing.
  An entry owned elsewhere is handed over by the catch-up, naming `lym adopt`.
  An entry a branch stops, or origin ended while the parent lacks its work, is paused naming what moved.
- The rest of a Finalize already past its parent-side merge finishes on the run's owner.
  A landing in flight is therefore never stranded.
- An owner is recorded for a beam project as `owner_machine` and for the root as `prime_owner`.
  Only `boardengine`'s owner setter writes either.
  It takes a project or the prime another machine owns only when `lym adopt` asks for it, or when `lym machine name`'s sweep names that recorded owner among this machine's former names.
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
- This machine's name comes only from `internal/machinename`, with the machine id beside it.
  It reads the machine file in the per-user lym config directory before the hostname rule,
  and a machine file that yields no valid name is an error, never a fallback to the hostname.

### Catch-up after an absence

Acting on a run or a hub after an absence starts with a catch-up from origin:
a sleep the dobby sees, a dobby start and a hand pass each run it before any act rule answer.
Every verdict above therefore reads the state origin holds.
A pair's pusher starts each push with the run-scope origin reconcile.
The one exception is a resume at Worktree-Teardown whose task worktree is gone.
It runs no origin reconcile and judges the owner on the local board alone.
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
  and the parent worktree's dobby state names the run as one it drives with no full pass owed.
  The refusal reads to dobby as a full pass owed.

### Enforcement

- `internal/machinename`'s sole-source scan fails on a production file outside the package that reads the hostname.
- The reconcile, batten, loom, `InnerRun` and Finalize tests reach each check.
  They cover `ReconcileOrigin`'s owned-elsewhere outcome and batten's start, resume and claim refusals.
  They cover loom's `owned_elsewhere` start refusal and the pusher's pre-push stops.
  They cover `InnerRun`'s owner hold and Finalize's `owner check:` halts.
- `TestReadsAsHere` and `TestRegistryCheck` cover the registry comparison.
  `TestCatchUp` and `TestRun_CatchUp` cover the catch-up after an absence.
- Review covers the rest: who writes an owner, and which Bolt each verdict reads.

## Pair parent

Rule: Picking a new pair's parent for a board entry or reading an entry's owner through its parent: only `ParentBeam` and hubgeom's one `ParentOf`, never the acting worktree.
