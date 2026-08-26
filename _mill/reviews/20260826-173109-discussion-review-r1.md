# Review: loom's status file can conflict on the landing merge

```yaml
verdict: REQUEST_CHANGES
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [BLOCKING:design] Consumer enumeration only searched Go source, missing a live-agent stencil that hardcodes the old path
**Section:** Technical context — "Where the path is declared and consumed": "`internal/loomengine/config.go` is the sole declarer… None of them build the path themselves, so the move is genuinely one constructor plus its two guard tests."
**Issue:** The enumeration method only traced `shedPaths.StatusPath` wiring through Go source. `contracts/stencils/loom/loom-rubric-webster-review.md` — wired live into two rows of `contracts/recipes/loom-recipe.yaml` (`rubric_stencil: loom-rubric-webster-review`, lines 241 and 285) — instructs the Webster-Review agent in plain prose: `1. Read `_lyx/loom/status.json` and take `product.parent`...` and `If `_lyx/loom/status.json` cannot be read... raise a BLOCKING finding`. This is a real agent's live prompt text, not a Go constructor call, so it is invisible to the stated enumeration method — and after the move it will read from a path that no longer exists on every Webster-Review round, forcing a spurious BLOCKING finding into every review segment of every future loom run. The same gap (text references to the literal path, outside any constructor call) also leaves stale prose in `contracts/recipes/loom-recipe.yaml:293` (a comment on why tool-use is required for that row), `internal/loomengine/status.go:8` and `report.go:25-39` (doc comments describing what `CheckSeed` validates), `internal/loomshed/seed.go:2`, `manifest/designs/shed.md:245`, `manifest/designs/self-report.md:15`, and `tools/sandbox/SANDBOX-CORE-SUITE.md:245` (a fixture note for scenario S8, tagged `**Covers:** loom` per the Sandbox Suite Coverage invariant) — none of these are named in the "In" scope list or the doc-update list.
**Suggested fix:** Add an explicit scope item: grep the full tree (not just Go call sites) for the literal string `_lyx/loom/status.json` before implementation, and update every hit — the stencil is the one with real behavioral consequences and must be in scope regardless of what else is found; the rest are doc/comment drift. State this as the enumeration method (a full-text grep, not a constructor trace) rather than re-deriving the list from Go source alone.

## Verdict

REQUEST_CHANGES
The consumer enumeration missed a live agent stencil hardcoding the old path, which breaks Webster-Review on every future run unless added to scope.
