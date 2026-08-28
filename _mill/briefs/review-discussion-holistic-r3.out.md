MILL_REVIEW_BEGIN
# Review: reed: resume/down leak lock directories at the stale pre-rename session-name path

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5 (Anthropic Claude Opus 5)
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:design] WorktreeRoot's told shape is never validated
**Section:** §Decisions "the-predicate-is-worktreeroot-exists-not-anchorpath-exists" / §Constraints
**Issue:** `Geometry.WorktreeRoot` today has no shape validator anywhere in `internal/reedengine` (verified: it appears only in `strand.go:176-177` and as message decoration inside `validateToldTmuxIdentity`/`validateToldAnchorPath`), yet this task promotes it to the load-bearing control-flow predicate; the discussion states no disposition for an empty or relative value. An empty `WorktreeRoot` makes `os.Stat("")` return `fs.ErrNotExist`, which under the chosen semantics yields the terminal sentinel — a permanent watchdog park plus a "this worktree was renamed" message about `""` — and a relative value silently stats against whatever cwd the process happens to have, exactly the failure `validateToldAnchorPath` was written to backstop for `AnchorPath`.
**Fix:** Decide and record whether the new live validator first rejects empty/non-absolute `WorktreeRoot` as a distinct told-contract error (non-sentinel, like `validateToldAnchorPath`'s), or whether `Geometry`'s contract is explicitly declared to leave it unchecked.

### [BLOCKING:scope] Fixture inventory enumerates callers, not op-running tests
**Section:** §Scope "Test-fixture update" / §Technical context (`lock_test.go:29`) / §Testing "Fixture"
**Issue:** The inventory method is "callers of `newTestEngine`" ("14 test files call it; a single helper change covers all of them"), but the package also contains op-running tests that build their own inline `Geometry` literal with an uncreated `WorktreeRoot` — `lock_test.go`'s `TestWithOpLock_PathIsUnderDotLyx` sets `worktreeRoot := filepath.Join(hub, "worktree")` without `MkdirAll` and then calls `e.withOpLock`, so the helper change does not cover it. The enumeration criterion is wrong, not merely short by one entry.
**Issue is method-level; not an enumeration of individual files.**
**Fix:** Restate the fixture scope as "every in-package test that reaches `withOpLock`/`withTryOpLock`, whether via `newTestEngine` or an inline `Geometry` literal", and say which of the two the inline sites adopt (materialize the root, or migrate onto the helper).

### [NIT:decision] "Stat seam" named without a disposition
**Section:** §Testing, unit bullet on non-`ErrNotExist` stat failure
**Issue:** "inject at the stat seam rather than skipping the case" references a seam that does not exist in `reedengine` today; whether the fix introduces an injectable stat indirection (a package var) or the test provokes a real EACCES is left open, and the former would be a production-code change not listed in §Scope.
**Fix:** State which of the two the plan takes, and if a seam is introduced, list it in §Scope.

## Verdict

REQUEST_CHANGES
Predicate field lacks shape validation; test-fixture enumeration criterion misses inline-geometry op tests.
MILL_REVIEW_END
