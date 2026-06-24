MILL_REVIEW_BEGIN
# Review: Fix failing TestRunCLI in internal/worktree — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-24
```

## Findings

### [NIT] Overview "All Files Touched" list omits lyxtest.go
**Location:** `C:\Code\loomyard\wts\fix-worktree-runcli-test\_mill\plan\00-overview.md`
**Issue:** Card 8 in `02-config-family.md` edits `internal/lyxtest/lyxtest.go`, but that file is absent from the "All Files Touched" list in the overview — making the list inaccurate (15 entries instead of 16).
**Fix:** Add `internal/lyxtest/lyxtest.go` to the "All Files Touched" list in `00-overview.md`.

### [NIT] Duplicate card number between batch 2 and batch 3
**Location:** `C:\Code\loomyard\wts\fix-worktree-runcli-test\_mill\plan\02-config-family.md:111` and `C:\Code\loomyard\wts\fix-worktree-runcli-test\_mill\plan\03-board.md:21`
**Issue:** Both batch files label their first card "Card 8", creating an ambiguous reference if cards are cited by number across batches.
**Fix:** Renumber batch 3 cards to 10/11/12 (continuing from batch 2's Card 8 at line 111) or adopt per-batch numbering with a batch prefix.

## Verdict

APPROVE
All constraint checks pass; the bug fix and sweep are correctly implemented.
MILL_REVIEW_END
