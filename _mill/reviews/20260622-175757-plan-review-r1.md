MILL_REVIEW_BEGIN
# Review: Extract internal/vscode; keep ide IDE-generic — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-22
```

## Findings

### [NIT] spawn.go doc comment lines 20-22 left stale after rewire
**Location:** Batch 1 / Card 3
**Issue:** Card 3 mandates the code changes in `spawn.go` (`pickColor` to `vscode.PickColor`, `writeVSCodeConfig` to `vscode.WriteConfig`) but does not call out updating the `Spawn` doc comment (lines 20-21) that still names `pickColor` / `writeVSCodeConfig`, leaving it referencing now-deleted unexported symbols.
**Fix:** Add to Card 3 a note to refresh the `Spawn` doc comment's step list to the `vscode.*` names.

### [NIT] cli.go package doc retains baked-value claim about palette/settings
**Location:** Batch 1 / Card 3
**Issue:** Card 3 rewrites the `// Package ide` doc to delegate VS Code specifics to `internal/vscode`, but the existing doc's last sentence ("Mill values (palette, settings keys, cmd /c code) are baked") describes details that now live in `internal/vscode`; the card does not explicitly direct whether that sentence moves or is dropped.
**Fix:** Specify in Card 3 that the baked-values sentence is removed from `ide`'s doc (or relocated to the `vscode` package doc on `config.go`).

## Verdict

APPROVE
Plan is accurate, complete, well-sequenced; only two cosmetic doc-comment nits.
MILL_REVIEW_END