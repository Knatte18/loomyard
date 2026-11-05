# PATTERN-child-processes

A started process is logged at its spawn, waited on by its starter, and, when it belongs to a run, registered in `internal/runproc` and ended only through it.

## Spawn observability

Rule: Starting OS process from `lym`: log the spawn via `internal/logger`; logging in a loop or poll follows its Level policy.

Every code path reachable from a `lym` command that starts a real OS process logs its spawn via `internal/logger`, and logs its teardown wherever it waits for one.
The level is `Info` for a lifecycle spawn and `Debug` for a spawn inside a polling probe.

- A detached spawn, reaped in the background by `proc.Reap`, logs the spawn alone; there is no teardown to observe.
- A site structurally barred from importing `internal/logger` is exempt, and carries a written reason wherever the exemption is recorded.
- Test-fixture machinery, standalone harnesses and dev tooling under `tools/` are outside this rule, not exemptions to it.
- Never re-exec `os.Executable()` under `go test`.
- A retry loop around a real spawn caps attempt COUNT, not only elapsed time.

### Log levels

Logging inside a loop or a poll follows `internal/logger`'s Level policy, stated in its package doc: no `Warn` in a loop that iterates without a state change, and a failure that repeats each tick logs `Warn` once per episode.

## Run process owner

Rule: A run's processes: registered in `internal/runproc`, ended only via `runproc`'s `End`, `EndSparing` or `EndRole`.

Every detached or long-lived process lym starts for a run joins one owner record, and a run's processes are ended only through `runproc`'s `End`, `EndSparing` or `EndRole`.

- The record lives in `internal/runproc`, one file per process under the run's slug, or under `runproc.HubOwner` for a process that belongs to the hub: a detached board sync.
- A spawner registers a child it starts with `runproc.ChildRecord`; a process that runs for a run registers itself with `runproc.SelfRecord`, under a role the package names.
- `End`, `EndSparing` and `EndRole` alone end the processes an owner holds, and every verb that ends or tears down a run calls one with the caller's own pid, so the caller and its ancestors are spared.
  `EndSparing` leaves named roles alone, and `EndRole` ends one role's records under an owner with `End`'s sequence, so the rest of the owner's processes go on.
- `EndProcess` serves only the orphans `lym tender reap` ends, which no record names.
- No process removes its own record; the read of a record prunes the ones whose process is gone.
- A site that signals a run's process by any other route is a review finding.

### Enforcement

- `internal/runproc`'s tests pin the record and the end sequence, and its leaf test pins the import set.
- Review covers the rest: that each spawn registers, and that each end goes through `End`, `EndSparing` or `EndRole`.

## Reap children

Rule: Starting a child process: its starter waits on it, by `cmd.Wait` or `proc.Reap`, never only releasing it; a process that re-execs itself calls `proc.ReapInheritedChildren` before it starts a child.
