# PATTERN-owner-machine

Starting, resuming or landing a run, bar the rest of a Finalize already past its parent-side merge, or acting on an entry after a wake: only on a machine that does not read the entry as owned by another, where owners sit only on the root project and on beam projects, a beam project with no owner recorded reads as owned by its parent's owner, every other entry reads as owned by its parent's owner, and the root of a hub with no prime owner recorded reads as owned by no other machine; its owner written only by `boardengine`'s owner setter; the owner verdict that gates a resume, a wake or a landing judged against origin inside `ReconcileOrigin`, a start's read from the Bolt it just pulled, the re-read after an owner write's push from the Bolt it just pushed, the landing's parent-owner read from the Bolt its reconcile left, and a resume with origin unreachable reading the local Bolt; this machine's name only from `internal/machinename`.

- A start, a resume, a claim and a landing proceed only when the entry does not read as owned by another machine.
- The rest of a Finalize already past its parent-side merge finishes on either machine, so a landing in flight is never stranded.
- An owner is recorded for a beam project as `owner_machine` and for the root as `prime_owner`; both are written only by `boardengine`'s owner setter, which takes a project another machine owns only when `lym adopt` asks for it.
  A task and a grouping project record none, and a load drops the key an older machine recorded on one.
- Every other entry reads the owner of its parent, and a beam project with no owner recorded does too; the root of a hub with no prime owner recorded reads as owned by no other machine.
  A run status on an entry does not change whose owner it reads.
  The parent comes through `hubgeom.ParentOf` alone, the one resolver every owner reader is handed, which reads the entry's origin record and, with none, the local board's beam chain.
- Adopting a beam moves the beam only: a member reads its beam's owner through that one resolver.
- The verdict that gates a resume, a wake or a landing is judged against origin inside `ReconcileOrigin`.
  A start reads it from the Bolt it just pulled, the re-read after an owner write's push reads the Bolt it just pushed, and the landing's parent-owner read uses the Bolt its reconcile left.
  A resume with origin unreachable reads the local Bolt.
- This machine's name comes only from `internal/machinename`.

## Origin reconcile first

Acting on a run or a hub after an absence starts with `fabricengine`'s origin reconcile, so every verdict above reads the state origin holds.

## Passes

A run is driven in passes, one `lym batten run` each, and a pass is either full or a continue pass.

- A continue pass is the one dobby schedules into a run it already drives, and it is not a resume: it needs no origin verdict, so it skips the resume's origin reconcile, the Open-Window re-run and the pre-run pause refusal.
- The first pass of a run after a dobby start or a machine wake, every pass while a full pass is owed, and every hand pass is a resume and gets the origin reconcile, which every full pass keeps first.
- A continue pass is refused unless batten's status reads `running` at Run-Shed, the parent worktree's dobby state names the run as one it drives with no full pass owed, and no origin-owed marker stands; the refusal reads to dobby as a full pass owed.
- A resume that judged its owner without origin leaves the origin-owed marker, which stays until a pass that reached origin exits 0, so the passes stay full until origin has been reached.

## Enforcement

- `internal/machinename`'s sole-source scan fails on a production file outside the package that reads the hostname.
- The reconcile, batten, loom, `InnerRun` and Finalize tests reach each check: `ReconcileOrigin`'s owned-elsewhere outcome, batten's start, resume and claim refusals, loom's `owned_elsewhere` start refusal, `InnerRun`'s owner pause and Finalize's `owner check:` halts.
- Review covers the rest: who writes an owner, and which Bolt each verdict reads.
