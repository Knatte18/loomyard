# pair-teardown: one sequence that ends a pair

## Why the package exists

`reedengine` imports `fabricengine`, so fabric cannot end a reed session itself without an import cycle.
Two callers need the same order, session end before worktree removal: `lyx fabric remove` and batten's Worktree-Teardown row.
`internal/pairteardown` holds that sequence once, so the callers cannot drift.
It imports `fabricengine`, `reedengine`, `hubgeom`, `loomengine` and `shedrun`; `fabricengine` imports none of `reedengine`, `loomengine` or `githubclient`.

The package sits outside the Fabric Vocabulary owner set.
It says "pair" and "task worktree", and passes `fabricengine.RemoveResult` through whole instead of naming its side-named fields.
The fabric exports it calls are vocabulary-neutral: `Topology.RemoveRefusal` and `ErrPairNotFound`.

## Sequence

`EndSession` runs three steps and stops at the first failure:

1. Quiet wait: poll until the pair's driver is quiet, bounded by `Request.QuietWait`.
2. Refusal probe: `Topology.RemoveRefusal` answers whether `Remove` would refuse, with nothing touched.
   A refusal leaves the session and its strands live.
3. Session end: `Engine.Down` while the task worktree is present, which also tears down the hub's tmux server after the last session; `reedengine.EndSessionByName` once it is gone.

`RemovePair` then calls `Topology.Remove`.
`Run` sequences both phases.

## Quiet rule

A pair is quiet when its run lock is free and its driver strand is absent, dead, retiring or parked.
A retiring driver is about to remove itself, and a parked driver has already committed its records through its park command, so neither blocks the teardown.
A gone task worktree is quiet by exception, bounded: neither the run lock nor reed's strand table can be probed there, so the session is ended by exact name with no wait.
"Gone" means the path is not a registered linked worktree.

## Caller policies

`fabric remove` passes `RemoveQuietWait` (two minutes, a constant) with `RefuseWhenBusy`.
The wait never kills a working driver on the operator path, and an unbounded wait would hang the hub's land step.
`--force` answers dirtiness only and never the wait.

Batten passes a zero wait without `RefuseWhenBusy`, since it already waited its `driver_exit_grace_s`.
It keeps its two-closure `TeardownDeps` and calls the two phases, so its producer still reports which half failed and still surfaces an abandoned session.
`ErrPairNotFound` is the done post-condition for both phases: `EndSession`'s refusal probe raises it once nothing of the pair remains, and so does `RemovePair`.
A teardown re-entered after a completed removal therefore reports done instead of halting at session shutdown.

## The finish path it relies on

`Remove` finishes a half-removed pair instead of refusing it, so a teardown re-entered after an interruption calls it again.
It commits pending sibling records before the archive tag, reports its steps and any stray path, and with `--remote` deletes the landed task branch on origin behind a lease.

## Out of scope

- Advancing `main-weft` on landing: weft content is per-branch and never a merge participant, and a landed pair's records live in its archive tag.
  The board item closes at landing, not here.
- Sweeping leftover task branches on origin is `Topology.CleanupRemoteWarp`, composed by `lyx fabric cleanup --remote` with the open-PR heads it fetches; it is not part of this sequence.
