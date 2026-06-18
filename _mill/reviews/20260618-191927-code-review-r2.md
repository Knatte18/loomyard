I have now read every file in the manifest. I have a complete picture. Let me compile the findings.

Key observations:

1. **Batch 1 (terminology-rename)** — All Go source and test files have been updated. `paths.go` has `Hub`, `Prime`, `PrimeName()`. All test files correctly use the new names. `launchers_test.go` has no stale `Container` comment. One stale comment remains in `portals_test.go` line 205: `// This should collapse to <Container>/_portals/<slug>` — should say `<Hub>`.

2. **Batch 2 (config-path-migration)** — `config.go` reads from `_lyx/config/<module>.yaml`. `init.go` creates `_lyx/config/` and writes there. All test fixtures use `_lyx/config/`. The regression test `TestLoad_OldFlatPathNotPickedUp` is present. All comments in `concurrency_test.go` were updated to reference `_lyx/config/board.yaml`. The `init_test.go` package comment was fixed. The `config_test.go` comment at line 20 now reads `_lyx/config/worktree.yaml`. These were all the R1 nits — they are fixed.

3. **Batch 3 (docs-and-roadmap)** — All doc files updated. No old field names, no old config paths in any doc. CONSTRAINTS.md has `PrimeName()` and "hub" terminology. `paths.md` has portal deprecation notices. `overview.md` has the weft overlay model section. `roadmap.md` has tasks 005 (named 4 per the roadmap's original numbering which was kept), 5 (weft engine), 6 (hub-creator), 7 (codeguide). `worktree.md` has the weft pointer and portal deprecation. `board.md` has `_lyx/config/board.yaml` throughout.

4. **Topology diagram concern** — The `docs/overview.md` topology shows `<prime>/<host-repo>/` and `<prime>-weft/<weft-repo>/` as nested sub-dirs under each worktree folder. The discussion says host and weft worktrees ARE the checkout directories directly (e.g. `<hub>/<prime>/` is the git root, not `<hub>/<prime>/<host-repo>/`). This nesting is architecturally inaccurate per the Layout model where `WorktreeRoot = filepath.Clean(git rev-parse --show-toplevel)` and `Hub = filepath.Dir(WorktreeRoot)` — the worktree directory IS the repo root, not a container for it.

5. **`portals_test.go` line 205 stale comment** — `<Container>` should be `<Hub>` per the rename. Card 5 explicitly listed lines 37, 99, 139, 189 for portals_test.go but missed line 205.

6. **`config_test.go` lines in the `TestLoad_OldFlatPathNotPickedUp` function** — Comments and `t.Fatalf` message at lines 534 and 538 intentionally reference `_lyx/board.yaml` as the OLD path being deliberately written (not a migration miss) — this is correct.

MILL_REVIEW_BEGIN
# Review: Weft repo — companion-repo overlay for lyx — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-18
```

## Findings

### [BLOCKING] Topology diagram shows nested repo dirs, contradicting Layout model
**Location:** `C:\Code\loomyard\wts\weft-repo\docs\overview.md:63-72`
**Issue:** The diagram shows `<prime>/<host-repo>/` and `<prime>-weft/<weft-repo>/` as sub-directories, but the actual `paths.Layout` model has `Hub = filepath.Dir(WorktreeRoot)` meaning each worktree directory IS the git repo root — there is no extra nesting level.
**Fix:** Remove the inner `<host-repo>/` and `<weft-repo>/` lines; the worktree directories are the repo roots directly (matching the `paths.Resolve` contract and the worktree.md layout diagram, which has no sub-nesting).

### [NIT] Stale `<Container>` in portals_test.go comment
**Location:** `C:\Code\loomyard\wts\weft-repo\internal\worktree\portals_test.go:205`
**Issue:** Comment reads `// This should collapse to <Container>/_portals/<slug>` — Card 5 updated lines 37, 99, 139, 189 but missed line 205.
**Fix:** Change `<Container>` to `<Hub>` on line 205.

## Verdict

REQUEST_CHANGES
One blocking doc inaccuracy (topology diagram nesting) and one missed rename nit.
MILL_REVIEW_END