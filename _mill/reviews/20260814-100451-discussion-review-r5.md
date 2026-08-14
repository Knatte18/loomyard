MILL_REVIEW_BEGIN
# Review: Relocate producer prompt files into a stencils/ directory

```yaml
duration_s: 163.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-14
```

## Findings

### [BLOCKING:design] Seed/refresh trigger site is never named
**Section:** `seeding-trigger` + `stencilstore-ownership` + Constraints/Fabric Git
**Issue:** "on every lyx run that needs a stencil" names no owner: `stencilstore` gets only a `baseDir` so it can write but cannot commit, while the commit is a new `fabricengine` verb — and if the pass runs lazily inside `Read`, treadle's read (deep in `runJudgeCall`) would drag `fabricengine` into treadle's stack, which is the exact thing the Runner-Seam allowlist amendment is being justified against.
**Fix:** Decide and record whether the seed/refresh/commit pass runs once per process at a named composition point (which CLI entry points) or lazily at read, and state the resulting import direction between `stencilstore` and `fabricengine`.

### [NIT:consistency] `seeding-trigger` still says "committed through Bolt"
**Demoted-from:** BLOCKING
**Section:** `seeding-trigger` vs Constraints → Fabric Git Invariant
**Issue:** The decision body says the board write "is committed through `Bolt` like any other board write"; the Constraints section later says explicitly "**Do not use `Bolt` for the seeding commit**" (verified: `Bolt.Commit` → `commitWeftAt` is stage-all, `Bolt.Sync` takes `board.push.lock` while `boardengine` writes take `board.lock`, `internal/boardengine/sync.go:24`).
**Fix:** Rewrite the `seeding-trigger` decision to name the new `board.lock`-taking `fabricengine` verb, so a plan writer reading Decisions alone does not implement `Bolt`.

### [NIT:consistency] `stencilcli` kernel breaks the `<module>engine` naming rule
**Demoted-from:** BLOCKING
**Section:** `cli-surface` / Constraints → CLI / Cobra Invariant
**Issue:** The invariant states a cobra-registered package is `<module>cli` and its domain kernel `<module>engine`; this task pairs `lyx stencil` with `internal/stencilstore` (and `internal/stencil` already holds the name), and the scope list amends only the seam counts, not the naming rule.
**Fix:** State the disposition — either name the kernel `stencilengine`, or record the deviation and its reason in the CONSTRAINTS bullet in the same commit as the seam-count edit.

### [NIT:decision] Push disposition of the seeding commit unstated
**Section:** Constraints → Fabric Git Invariant
**Issue:** The new verb is specified to commit a positive pathspec under `board.lock`, but nothing says whether it pushes; the commit then sits on `weft:main` until some unrelated board write's `Bolt.Push` carries it, and two hubs seeding concurrently diverge outside `coalescePush`.
**Fix:** State explicitly that the seeding verb commits only and relies on board's next push, or that it pushes under its own coalescing path.

### [NIT:scope] Two behaviours decided but untested / unstated
**Section:** `port-back-is-mechanical-not-remembered`, `seeding-trigger`, Testing
**Issue:** The run-time drift `logger.Warn` is defined against "the worktree's `stencils/` source", but unlike `promote`/`diff --all` its behaviour with no source tree (a consumer repo) is unstated; and Testing covers dev-skip vs prod-refresh but not the decided "explicit `sync` refreshes even from a `-dev` build" row.
**Fix:** Say the warn is silently skipped when no source tree exists, and add the dev-binary-`sync`-refreshes assertion to the Dev/prod testing bullet.

## Verdict

REQUEST_CHANGES
Trigger site undecided; two superseded/deviating statements left standing in the artefact.
MILL_REVIEW_END
