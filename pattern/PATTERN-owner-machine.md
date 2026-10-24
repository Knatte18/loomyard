# PATTERN-owner-machine

Starting, resuming or landing a run, bar the rest of a Finalize already past its parent-side merge, or acting on an entry after a wake: only on a machine that does not read the entry as owned by another, where a run-held entry with no owner recorded, and every entry of a hub with no prime owner recorded, reads as owned by no other machine, and any other entry with no owner recorded reads as owned by its parent's owner; its owner written only by `boardengine`'s owner setter; the owner verdict that gates a resume, a wake or a landing judged against origin inside `ReconcileOrigin`, a start's read from the Bolt it just pulled, the re-read after an owner write's push from the Bolt it just pushed, the landing's parent-owner read from the Bolt its reconcile left, and a resume with origin unreachable reading the local Bolt; this machine's name only from `internal/machinename`.

- A start, a resume, a claim and a landing proceed only when the entry does not read as owned by another machine.
- The rest of a Finalize already past its parent-side merge finishes on either machine, so a landing in flight is never stranded.
- An entry's owner is recorded per task as `owner_machine` and for the prime as `prime_owner`; both are written only by `boardengine`'s owner setter, which takes an entry another machine owns only when `lyx adopt` asks for it.
- An entry with no owner recorded reads by its state: a run-held entry, and every entry of a hub with no prime owner recorded, as owned by no other machine, and any other as owned by its parent's owner.
- The verdict that gates a resume, a wake or a landing is judged against origin inside `ReconcileOrigin`.
  A start reads it from the Bolt it just pulled, the re-read after an owner write's push reads the Bolt it just pushed, and the landing's parent-owner read uses the Bolt its reconcile left.
  A resume with origin unreachable reads the local Bolt.
- This machine's name comes only from `internal/machinename`.

## Origin reconcile first

Acting on a run or a hub after an absence starts with `fabricengine`'s origin reconcile, so every verdict above reads the state origin holds.

## Enforcement

- `internal/machinename`'s sole-source scan fails on a production file outside the package that reads the hostname.
- The reconcile, batten, loom, `InnerRun` and Finalize tests reach each check: `ReconcileOrigin`'s owned-elsewhere outcome, batten's start, resume and claim refusals, loom's `owned_elsewhere` start refusal, `InnerRun`'s owner pause and Finalize's `owner check:` halts.
- Review covers the rest: who writes an owner, and which Bolt each verdict reads.
