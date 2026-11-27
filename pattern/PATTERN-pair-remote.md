# PATTERN-pair-remote

Pushing run records or tearing down a pair: the code branch goes along after the records side, from the worktree's background pusher, which no transition waits for;
a pair's remote steps hold the slug's teardown lock;
teardown only via `internal/pairteardown`.

## Push both sides

Rule: Pushing run records from Go: the records side first and the code branch only once it landed;
a push refused after retry leaves a failed-push record and creel notice.
A teardown's push of a weft branch it archives and deletes goes alone and only logs a refusal. (test)

Go pushes a run's records side, and in a task pair the code branch goes after it.

- In a task pair the records push carries the code branch after it, rebase-free, under fabric's absorbing push lock.
  The records side goes first and the code side only once it landed:
  a failed or rejected records side pushes no code.
- The push is never left to an agent.
- The worktree's pusher pushes the records side and then the code side, and no transition waits for it.
  An owed push, on an unreachable origin or a held teardown lock, is retried in the pusher.
  A refused push leaves its record and notice and stops the pusher until the record's way forward clears it.
- The detached pushes that stay keep that order in `coalescePushSides`:
  the anchor change, fabric's commit spawn, `lym fabric sync` and `add`, and a lift's deferred push.
  The pending-flag respawn and the failed-remove respawn are gone, and a left flag waits for its worktree's next push.
- The prime's pushes stay records-only, its code branch being the operator's.
- A teardown's archive step pushes the weft branch alone, with no code branch, before the archive tag naming its tip,
  and a refusal of that push is logged only, since the removal archives and deletes the branch anyway.

### A refused push stays visible

A push still refused after its retry leaves a failed-push record and a creel notice, never a silent loss.
A test enforces it.

## Slug remote order

Rule: Touching a slug's pair or origin branches:
under its teardown lock, shared for a pair push outside a removal and else exclusive;
a teardown's detached child holds it to its last remote step;
an origin weft delete leases the tip read.

Held by review alone; no test enforces it.

## Pair teardown

Rule: Tearing down a pair: only via `internal/pairteardown`;
a producer creating or destroying a task worktree never runs inside it. (test)

Every teardown of a pair goes through `internal/pairteardown`.
It ends the pair's owned processes and then its reed session before any worktree is removed.
Its driver checks are skipped for a pair that is paused, ended or owned by another machine.
They are also skipped for a pair done locally with batten no longer running.
Its run-lock wait is never skipped while the task worktree is present;
it then runs before the owned processes are ended.
A batten cancel's phase instead ends the owned processes and strands first and waits for the run lock after,
and its removal leaves the board claim to the cancel, which clears it.

- The dobby tears down the leftover pairs of its runs through `internal/pairteardown` alone:
  a landed run's pair, a cancelled run's pair its cancel did not keep, an ended run's pair through the origin reconcile,
  and a handed-over run's local pair in local mode.
  The prime's dobby also tears down an orphan pair, one no board entry or batten record names,
  once it has gone idle for `dobbyengine.OrphanGrace`, its tips archived first.
- A run's Worktree-Teardown tears down the bench pairs its darn writer made, through `internal/pairteardown`,
  before it pushes the run's own pair and with no prime lock held.
  It ends each bench pair's live processes first, then archives its tips and removes it with the driver checks skipped.
  That order is an exception to the ones below:
  the processes end before the run-lock wait and the refusal probe, so a failed archive or removal leaves them ended.
- No production package outside `internal/pairteardown` and `internal/fabricengine` calls `Topology.Remove`.
- The composite waits for the pair's driver to go quiet, bounded by `Request.QuietWait`;
  `--force` answers dirtiness only and never shortens or skips that wait.
  The wait is skipped only for the pair the opening line names.
  Such a pair waits for its run lock first, before the refusal probe and the owned-process end:
  a holder a live waiting marker of its own pid names, or a pause with no parent lock record covers, passes to that end;
  any other holder refuses.
- The session end runs `runproc.End` for the pair's slug first, after the refusal probe and before anything else,
  so a refused removal ends no owned process, and with the task worktree present the driver-lock re-check follows it.
  It then kills a live step loop and its step tree, holds the loop lock through the session end,
  and leaves the teardown's mark in the loop's pid file.
  The mark bars every later loop up to the removal and is withdrawn when the session end fails.
  It then waits for the run lock, keeping the order owned processes, loop, session end, then worktree removal.
- The composite copies the pair's trace and verify logs into the hub log archive.
  It does so after the run-lock wait and before the removal, and a failed copy never fails the teardown.
- After a successful removal the composite settles the pair's board entry:
  a landed run's entry is marked done and an abandoned run's claim is cleared;
  neither happens while batten's run for the slug is `running`.
  So every teardown route releases the claim.
  A `running` record with no live registered batten process means the run is gone.
  The composite keeps the claim and warns with `lym batten cancel <slug>`.
- After a successful removal the composite drops the hub's unread creel mail tagged with the pair's slug as its run,
  through the run mail dropper the root pre-run installs, so no notice about a gone run reaches an orch as live.
  A failed drop never fails the teardown.
- A request with `ClearStray` deletes a directory that is no worktree at the task worktree's path.
  It does so once the removal left the pair gone, through `fabricengine`'s `RemoveStrayTaskPath`.
  A failed delete fails the teardown.
- `EndForCancel` is a batten cancel's phase.
  It ends the owned processes under the prime's and the pair's records write locks.
  The pair's drift capture job, the strands, the loop, the run-lock wait and the session end follow.
  The caller's own locks are taken between the owned-process end and the capture job's termination.
  Its removal passes `Request.LeaveClaim`, which settles no claim, since the cancel clears it under its own locks.
- Worktree-Teardown pushes the pair to completion with the prime lock released,
  and removes it only while origin's tracking refs hold both tips.
  With git or pushes off it pushes nothing and removes the pair without that read.
- `internal/pairteardown` stays inside the fabric-boundary entry:
  it names no side and passes `fabricengine.RemoveResult` through whole.
- Enforced by `internal/pairteardown/teardown_enforcement_test.go`, a tripwire and not a completeness proof.
  It flags a five-argument `Remove` call in a file importing `internal/fabricengine`.
  It also flags any reference to `RemovePairBranch`, a name that stays retired.

### Batten bookend

A producer that creates or destroys a task worktree never runs from inside that worktree.

- The batten Shed is anchored at the prime.
  Its status file is durable under prime's own anchor,
  and its locks stay ephemeral there, never under the worktree being managed.
- A teardown row sequences session shutdown before worktree removal, in one producer, never two rows.
- "Prime" means the warp prime.
  The weft sibling is a repository of its own whose prime is itself, so the name comparison alone admits it.
  `battencli`'s pre-run therefore calls `fabricengine.RequireDrivableWorktree` ahead of the name check;
  that function is `RequireWarpWorktree` under a name a non-owner may say at all.
  Integration test `TestBattenIntegration_Rows/RecordsPrimeRefusal` covers it.

#### Enforcement

Review discipline with two partial mechanical proxies, not an enforcing test:
the rule constrains which directory a running process is driven from, which has no static shape an AST scan can see.
- `internal/battenshed`'s seam-enforcement scan bars a direct resolver import,
  so the package cannot resolve its way into the managed worktree.
- `internal/battencli`'s path-derivation tests pin the status and lock paths to prime's anchor,
  so a relocation under the managed worktree fails there.
- Neither proves the driver's own working directory, which stays a review obligation.
