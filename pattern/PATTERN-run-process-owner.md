# PATTERN-run-process-owner

Every detached or long-lived process lyx starts for a run joins one owner record, and a run's processes are ended only through `runproc`'s `End`, `EndSparing` or `EndRole`.

- The record lives in `internal/runproc`, one file per process under the run's slug, or under `runproc.HubOwner` for a process that belongs to the hub: a detached board sync.
- A spawner registers a child it starts with `runproc.ChildRecord`; a process that runs for a run registers itself with `runproc.SelfRecord`, under a role the package names.
- `End`, `EndSparing` and `EndRole` alone end the processes an owner holds, and every verb that ends or tears down a run calls one with the caller's own pid, so the caller and its ancestors are spared.
  `EndSparing` leaves named roles alone, and `EndRole` ends one role's records under an owner with `End`'s sequence, so the rest of the owner's processes go on.
- `EndProcess` serves only the orphans `lyx tender reap` ends, which no record names.
- No process removes its own record; the read of a record prunes the ones whose process is gone.
- A site that signals a run's process by any other route is a review finding.

## Enforcement

- `internal/runproc`'s tests pin the record and the end sequence, and its leaf test pins the import set.
- Review covers the rest: that each spawn registers, and that each end goes through `End`, `EndSparing` or `EndRole`.
