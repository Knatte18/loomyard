# Orchestrator review — planparser-plan-dir (T1)

Reviewed against `manifest/designs/producers-standalone.md`'s T1 brief (told-geometry, wave 1).
Spot-checked every file/line reference in the discussion against current source — all confirmed accurate (`loomengine/config.go:32,40`, `planparser/parse.go:25,31,37`, `webstercli/cli.go:194` + its `loomengine` import, `cli_test.go:172`, `verbs_test.go:221,259`, `notransients_test.go:50-51`, `constructoranchoring_test.go:71-72,120-121`).

## Scope verdict: correct

The in-scope list is a faithful, near-literal match of the design's T1 Files list and brief.
Function signatures (`PlanDir(anchorPath string) string`, `PlanOverview(anchorPath string) string`), the delete-the-twins move, and the CONSTRAINTS.md reword are all pinned exactly as the design states them.
The `Out` section correctly fences off T7's territory (`--plan-dir`, standalone mode, `PersistentPreRunE` refactor) and correctly leaves the other `loomengine` path constructors (`Discussion*`/`LoomStatus*`) alone — those are genuinely loom-owned, not plan-format-owned.

Two additions go beyond the design's literal T1 Files line. Both are justified, bounded, and low-risk — not scope creep:

1. **Anchor-always test-gap closure** (flipping `AnchorRel` off `"."` in `cli_test.go`/`verbs_test.go`, new subpath-anchored `loomengine.PlanSpec` case). Confined to files the design *already* lists as touched; no new production files. Directly mitigates the one real regression risk this mechanical migration introduces (a caller silently passing `WorktreePath()` instead of `AnchorPath()` once the compile-time `*lyxcwd.Location` guarantee is gone). Explicitly does *not* touch `PersistentPreRunE` itself, correctly leaving that closure's coverage to T7 as stated.
2. **`doc.go` paragraph + `docs/overview.md` touch-up.** Not in the design's terse Files line, but justified under CLAUDE.md's Documentation Lifecycle rule (module contract changed: planparser now owns path, not just grammar). Both alternatives (skip either edit) are recorded as explicitly rejected with reasons — this was a deliberate call, not an oversight.

Neither addition collides with sibling tasks: T2 (config loaders) is untouched territory, and the one real T3 adjacency (`cli.go:194` vs. T3's `cli.go:179-181`) is correctly identified and already flagged as a mechanical rebase-on-conflict, matching the design's own note verbatim.

## Minor notes (non-blocking)

- Line range for the deleted functions is given as `internal/loomengine/config.go` "lines ~29-42" in the Scope section; actual source has `PlanDir` at line 32 and `PlanOverview` at line 40 (ending ~42). The design brief itself says "~32-42". Cosmetic — the code excerpt quoted later in the doc is correct — but worth tightening so a future reader doesn't chase a wrong anchor.
- "Project rules" says this task "never [pushes] to main" — true, but slightly underselling the actual target: per `parent: standalone-producers` in this file's own frontmatter, `mill-merge` lands this on `standalone-producers`, not `main`, directly. Not wrong, just could be more precise.

## Bottom line

No scope violations found. Ready to proceed to planning as written.
