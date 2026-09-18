MILL_REVIEW_BEGIN
# Review: Worktree spawn/teardown as Shed producers

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-class (Claude Opus, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:design] Teardown seam hides the ordering it must prove
**Section:** `three-registry-entries-closures-not-engine-imports` vs `teardown-is-one-row-sequencing-internally` / Testing
**Issue:** `Env.TeardownWorktree func(context.Context) error` is one opaque closure, yet the producer is required to sequence shutdown-then-removal, to *not* call removal when shutdown fails, to distinguish the two in its stuck reason, and to surface `DownResult.AbandonedSession` — none of which is observable or testable at the `lifecycleshed` layer through a single `error`; the Testing section's "fake recording the call sequence" and the new invariant's "sequences session shutdown before worktree removal, in one producer" both presuppose two seams.
**Fix:** Decide the teardown seam shape explicitly (e.g. a `lifecycleshed.TeardownDeps` passthrough with separate `Shutdown`/`Remove` fields, `Shutdown` returning the abandoned-session value) — this is the same two-layers-at-once defect Round 2 fixed for `LoomRun`, left uncorrected for teardown.

### [BLOCKING:design] Sandbox scenario cannot avoid a real loom run
**Section:** Constraints → Sandbox Suite Coverage
**Issue:** The scenario claims "a task whose loom run is already complete … a fast `Done` from Loom-Run … without spawning an LLM", but `Worktree-Create` makes a brand-new pair with no loom status, and `Loom-Run` then spawns `lyx loom run --no-attach`, which seeds a fresh status and — `mustSpawnDriver(runLockHeld)` is `!runLockHeld` (`internal/loomcli/bootstrap.go:30-32`) — always spawns a real `loom drive` into loom's LLM rows; no `step`/`pause` verb exists on the lifecycle CLI to interpose a fixture, and the existing loom scenario (`tools/sandbox/SANDBOX-CORE-SUITE.md:245`) hand-writes `_lyx/loom/status.json` precisely because no shipped verb seeds one otherwise.
**Fix:** State the actual short-circuit for the sandbox path (a fixture written into the new worktree, a stub engine selection, or a scenario that does not drive Loom-Run at all) or revise the scenario's claim.

### [BLOCKING:design] Driver-runs-from-prime rests on a refusal that does not exist
**Section:** `lifecycle-driver-runs-from-prime`
**Issue:** The rationale asserts `Add` and `Remove` "both refuse to operate on the worktree they are invoked in"; `Topology.Remove` refuses only the *prime* slug (`refusePrimeSlug`, `internal/fabricengine/remove.go:208-219`, comparing slug against `PrimeName(l)`), so removing the worktree you are standing in is not refused — and the discussion states no disposition for `lyx lifecycle run` invoked from a non-prime worktree, while conceding the Bookend invariant has no enforcing test.
**Fix:** Correct the stated fact and decide whether `lifecyclecli` refuses outright when its resolved `Location` is not the hub's prime worktree (the only thing that makes the invariant's cwd claim true at runtime).

### [BLOCKING:decision] `lifecyclecli`'s `RunCLIIn` and the CLI/Cobra count undecided
**Section:** Constraints → CLI / Cobra Invariant
**Issue:** The discussion commits `lifecyclecli` to `Command()`/`RunCLI` only; the invariant's "eleven of twelve also carry `RunCLIIn`" (eleven confirmed in-tree) goes stale at thirteen modules, and the path-derivation tests that serve as the Bookend invariant's mechanical proxy need an injectable cwd, which `RunCLI` alone does not give.
**Fix:** State whether `lifecyclecli` carries `RunCLIIn(cwd, out, args) int`, and that the CLI/Cobra Invariant's module count line is amended in the same commit.

### [NIT:consistency] `Env.Slug` described as surviving, not as new
**Section:** `three-registry-entries-closures-not-engine-imports`
**Issue:** "`Env.Slug` stays" reads as if the field exists; `shedrecipe.Env` (`internal/shedrecipe/recipe.go:34-114`) has no `Slug` field today, and Scope correctly lists it as new.
**Fix:** Phrase it as an added field so the plan writer does not look for an existing one.

## Verdict

REQUEST_CHANGES
Teardown seam, sandbox feasibility, and the prime-cwd premise need deciding before planning.
MILL_REVIEW_END
