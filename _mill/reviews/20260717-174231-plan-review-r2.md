MILL_REVIEW_BEGIN
# Review: Extend codeintel lookup to non-Go languages via LSP — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-17
```

## Findings

### [NIT] refs subcommand lacks positional-arg validation
**Location:** Batch 3 / Card 13
**Issue:** The `refs` verb takes one positional (`<symbol|file:line:col>`) but no `Args:` constraint is specified, so bare `lyx codeintel refs` or a 2-arg call is unguarded and won't emit the clean JSON envelope Card 14 exercises.
**Fix:** Pin `Args: cobra.ExactArgs(1)` on the `refs` command in Card 13's Requirements.

### [NIT] "hub base" for LoadRegistry overlay is ambiguous
**Location:** Batch 3 / Card 13
**Issue:** Requirements say load the overlay via `LoadRegistry(<hub base>)` after `hubgeometry.Resolve(cwd)`, but `_lyx/config/servers.yaml` resolves under a worktree root, not `Layout.Hub` (the container); "hub base" could mislead the implementer into passing `l.Hub`, silently missing every overlay (degrades to builtins, no crash).
**Fix:** Name the exact `hubgeometry.Layout` field to pass as `baseDir` (e.g. `l.WorktreeRoot`), matching how weftcli builds its config base from the worktree, not the hub.

## Verdict

APPROVE
Plan is complete, constraint-clean, and faithfully mirrors modelspec; only two minor specificity NITs.
MILL_REVIEW_END
