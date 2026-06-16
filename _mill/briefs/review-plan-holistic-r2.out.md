The new fsx/state packages use no cwd primitives, so the path invariant is satisfied. The enforcement test skips `_test.go` files. Note: the per-batch `verify:` runs only the package suite, not the enforcement test, but since fsx/state introduce no banned tokens this is fine.

Now I have enough to assess. Let me confirm a few remaining plan-accuracy points:

- Card 9 says `ReadJSON` on a missing path returns `found=false` AND asserts "the parent dir plus a `<path>.lock` file now exist." The plan's `ReadJSON` does `os.MkdirAll(filepath.Dir(path))` and `lock.AcquireReadLock(path + ".lock")`. The `flock.New` + `RLock` creates the lock file. So both the parent dir and lock file would exist. Consistent.

- Decision "internal/state API" ReadJSON: on `os.IsNotExist` return `(zero, false, nil)`. Card 8 matches.

- The fsx Decision says `PathError` is "the renamed `board.BoardPathError`." Confirmed BoardPathError exists.

Now, one subtle concern: Card 8 `ReadJSON` acquires a **read lock** but also does `os.MkdirAll` to create the parent so the lock file can live there. That mirrors muxpoc. Fine.

Let me check the All Files Touched list vs. the docs batch — it lists `docs/shared-libs/fsx.md` (created), `state.md`, `README.md`, `roadmap.md`. The overview "All Files Touched" omits nothing of note. Global step numbering: cards 1-13, sequential, unique. Batch DAG: batches 1-5, depends-on references valid, no cycles, files all present.

Let me verify the docs batch claim about roadmap milestone 3 line numbers (~29-34). Confirmed lines 29-34.

One thing to verify: Card 11/state.md rewrite says remove "A note on AtomicWrite / PathGuard" section — that section exists (lines 30-38). Good.

I have completed my verification. Let me compose the review.

MILL_REVIEW_BEGIN
# Review: Extract internal/fsx and build internal/state — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-16
```

## Findings

### [NIT] state_test models an internal-package test
**Location:** Batch 4 / Card 9
**Issue:** Card 9 lists `internal/muxpoc/state_test.go` as Context and is `package state_test` (external), but the model file is `package muxpoc` (internal, line 3) and exercises unexported helpers — a slightly misleading template, though the listed assertions only use exported `WriteJSON`/`ReadJSON`.
**Fix:** None required; the card already names the external package and the exported surface explicitly, so the implementer has stable identifiers.

### [NIT] Enforcement test not in any batch verify
**Location:** Batch 1/4 verify blocks
**Issue:** Per-batch `verify:` runs only the touched package suite, so `internal/paths/enforcement_test.go` (the path-invariant guard) is never run by the plan; new fsx/state code is unguarded against the ban at verify time.
**Fix:** Acceptable — fsx/state introduce no `os.Getwd`/`--show-toplevel` tokens (verified), so the invariant holds regardless; CI re-runs the full tree.

## Verdict

APPROVE — line cites, decisions, DAG, numbering, and behaviour-preserving moves all verify against source.
MILL_REVIEW_END