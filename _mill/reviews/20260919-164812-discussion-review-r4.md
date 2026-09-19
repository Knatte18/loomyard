MILL_REVIEW_BEGIN
# Review: Seeded Shed core: run addressing, seed contract, batten

```yaml
duration_s: 137.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-20250514-class (self-assessed; brief dictates "opusmedium")
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] Prime's batten seed `recipe` conflated with Board `type`
**Section:** `seed-verb-plus-auto-seed-on-the-batten-entry-path` (and the matching Q&A)
**Issue:** "`lyx batten run <slug>` auto-seeds prime's own batten run when absent, copying the recipe from the Board task's `type`" gives prime's `_lyx/shed/<slug>/seed.json` a `recipe` of e.g. `loom`, so `lyx shed status <slug>` from prime would look that up in `shedcli`'s table and arm `loomcli` against prime — not batten. The Board `type` is the *child's* recipe, which `seed-child-writes-and-commits` separately says `Seed-Child` reads for the child's own seed.
**Fix:** State that prime's batten seed carries `recipe: batten` and that the Board `type` travels in that seed's `params` (or is read fresh by `Seed-Child`), and say which of the two is the source `Seed-Child` consumes.

### [NIT:consistency] Three `shed`-segment paths have no declarer under the new invariant
**Demoted-from:** BLOCKING
**Section:** Technical context (`lifecyclecli/paths.go`, `loomengine/config.go`), `run-shed-re-entrancy...`, new *Shed Run-Directory Invariant*
**Issue:** The invariant says "no other production file names the segment in path-construction context", but three paths are assigned elsewhere or to nobody: `PrimeRunLock` "stays a batten-owned path (under `.lyx/shed/` now)"; the `.lyx/shed/<run-id>/last-commit` marker has no named constructor; and `loomengine.LoomStatusRel`'s anchor-relative form (fed to `ScopedPathspec`, verified at `internal/loomcli/wiring.go:57`) has no stated new owner. The testing bullet's "durable four" also counts only three durable constructors.
**Fix:** Either extend `shedrun`'s constructor list to cover the prime run-lock, the last-commit marker and the anchor-relative status form, or carve each out in the invariant's own text by name.

### [NIT:consistency] `entry.Args` deletion mis-attributed to `lyx batten run`
**Demoted-from:** BLOCKING
**Section:** Technical context, `internal/shedcli` bullet
**Issue:** "`lifecycle`'s `cobra.ExactArgs(1)` becomes optional, so `lyx batten run` with no argument is now a `self` address" names the wrong site: deleting `table.entry.Args` changes the `lyx shed` path only. `internal/lifecyclecli/cli.go:126-128` sets `ExactArgs(1)` on batten's own `run`/`status`/`pause`, and `shedcli/parity_test.go`'s `TestParity_PositionalArgs` asserts both paths share that identical value.
**Fix:** State separately whether `battencli`'s own verbs relax to `MaximumNArgs(1)` (and gain `step`'s arity), and what `TestParity_PositionalArgs` asserts once the shared `Args` value is gone.

### [NIT:design] No-op commit skip makes the bounce budget non-durable
**Section:** `run-shed-re-entrancy-via-self-pointing-on-stuck`, known costs
**Issue:** The bounce budget is computed from the status file's trailing Stuck history (`shedengine/run_routing_test.go:612-636`), and the skip means those ~1440 entries are never committed — so a machine switch mid-`Run-Shed` resumes from a status with an empty bounce run and restarts the 12-hour budget from zero.
**Fix:** State this as a third known cost and say it is accepted (it extends the watch window rather than truncating it).

## Verdict

REQUEST_CHANGES
Three blocking issues: seed `recipe` value, unowned `shed`-segment paths, and mis-attributed arity change.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
