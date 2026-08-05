MILL_REVIEW_BEGIN
# Review: fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 5 (claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-08-05
```

## Findings

### [GAP] `_board` re-wire path is unreachable when only it is broken
**Section:** `board-junction` (2nd decision) + Testing bullet "`_board` junction wiring"
**Issue:** The claimed "silent idempotent repair on the next reconcile" pins the wiring to `reconcile.go:141`, but that line sits inside `if !junctionHealthy` (`reconcile.go:130-150`), and `checkJunctionHealth` deliberately iterates only `HostJunctionsHere(RepoWiredNames(l))`, which excludes `_board` — so a broken `_board` link alone leaves the pair `AlreadyHealthy` (`:149`) and is never re-wired; the "re-created by reconcile when missing or mispointed" test would fail as designed.
**Fix:** State that the explicit `_board` wiring runs unconditionally in the pair loop (alongside `applyStaleRemoval` at `reconcile.go:152`), not inside the `!junctionHealthy` branch, and reconcile the "wire-only and unmonitored" wording with it.

### [GAP] Batch-1 `Prime` cut-over undercounts its dependents
**Section:** `batching` (2nd decision) / `prime-and-list-move`
**Issue:** Two errors: `LauncherSpawnRel` (`hubgeometry.go:458-461`) uses no `Prime` at all — the second in-module `Prime` user at `:466` is `MenuLauncherRel`; and `WeftRepoRoot` (`:476-478`) calls `l.PrimeName()`, so dropping `Prime` in batch 1 also breaks it and its ~10 production call sites (`add.go:108,149`, `cleanup.go:176,208`, `reconcile.go:172,204,227`, `prune.go:137,152`, `weftwiring.go:36,55,69,126`) — none budgeted, and it contradicts "batches 2-4 change no signature".
**Fix:** Correct the function name and add `WeftRepoRoot` (plus its fabricengine callers) to the batch-1 prime-threading list.

### [GAP] `_lyx` ownership map may be vacuous against the actual guard
**Section:** `enforcement-rewrite` (token table, `_lyx` row) / `per-module-constructors`
**Issue:** The guard matches whole tokens only (`enforcement_test.go:222-228`, exact `strconv.Unquote` equality), so a module declaring its "private relative-path constant" as `"_lyx/plan"` — the natural form for the relative constants `per-module-constructors` calls for — never trips it and needs no map entry, making batch 2's "register the `_lyx` per-module owners" register nothing.
**Fix:** Pin the constant form each relocated constructor uses (bare `_lyx` token joined per-segment vs. a single `"_lyx/<sub>"` literal) and state which one the ownership map is asserting over.

### [GAP] `ResolveWithAnchor`'s gate behaviour is unspecified
**Section:** `anchor-read-ownership` / `strict-anchor-gate`
**Issue:** `strict-anchor-gate` names only `Resolve`, and `ResolveWorktree` is explicitly ungated, but `ResolveWithAnchor(cwd, anchor)` — the new seam `fabricengine` clone and `lyxtest` both use — is never said to gate or not; with an injected non-`"."` anchor the strict equality check would reject a caller standing at the worktree root.
**Fix:** State explicitly whether `ResolveWithAnchor` applies the gate helper, and add the corresponding case to the strict-gate table test.

### [GAP] Doc-update list omits files that name the package normatively
**Section:** Scope "In" (last bullet) / `batching` batch 5 / Technical context "Discovered during discussion"
**Issue:** Beyond the four named docs, `manifest/designs/pattern.md:18,62` and `manifest/designs/loom.md:103` (module docs for packages receiving moved constructors), `docs/reference/plan-format.md:38`, `builder-contract.md:170`, `discussion-format.md:14`, `status-schema.md:9`, `model-spec.md:32` (names `hubgeometry.ConfigFile`, which moves to `configengine`) and `docs/shared-libs/README.md` all assert `internal/hubgeometry` ownership or the Hub Geometry Invariant by name.
**Fix:** Enumerate these in batch 5, per the Documentation Lifecycle's same-commit rule.

### [NOTE] In-module callers of the departing `WorktreePath(slug)`
**Section:** `location-struct` (name-collision decision)
**Issue:** Its callers are listed as `fabricengine` plus `ideengine/spawn.go:20`, but four callers live inside the module itself and stay there until batch 3: `LauncherSpawnRel:459`, `HostLyxLink:524`, `HostPatternLink:537`, `HostJunctions:570`.
**Fix:** Note that batch 1 rewrites those to an inline `filepath.Join(l.HubPath, slug)`, since the module cannot import `fabricengine`.

## Verdict

GAPS_FOUND
Five gaps: `_board` repair path, batch-1 prime scope, guard token form, `ResolveWithAnchor` gating, docs.
MILL_REVIEW_END
