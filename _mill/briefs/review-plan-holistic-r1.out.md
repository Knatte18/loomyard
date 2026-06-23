Now I have sufficient understanding. Let me also double check that `dispatch` baseDir `filepath.Join(l.WorktreeRoot, l.RelPath)` is the host worktree subpath that holds the `_lyx` junction — matching `HostLyxLinkHere()` parent. Yes, `HostLyxLinkHere()` = `filepath.Join(WorktreeRoot, RelPath, "_lyx")`, so its parent is exactly the baseDir. Consistent.

Let me finalize. Summary of findings:

1. BLOCKING: Card 14(e) e2e test design gap — CopyPaired host has no `_lyx`, so `config.Edit` → `FindBaseDir` fails before any write/sync; the test must run `w.Add` first to seed the junction.
2. NIT: Card 6 optional rename would make add.go an Edit (currently only Context).

MILL_REVIEW_BEGIN
# Review: weft producers: _lyx/config, lyx config, codeguide — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-23
```

## Findings

### [BLOCKING] e2e config write-through test cannot reach sync
**Location:** Batch 4 / Card 14 (e)
**Issue:** `CopyPaired`'s host worktree (`buildHostHub`) has no `_lyx`; running `dispatch` directly makes `config.Edit` call `FindBaseDir(baseDir=WorktreeRoot)` which returns the not-initialized error before any scaffold, write, or `weft sync` — so the test never exercises write-through to the weft repo nor the host-pristine assertion.
**Fix:** Have Card 14(e) first run `w.Add(slug, AddOptions{SkipPush:true})` (or otherwise seed the host `_lyx` junction → `<slug>-weft/_lyx`) so the edit lands through the junction, then assert the weft commit and pristine host.

### [NIT] Optional seeder rename would silently promote add.go to an Edit
**Location:** Batch 4 wrong — Batch 2 / Card 6
**Issue:** Card 6 lists `internal/worktree/add.go` under Context (read-only) but says renaming `seedLyxJunction`/`seedGitExclude` requires updating add.go's call sites; a rename therefore needs add.go in Edits, not Context.
**Fix:** Either drop the optional-rename clause (keep current names, add.go stays Context) or move `add.go` to Edits and note remove.go/remove_test.go are unaffected (verified: those files contain no references).

## Verdict

REQUEST_CHANGES
One blocking e2e test gap (junction never seeded) plus a minor card-field nit.
MILL_REVIEW_END