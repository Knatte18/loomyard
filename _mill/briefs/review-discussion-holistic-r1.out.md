All load-bearing claims verified against source. The discussion accurately describes the root cause (`applyExistingOverrides` only copies matching leaves; orphaned existing leaves vanish), the two plain-text success paths (`configcli.go:128` and `:186`), the contained caller graph (`configengine.Set` and `yamlengine.SetValues` each have a single production caller), the flat board template, the surviving `strings.Contains` assertions, and the existing docs files.

MILL_REVIEW_BEGIN
# Review: Fix lyx config --set dropping unrecognized keys + reconcile not detecting drift

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-02
```

## Findings

### [NOTE] Preserve path idempotency unspecified
**Section:** Decisions → set-preserve-and-warn / Testing
**Issue:** On a second `--set`, preserved keys are re-read from disk as existing leaves and re-grafted; the header comment (`# preserved (not in current template)`) attaches as a HeadComment on the first preserved node and could duplicate or grow on repeat runs — Reconcile documents idempotency but the preserve path does not.
**Fix:** State that a preserving `--set` is idempotent (stable bytes, no comment duplication on repeat) and add a repeat-run test case.

### [NOTE] Non-flat orphan behavior not pinned
**Section:** Decisions → flat-keys-only
**Issue:** `collectLeafPaths` emits dotted/indexed paths (`a.b`, `items[0]`); the decision says preservation handles flat top-level keys only but not what happens to a non-flat orphan if one appears on disk — silently dropping it would reintroduce the exact data-loss class this task fixes (currently impossible since all templates are flat, but unguarded).
**Fix:** Specify the guard: detect a non-flat existing-leaf path and either preserve verbatim or fail loudly — never silently drop.

## Verdict

APPROVE
Scope, decisions, and constraints are well-grounded and source-accurate; two non-blocking refinements noted.
MILL_REVIEW_END
