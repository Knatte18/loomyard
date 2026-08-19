MILL_REVIEW_BEGIN
# Review: loom: session bootstrap

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4 class (Anthropic); exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-19
```

## Findings

### [BLOCKING:design] origin.json is written but never committed
**Section:** `parent-branch-is-recorded-never-guessed`, "The fabric change, concretely"
**Issue:** The stated rationale is that the record "survives a fresh clone on another machine", but the discussion specifies only a `state.WriteJSON` into the weft worktree — no weft commit, no `ScopedPathspec`, no `board.lock`; an uncommitted file under `_lyx` also contradicts "`_lyx` holds tracked content only".
**Fix:** State explicitly whether `Add` commits (and pushes) `origin.json` on the weft branch and through which `fabricengine` commit path, or drop the clone-survival rationale.

### [BLOCKING:design] mergestate.go is not the precedent claimed
**Section:** `parent-branch-is-recorded-never-guessed`
**Issue:** `mergestate.go:72` puts its record in the weft **gitdir** (`f.weftGitDir()`), machine-local and never tracked — it is a precedent for an ephemeral record, not for a tracked `_lyx/fabric/origin.json`; `corrindex.go` sits in the same gitdir.
**Fix:** Cite a genuine tracked-weft-file precedent or acknowledge this is a new class and specify its tracking/commit contract.

### [BLOCKING:design] origin.json read/write seam is unspecified
**Section:** "Seed inputs", `missing-record-refusal`
**Issue:** `_lyx/fabric/` is a new fabric-owned module subdirectory; per the Cwd Resolution Invariant its relative-path constant belongs to `fabricengine`, yet the discussion has `loomcli` both read it and (under `--parent`) write it with no named exported accessor/writer, and never says whether loom reads via the warp junction or `WeftWorktreePath`.
**Fix:** Name the exported `fabricengine` path accessor plus read/write functions loom calls, and pick the junction-vs-weft-path side.

### [BLOCKING:consistency] `loomshed.Seed` is not idempotent
**Section:** Scope ("the already-idempotent `loomshed.Seed`"), `self-seeding`
**Issue:** `internal/loomshed/seed.go:43-46` returns an error when the file exists — it refuses, it does not no-op. Re-entrancy (`reentrancy-ensure-and-attach`) requires a second `lyx loom run` to succeed, so blind `Seed` breaks it.
**Fix:** Restate the property accurately and specify the rule — probe-then-seed, or call `Seed` and tolerate only the already-exists error.

### [BLOCKING:decision] `Deps.WebsterRun` / `Deps.WebsterDeps` have no disposition
**Section:** "Technical context", Scope
**Issue:** Webster (row 10) is a **real** producer (`loomshed.go:108`, `webster.go:59-70`), and `shedadapters.NewWebsterProducer` defaults a nil runner to production `websterengine.Run` (`webster.go:48-54`), which would run with a zero `RunDeps` (Starter/Engine/ReedOps/Geom/Roles all nil). The discussion never says how `loomcli` fills them.
**Fix:** Decide — build the full `RunDeps` in `loomcli` (as `webstercli` does), or explicitly substitute a stub/refusing runner for this task — and put it in Scope.

### [BLOCKING:scope] `lyx run` becomes a registered "module" for two guards
**Section:** `run-alias-as-registered-command`, "Registration fallout"
**Issue:** `cmd/lyx/sandbox_coverage_test.go:41-47` enumerates every non-help/completion child of `newRoot()`; a top-level `run` command therefore needs its own `**Covers:** run` tag or an `excludedModules` entry, and `longlist_test.go:24` requires `root.Long` to contain `run`. Only `**Covers:** loom` is planned.
**Fix:** Add the `run` disposition (covers-tag or allowlist entry with reason) plus the `Long`-list mention to the fallout list and the Testing section.

### [BLOCKING:decision] `--parent` on an already-recorded worktree left undecided
**Section:** Testing, "Scenarios that must be covered somewhere"
**Issue:** The discussion explicitly defers refuse-vs-overwrite to the plan; overwriting a recorded provenance value silently re-targets the merge-back, which is the exact failure the whole decision exists to prevent.
**Fix:** Decide it here — refuse unless the recorded value matches, or require an explicit override flag.

### [NIT:consistency] Stub count is eight, not seven
**Section:** Scope Out
**Issue:** `internal/loomshed/stub.go:12-16` names eight stub rows (Discussion-Write, Discussion-Review, Plan-Sweep, Plan-Write, Plan-Review, Webster-Review, Publish, Finalize); the discussion says "all seven stubs".
**Fix:** Correct to eight.

### [NIT:consistency] Two small source-citation drifts
**Section:** "Technical context"
**Issue:** `parentBranch` is computed at `add.go:132`, not `:141` (141 is the `AppendRef` comment); and `loomengine.Preflight` is `func(cwd string) (Report, error)` — the `ShedProducer` adapter already exists as `loomshed.NewPreflightProducer(cwd)`, which the Deps list does not mention.
**Fix:** Fix the line reference and name `NewPreflightProducer` as the constructor loomcli calls.

### [NIT:consistency] "dead pane" rationale contradicts the detached design
**Section:** `reentrancy-ensure-and-attach` (rejected alternatives)
**Issue:** A second driver is spawned detached with output to `.lyx/loom/driver.log`, not into a pane, so "leaving a confusing dead pane" describes a shape this design does not have.
**Fix:** Reword to the actual consequence (a second process dying on `Shed.LockPath` with the failure only in `driver.log`).

## Verdict

REQUEST_CHANGES
Commit/ownership of the new fabric record, Webster deps, and the `run` alias fallout are unresolved.
MILL_REVIEW_END
