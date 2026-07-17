MILL_REVIEW_BEGIN
# Review: loom: Preflight phase (precondition validation) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-17
```

## Findings

### [BLOCKING] Card 8 healthy-pair scenarios never wire the _lyx junction
**Location:** Batch 2 / Card 8
**Issue:** The card seeds `_lyx/status.json` at `fixture.Layout.LoomStatusFile()` but never wires the host↔weft junction; `CopyPaired` does not create it, so `PairInSync` returns `junction missing` → `CheckJunction`, and the anchor `Report.OK` assertion (plus every seed-* scenario reachable only through a healthy pair) fails. Worse, seeding first creates a real `_lyx` dir, which then makes `WireJunctions` error (it rejects a real host `_lyx`).
**Fix:** Require a `warpengine.WireJunctions(l, slug)` step *before* seeding, seed through the resulting junction, and list `internal/warpengine/junction.go` in Card 8 `Context`.

### [MAJOR] Card 7 defensive `!found` branch returns a nil error
**Location:** Batch 2 / Card 7 (check 4)
**Issue:** `!found` after a good stat (a TOCTOU delete — `ReadJSONStrict` returns `(zero,false,nil)` on `IsNotExist`) is specified as "return `Report{}, err`", but `err`/`rerr` is nil there, yielding `Report{OK:false}` with empty `Failures` — violating the `OK == (len(Failures)==0)` invariant and the error-vs-Report contract (neither escalated nor a listed failure).
**Fix:** Return a synthesized non-nil error (escalate) for the defensive race branch, not the nil `err`.

### [NIT] Card 1 "read-only / creates no directory" is imprecise
**Location:** Batch 1 / Card 1
**Issue:** `lock.AcquireReadLock` still opens/creates the sidecar `.lock` file, and the "missing parent" test would return a wrapped lock-acquire error (parent absent ⇒ lock open fails), not a clean `(zero,false,nil)` miss — so `ReadJSONStrict` is "no `MkdirAll`", not fully side-effect-free.
**Fix:** Tighten the godoc/test wording to "creates no directory (a sidecar `.lock` is still taken)"; keep the missing-file case with an existing parent.

### [NIT] Card 8 not-a-git-repo scenario needs process-global chdir
**Location:** Batch 2 / Card 8
**Issue:** `Preflight()` reads the process cwd via `hubgeometry.Getwd()`, so exercising "run from a non-repo temp dir" requires `os.Chdir` (+restore), which is process-global and cannot run under `t.Parallel()` alongside the other scenarios.
**Fix:** Note the chdir/restore and that this scenario is non-parallel.

## Verdict

REQUEST_CHANGES
Sound plan; fix the Card 8 junction-wiring gap and Card 7 nil-error branch.
MILL_REVIEW_END
