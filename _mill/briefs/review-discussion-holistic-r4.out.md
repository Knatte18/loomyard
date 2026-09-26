MILL_REVIEW_BEGIN
# Review: Loom persists done only after post-run friction reflection

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewed_file: _mill/discussion.md
date: 2026-09-26
```

## Findings

### [BLOCKING:design] Lock-skip reopens the teardown race after a blocked resume
**Section:** Decisions § How the row reaches loomcli's reflection; Scope Out ("that path has no race").
**Issue:** A blocked halt's `PostRun` reflection holds `LoomFrictionLock` lock-free for up to `friction_timeout_min`, so an operator who resumes the child (`lyx loom start` sees a free run lock) and then batten inside that window gets `Friction-Reflect` hitting `reflectFriction`'s `!free` arm, which returns `skipped`, and then persists `done`, after which batten's teardown deletes `.lyx/loom/friction/` under the still-running blocked-path agent. This is the bug the task exists to fix, and "batten never tears down a blocked child" is only true until that child is resumed.
**Fix:** Decide the row's disposition when the friction lock is held: wait on it, route the row differently, or accept the residual in writing with its rationale; then pin that choice with a test.

### [BLOCKING:consistency] Reinvoke rationale contradicts the policy's own contract
**Section:** Decisions § Row naming and interrupt policy.
**Issue:** `InterruptPolicyReinvoke` is defined in `internal/loomshed/interruptpolicy.go` as "a re-invocation never double-spawns", and the table's doc comment justifies every Reinvoke row by an `Attach` probe; `frictionengine.Reflect` calls `Shuttle.Run` with no attach. The discussion instead justifies Reinvoke by tolerating duplicate issues on the run path. The policy governs an interrupted `lyx loom step`, where this row never spawns at all, and that is the premise that actually makes Reinvoke correct.
**Fix:** Restate the rationale on the step-path no-spawn premise, and put the interruptpolicy.go doc-comment justification (not only its row count) in the doc sweep.

### [NIT:design] Ordering and pause tests cannot reach the row through the unmodified fixture
**Section:** Testing § TDD candidate; § Pause at the row.
**Issue:** `buildSequenceFixture` (`internal/loomrecipe/fixture_test.go`) blocks the run at Publish by design and never invokes Finalize, so "drive loom's recipe to the end" in that fixture does not reach `Friction-Reflect`.
**Fix:** State that these tests substitute the Publish/Finalize producers after `New` (the fixture already uses this seam for row 1), or that they seed the status file at `Finalize`.

## Verdict

REQUEST_CHANGES
Blocked-then-resumed path still races teardown, and the interrupt-policy rationale contradicts its contract.
MILL_REVIEW_END
