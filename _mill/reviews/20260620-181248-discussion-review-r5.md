MILL_REVIEW_BEGIN
# Review: Optimise and slim the test suite

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-20
```

## Findings

### [GAP] junction_test.go classified untagged but spawns on Windows
**Section:** Decisions › build-tag gating (Untagged list)
**Issue:** `junction_test.go`'s `TestCreateJunction` calls `createJunction`, which on the primary platform (Windows) spawns `cmd /c mklink /J` (`junction_windows.go:40`); listing it as "pure unit, no subprocess" contradicts the stated criterion and the "default untagged loop must be fully offline and subprocess-free" constraint.
**Fix:** Either tag `junction_test.go` `integration` on Windows, or state explicitly that this test stays untagged because its non-Windows `os.Symlink` path is non-spawning and the Windows `mklink` spawn is an accepted exception to the offline rule.

### [NOTE] Two same-named addWeftRemote helpers, opposite fates
**Section:** Decisions › conservative pruning; Technical context
**Issue:** `addWeftRemote` exists twice with different behaviour — worktree `testhelpers_test.go:166` (no push, verified uncalled → delete) and weft `sync_test.go:59` (real `git push -u`, actively used by `weft_integration_test.go`/`sync_test.go` → keep/migrate); the brief is correct but a plan writer skimming could delete the wrong one.
**Fix:** Make the delete-vs-keep distinction explicit per package path in the pruning decision.

### [NOTE] Success bar < ~5s default loop unmeasured/unbudgeted
**Section:** Testing › Success bar
**Issue:** The "< ~5s default" target has no current untagged-loop baseline; with junction/symlink-permission checks and remaining pure-unit tests, the number is asserted rather than grounded.
**Fix:** Note that the < ~5s figure is a target to confirm against a measured post-gating untagged baseline, not a precondition.

## Verdict

GAPS_FOUND
One classification contradicts the offline-loop constraint on Windows; all other claims verified against source.
MILL_REVIEW_END
