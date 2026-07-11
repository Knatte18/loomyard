MILL_REVIEW_BEGIN
# Review: Build builder - the batch-implementation loop — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-11
```

## Findings

### [BLOCKING] Validate never receives worktreeRoot for check 5
**Location:** Batch 1 card 6 (consumed by batch 6 card 25, batch 7 card 27)
**Issue:** The pinned signature is `Validate(plan *Plan, caps ValidateCaps) []ValidationError` and `ValidateCaps` carries only `ContextCapTokens`/`CardCap`, yet check 5 (`batch-oversized`) must sum on-disk byte sizes of Scope+Where files "resolved against a caller-supplied `worktreeRoot string` argument" — a path that exists in neither the signature nor `ValidateCaps`; cards 25/27 build caps "from Config," and `Config` has no worktree path. Check 5 cannot resolve files, and if the implementer adds a third param the later-batch callers won't compile.
**Fix:** Add `worktreeRoot` to the `Validate` signature (or to `ValidateCaps`) in card 6 and specify cards 25/27 pass `layout.Cwd`/`worktreeRoot`.

### [MEDIUM] poll gather computes the git diff on every running tick
**Location:** Batch 4 card 18 + batch 7 card 28
**Issue:** Card 18 pins "a `running` snapshot never touches git," but `Classify` takes `Changed`/`Dirty` as eager `ClassifyInputs` fields and card 28 wires "diff/dirty via the gitquery helpers" into the per-tick (1s) gather closure `PollUntilTerminal` calls — a literal implementation runs `git diff` up to hundreds of times per poll, violating the pinned rule.
**Fix:** Specify the gather closure computes diff/dirty lazily, only inside the report-present terminal branch, never before every `Classify` call.

### [NIT] Distill duplicates report.OutOfScope as a separate param
**Location:** Batch 3 card 15
**Issue:** `Distill(report, changed, scope, outOfScope, dirty)` passes `outOfScope` separately although `report.OutOfScope` already carries it; card 18's `Classify` would feed `report.OutOfScope` back in, duplicating the field.
**Fix:** Drop the `outOfScope` parameter and read `report.OutOfScope` inside `Distill`.

### [NIT] Card 30 lists dynamic guards as Edits
**Location:** Batch 8 card 30
**Issue:** `registration_test.go` (AST-based) and `longlist_test.go` (derives from `newRoot().Commands()`) carry no hardcoded sets and need no edits; only `helptree_test.go` has a pinned `requiredModules`/`wantSubs` to extend.
**Fix:** Drop `registration_test.go`/`longlist_test.go` from Edits (keep the "run the suite, fix any drift" step); note only `helptree_test.go` needs a pin update.

## Verdict

REQUEST_CHANGES
One blocking contract gap (Validate/worktreeRoot) plus a git-on-running-tick tension; otherwise sound.
MILL_REVIEW_END
