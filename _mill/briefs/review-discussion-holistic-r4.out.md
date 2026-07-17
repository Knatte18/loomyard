MILL_REVIEW_BEGIN
# Review: loom: Preflight phase (precondition validation)

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-17
```

## Findings

### [GAP] Check 3 junction anchoring disagrees with seed path
**Section:** seed-read-path / weft-pairing-composition
**Issue:** `LoomStatusFile()` is WorktreeRoot-anchored (`WorktreeRoot/_lyx/status.json`, RelPath dropped) for subdirectory robustness, but check 3's junction validation runs through `PairInSync` → `HostLyxLinkHere()` = `Join(WorktreeRoot, RelPath, _lyx)` = `Cwd/_lyx` (hubgeometry.go:531-532), so under subdir invocation (RelPath != ".") check 3 validates a *different* `_lyx` than check 4 reads — breaking both the "all five checks agree on one worktree regardless of cwd" claim and the "seed-unreadable, see check 3" attribution premise.
**Fix:** Resolve the divergence explicitly — either mandate Preflight runs at WorktreeRoot (RelPath ".") and drop the subdir-robustness justification, or note check 3's `PairInSync` junction check is Cwd-anchored and reconcile the two paths.

### [NOTE] Lock-exclusion mechanism mischaracterized
**Section:** strict-read-mechanism → lock-path
**Issue:** The discussion says loom's `*.lock` "inherits the same exclusion ... via the .gitignore/ScopedPathspec mechanism"; in fact there is no `*.lock` .gitignore entry and `ScopedPathspec` does not exclude — exclusion is a per-command `:(exclude)*.lock` pathspec token owned by the committing command (buildercli/weft.go:36), and Preflight commits nothing, so any weft-side exclusion is the future seed/loom-commit command's job (out of scope), while Preflight's own host-only clean check never sees a weft-tree lock through the junction.
**Fix:** Reword to state the lock poses no risk to *this* task's checks and that weft-commit exclusion belongs to the future committing command, not `.gitignore`.

## Verdict

GAPS_FOUND
Check 3 junction path is Cwd-anchored, contradicting the WorktreeRoot-anchored seed read and cross-check attribution.
MILL_REVIEW_END