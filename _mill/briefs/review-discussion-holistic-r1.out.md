MILL_REVIEW_BEGIN
# Review: Audit the remaining leaf and seam import invariants

```yaml
verdict: GAPS_FOUND
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-08
```

## Findings

### [GAP] Deferred rename has an unlisted dependent site
**Section:** Technical context ("The existing lyxtest test") vs Audit results (sweep, "not touched")
**Issue:** `internal/shuttleengine/seam_enforcement_test.go:22` names the function — "in the style of internal/lyxtest/leaf_enforcement_test.go's TestLeafInvariant" — so the discussion's verdict that this site "stays accurate, not touched" holds only if the rename to `TestLeafInvariant_AllowlistOnly` does *not* happen, which is the very question left to mill-plan.
**Fix:** Decide the rename in discussion, or state explicitly that renaming pulls `shuttleengine/seam_enforcement_test.go:22` into scope.

### [GAP] Renaming needs a third CONSTRAINTS.md edit the scope forbids
**Section:** Work already landed / Scope
**Issue:** `CONSTRAINTS.md:40` reads "**Enforced by** `internal/lyxtest/leaf_enforcement_test.go`." with no test-function name, unlike all six sibling entries (lines 48, 65, 74, 81, 265); a rename — or plain consistency — requires editing that line, but the discussion declares the CONSTRAINTS.md work done and instructs the plan not to redo it.
**Fix:** Say whether line 40 gains the test name in mill-go, and if so list `CONSTRAINTS.md` as an in-scope mill-go edit rather than "already landed".

### [NOTE] Sweep method is not reproducible
**Section:** Audit results ("Sweep for other now-invalid claims") / Testing
**Issue:** Testing makes "no remaining comment in the tree asserts an isolation property the import graph does not provide" a reviewer obligation with the sweep list as the checklist, but no search pattern or command is recorded, so a plan writer or reviewer cannot re-derive or extend the list.
**Fix:** Record the grep/pattern used (e.g. bare-token scan for `lyxcwd`/`never imports` across `internal/**/*.go`) alongside the sweep results.

### [NOTE] Treadle's own seam test header is on neither list
**Section:** Audit results (sweep; verified-untouched list)
**Issue:** `internal/treadleengine/seam_enforcement_test.go:4,8` restates the treadle rule ("never internal/lyxcwd as a direct import", "a convenience lyxcwd import") — verified accurate as written, but it is absent from both the in-scope list and the deliberately-untouched list that exists precisely so a reader can tell "checked" from "missed".
**Fix:** Add it to the verified-untouched list with its one-line reason ("as a direct import" already carries the correct reading).

### [NOTE] lyxtest/doc.go stale span understated
**Section:** Scope ("`internal/lyxtest/doc.go` (~lines 7-9)")
**Issue:** The denylist framing runs to line 13, not 9 — lines 10-13 continue "It must not import internal/configreg or any feature package … a configreg or feature import would close a test-build cycle", which is denylist-shaped restatement of what the allowlist will imply.
**Fix:** Widen the cited range to ~7-13 so the plan rewrites the whole block rather than the first sentence.

### [NOTE] "Six" vs "seven" enforcement-test count is inconsistent
**Section:** Technical context / "lyxtest converts to an allowlist"
**Issue:** "the other six enforcement tests" includes `scoutengine`'s, which is out of scope and being changed concurrently by `scout-seam-conversion`, so the shape being matched may not be stable at merge time.
**Fix:** Name `internal/pattern` (already cited as the model) as the sole shape reference and drop the count.

## Verdict

GAPS_FOUND
Rename decision and its CONSTRAINTS.md/shuttleengine dependents must be settled before planning.
MILL_REVIEW_END
