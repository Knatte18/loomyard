The roadmap uses milestone numbers 5, 6, 7 for what the plan calls "tasks 006, 007, 008." The plan says "add milestone entries for task 006, task 007, and task 008" but the roadmap numbered them 5, 6, 7 instead of having the word "task 006" etc. The portals deprecated note says "task 006" and milestones are numbered 5/6/7 respectively. The Card 14 requirement says "add milestone entries for task 006 (weft engine), task 007 (hub-creator / `lyx-clone` skill), and task 008 (`_codeguide` junction)." The actual roadmap entries match the descriptions correctly for content but use sequential milestone numbers (5, 6, 7) rather than having "task 006" as labels. This is fine — the roadmap document has always used numbered milestones and having item 5 = task 006 is consistent with the existing scheme. This is not a defect.

I now have enough to produce my review.

MILL_REVIEW_BEGIN
# Review: Weft repo — companion-repo overlay for lyx — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-18
```

## Findings

### [NIT] Stale `<Container>` in LauncherSpawnRel godoc
**Location:** `C:\Code\loomyard\wts\weft-repo\internal\paths\paths.go:178-179`
**Issue:** The `LauncherSpawnRel` godoc still reads `<Container>/_launchers` and `<Container>/<slug>` — Card 1 requirement (6) explicitly required updating all method doc comments to the new names.
**Fix:** Replace both `<Container>` occurrences with `<Hub>` in the method's doc comment.

### [NIT] Stale `_lyx/board.yaml` in board.md prose
**Location:** `C:\Code\loomyard\wts\weft-repo\docs\modules\board.md:227`
**Issue:** Line 227 reads `defaults overlaid with \`_lyx/board.yaml\`` — the old flat path — while line 234 in the same section correctly uses `_lyx/config/board.yaml`; Card 16 required updating all such references.
**Fix:** Change `_lyx/board.yaml` on line 227 to `_lyx/config/board.yaml`.

### [NIT] Stale `# _lyx/board.yaml` in config.md YAML example
**Location:** `C:\Code\loomyard\wts\weft-repo\docs\shared-libs\config.md:55`
**Issue:** The YAML code-block header comment reads `# _lyx/board.yaml`; Card 17 required updating every `_lyx/board.yaml` reference in this file.
**Fix:** Update the comment to `# _lyx/config/board.yaml`.

### [NIT] Stale comments in concurrency_test.go
**Location:** `C:\Code\loomyard\wts\weft-repo\internal\board\boardtest\concurrency_test.go:36,112,153`
**Issue:** All three in-function comments say `seedWiki creates _lyx/board.yaml` — the old path — while `seedWiki` (in `bench_test.go`) was correctly migrated to `_lyx/config/board.yaml`. Card 11 noted these are call-site-only lines with no independent path strings, but the comments were not updated.
**Fix:** Update each comment to `seedWiki creates _lyx/config/board.yaml`.

### [NIT] Stale package-doc comment in init_test.go
**Location:** `C:\Code\loomyard\wts\weft-repo\internal\board\init_test.go:3`
**Issue:** File's package-level comment says "Covers: creating `_lyx/board.yaml` and .gitignore managed block" — the old flat path; Card 9 required updating init_test.go for the new paths.
**Fix:** Change `_lyx/board.yaml` to `_lyx/config/board.yaml` in the package comment.

### [NIT] Stale inline comment in worktree/config_test.go
**Location:** `C:\Code\loomyard\wts\weft-repo\internal\worktree\config_test.go:20`
**Issue:** Comment reads `// yaml, when non-empty, is written to _lyx/worktree.yaml` while the actual fixture (line 47) correctly writes to `_lyx/config/worktree.yaml`; Card 12 updated the fixture paths but missed the descriptive comment.
**Fix:** Update the comment to say `_lyx/config/worktree.yaml`.

## Verdict

REQUEST_CHANGES
Six NIT-level stale comments/doc references; no blocking issues. All Go source logic, renames, and test fixtures are correctly implemented.
MILL_REVIEW_END