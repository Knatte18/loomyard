MILL_REVIEW_BEGIN
# Review: shed: the LLM driver as a generic stepper and mender

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewed_file: _mill/discussion.md
date: 2026-09-26
```

## Findings

### [BLOCKING:design] Absent-status baseline has no recipe-blind rule
**Section:** `repair-cap`, `repair-scope` (interrupted sub-cases), `recipe-blind-skill`
**Issue:** With no status file, loom's `status` returns a kind-less refusal (`AbsentStatus.Refuse: true`, `internal/loomcli/arm.go` ~174), while batten's returns `found: false` (`internal/battencli/arm.go` ~331). The current skill reconciles the two in a loom-named "pre-loop baseline" paragraph that the rewrite must drop. The discussion never says how the recipe-blind skill takes its baseline or its `repair-cap` key (`current_producer` + `history_length`) when the status read is that refusal. This is exactly the `kind: unseeded` repair case. A status refusal carries no discriminator (only `trace_dir` is added), so the skill cannot tell "absent" from "broken".
**Fix:** Decide the recipe-blind disposition: which status results count as an empty baseline/key and which hand back. Either add a generic discriminator on the absent-status envelope or state the rule in words the skill can apply without naming a recipe.

### [BLOCKING:consistency] logger admission understated for path accessors
**Section:** `shedverbs-imports-logger`, `trace-dir-and-trace-id`
**Issue:** The rationale says "`shedverbs` calls no resolver itself" and cites the `internal/friction` precedent. But `TraceDir`/`TraceFile` are path-returning accessors that run `lyxcwd.Getwd` + `lyxcwd.Resolve` (`internal/logger/sink.go` ~126-137), and `shedverbs` emits the returned path. `friction` only logs; it never obtains a path. The Shed Verb-Set Invariant keeps "derives no path, imports no resolver" as `shedverbs`' no-derived-paths obligation. The planned amendment only "admits `internal/logger`", which leaves the invariant's text silent on a resolved path entering the package.
**Fix:** State in the decision, and in the invariant amendment, that the two logger accessors are the only admitted path sources and why that is not "deriving". Otherwise, tell `trace_dir` through `Spec` as `ScratchDir` is.

### [NIT:consistency] rollbackAdd local-branch premise is config-dependent
**Section:** `repair-scope` (stranded-branch exception, coverage list)
**Issue:** "`rollbackAdd` deletes the local warp branch (recorded as `branch_deleted`)" holds only with a non-empty `branch_prefix`. Under the default empty prefix, `ownedManagedBranch` refuses and the branch is left behind, with only a `Warn` (`internal/fabricengine/add.go` ~335-353). In that configuration the local-delete rule runs raw `git branch -D` over a destruction-gate ownership refusal. That contradicts the rationale "keeps the driver inside the same destructive gates".
**Fix:** State both configurations, and state that trace-proven `branch_created` substitutes for the gate's ownership check in this exception.

## Verdict

REQUEST_CHANGES
Absent-status baseline lacks a recipe-blind rule; the logger-admission rationale misstates what TraceDir does.
MILL_REVIEW_END
