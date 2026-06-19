MILL_REVIEW_BEGIN
# Review: Reconcile stale design docs (stateless + weft model) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-19
```

## Findings

### [BLOCKING] mux.md v1 section retains worktree-registry framing

**Location:** `docs/modules/mux.md:132` and `:157`

**Issue:** The `mux-registry-semantics` shared decision requires mux.md to be free of worktree-registry framing throughout; the top-level intro (L21-22) was correctly fixed to "derives worktree layout from `git worktree list`", but the v1 section was not swept: L132 says "Each active worktree (from the registry) owns one full-height column" and L157 says "Reconcile the psmux window against the worktree registry: a column per worktree in registry order". These are plain worktree-registry coupling — the batch-2 broken-term test explicitly requires mux.md to be free of this framing.

**Fix:** Replace "from the registry" on L132 with "from `git worktree list`" and reword the `mux sync` description on L157 to derive layout from `git worktree list` instead of a registry.

### [NIT] worktree package doc-comment omits "removed in task 006"

**Location:** `internal/worktree/cli.go:9`

**Issue:** Card 13 requires the worktree package doc-comment to state "portals are deprecated-but-present (removed in task 006)"; the comment says "deprecated-but-present in the codebase" but omits the task-006 removal qualifier, breaking cross-doc consistency with `docs/overview.md`, `docs/shared-libs/paths.md`, and `CONSTRAINTS.md` which all include the task-006 callout.

**Fix:** Change "deprecated-but-present in the codebase" to "deprecated-but-present; removal planned for task 006".

## Verdict

REQUEST_CHANGES
One blocking issue: worktree-registry framing survives in mux.md's v1 section.
MILL_REVIEW_END