I have verified all the major claims in the discussion against source. The discussion is accurate: `SetPhase` silent no-op (store.go:352-353), `id_or_slug` vs `slug` key inconsistency (cli.go:137,159,180 vs store UpsertTask reads `slug`), `group` rejection in task.go:30,77, the proposal-only glob cleanup in render.go:41-52, `nextID()` starting at 0 (store.go:99-101), helptree pinning `set-phase` (helptree_test.go:51), and the `writeOp`→`RenderToDisk` seam (board.go:80).

MILL_REVIEW_BEGIN
# Review: Board fixes from sandbox run — payload keys, help, rerender

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: C:\Code\loomyard\wts\board-sandbox-fixes\_mill\discussion.md
date: 2026-06-28
```

## Findings

### [NOTE] upsert-batch top-level `{tasks}` wrapper not in allowlist
**Section:** Scope / Decision reject-unknown-keys (C)
**Issue:** The decision enumerates merge's top-level keys, single-target, set-deps, and inner upsert fields, but never the `upsert-batch` payload wrapper `{"tasks":[...]}`; a typo'd wrapper key (`"taks"`) currently decodes to an empty batch and silently succeeds with `count:0` (cli.go:115-124) — the exact W11 silent-no-op the task exists to kill.
**Fix:** State whether the `upsert-batch` wrapper rejects unknown top-level keys (and whether an empty/absent `tasks` is an error), so the "no silent drop on every entry point" principle is explicit for this entry point too.

### [NOTE] `get` not-found semantics (null vs error) unspecified
**Section:** Decision error-on-missing-target (Q2)
**Issue:** error-on-missing is scoped to `set-status`/`merge`/`remove`; `get` currently returns `task:null` on a genuinely-missing target (cli.go:189-192), and the discussion never states whether that null-on-miss behavior is retained after the key fix or also becomes an error.
**Fix:** One sentence confirming `get` keeps `task:null` for a valid-but-absent slug/id (a query, not a mutation), so a plan writer does not mistakenly make it error.

## Verdict
APPROVE
Thorough, source-accurate, decisions justified; two minor NOTEs to tighten entry-point coverage.
MILL_REVIEW_END
