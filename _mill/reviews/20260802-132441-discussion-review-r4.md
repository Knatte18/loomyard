MILL_REVIEW_BEGIN
# Review: webster: stop re-rendering already-inherited context into fork prompts

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: claude-opus-4.8
reviewed_file: _mill/discussion.md
date: 2026-08-02
```

## Findings

### [GAP] Recovery-prefix doc refs vs cwd-relative card pointer diverge
**Section:** Decisions › full-cold-recovery-prompt / card-pointer-relative-via-hubgeometry
**Issue:** The card pointer is composed `filepath.Rel(l.Cwd, join(WorktreeRoot,"_lyx/plan/NN.md"))` — so at `RelPath != "."` it renders `../_lyx/plan/NN.md`, yet the same recovery prefix instructs reading `_lyx/plan/00-overview.md`, `CONSTRAINTS.md`, `_pattern/PATTERN.md` as fixed worktree-root literals; from cwd=`layout.Cwd` (a subdir) these resolve to different, mutually-exclusive locations, and render.go's own comment (lines 103–110) treats `RelPath != "."` as the live case the plumbing exists for.
**Fix:** State the resolution — either give the prefix doc references the same cwd-relative composition via hubgeometry, or record that the recovery strand assumes/chdirs to `RelPath == "."` (worktree root) so bare `_lyx/...` tokens hold.

### [NOTE] Card-file read is newly required of the in-session fork
**Section:** Scope › Card content delivered by relative pointer
**Issue:** After fix #1 Master reads only `00-overview.md`, so card bodies are no longer inherited; the in-session fork must now `Read` its `NN-slug.md` — a new tool action the old inlined scheme avoided (correct and necessary, not redundant, but not stated as an accepted cost).
**Fix:** One sentence noting the fork's card-file read is the deliberate consequence of Master no longer pre-reading card files, distinct from the redundant PATTERN re-read being removed.

## Verdict

GAPS_FOUND
One unresolved geometry inconsistency between the cwd-relative card pointer and the prefix's fixed-token doc references.
MILL_REVIEW_END
