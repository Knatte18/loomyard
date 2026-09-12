MILL_REVIEW_BEGIN
# Review: lyx loom step + external supervisor skill

```yaml
duration_s: 129.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-class (self-assessed; exact build unknown)
reviewed_file: _mill/discussion.md
date: 2026-09-12
```

## Findings

### [BLOCKING:design] Early run-lock probe precedes the MkdirAll it needs
**Section:** § Decisions → `step-bootstraps-idempotently-but-never-spawns-a-driver`, "Ordering against a live driver" **Issue:** the run lock is `.lyx/loom/run.lock` (`loomengine.LoomRunLock`), and `run` only creates that ephemeral parent at step 4 (`run.go:139` `os.MkdirAll` before the bootstrap lock) — which is *before* its step-5 probe; hoisting the probe "to the very top" of `step` runs it on a fresh worktree where the parent directory does not exist, and `lock.TryAcquireWriteLock` opens with `O_CREATE` and never creates a parent (the documented reason `Run` MkdirAlls at all). **Fix:** state whether `step` does the `MkdirAll` of `filepath.Dir(LoomRunLock)` before the early probe, or treats a missing-parent error as "lock free", and pin which — the probe is not a verbatim reuse of `run`'s.

### [NIT:consistency] `next_interrupt_policy` missing from the pinned envelope tuple
**Demoted-from:** BLOCKING
**Section:** § Decisions → `step-return-contract` **Issue:** the decision pins the CLI envelope as `{producer, outcome, output, next, state, reason, continue, history_length, status_file}`, then documents `next_interrupt_policy` as a tenth key in the bullet list below, while § Testing asserts "the full envelope key set is present" — the pinned tuple and the key list contradict each other, and the test has no unambiguous target. **Fix:** add `next_interrupt_policy` to the pinned tuple (or state explicitly that the tuple is `StepResult`-derived and the envelope is tuple + policy).

### [BLOCKING:design] Interrupt-policy meta-test pins against a test-only symbol
**Section:** § Scope In (loomshed bullet) and § Testing (loomcli bullet) **Issue:** `loomRowEngines` is an unexported `var` in `internal/loomrecipe/coverage_guard_test.go`, so it is unreachable from a meta-test living beside the table in `internal/loomshed` — the stated pinning mechanism cannot be built where the discussion places it. **Fix:** name the package the meta-test lives in (`internal/loomrecipe`'s own test package is the only one that can see both `loomRowEngines` and an exported `loomshed` table), or pin against `loomrecipe.New`'s assembled row list instead.

### [BLOCKING:design] Policy unavailable when the interrupted step is the first one
**Section:** § Decisions → `interrupted-step-re-invokes-on-loom-s-own-crash-resume-with-a-consecutive-cap` **Issue:** the branch reads "the envelope's `next_interrupt_policy`", but an interrupted invocation writes no parseable envelope, so the value must come from the *previous* step's envelope — which does not exist when the very first step of a supervised session is interrupted (fresh loop, or an operator re-invoking the skill after the 40-cap), and `lyx loom status`'s envelope carries no policy key (`status.go` emits `current_producer`/`state`/`error`/`pause_requested`/`activity`/`history_length`/`slug`/`parent`). **Fix:** decide the no-prior-envelope case — either surface the policy on `lyx loom status` too, or pin "hand back when no policy is in hand", since the skill may not branch on the producer name.

### [NIT:design] Interrupted-step "moved" comparison has no stated baseline
**Section:** same decision, first branch **Issue:** "`current_producer` moved, or `history_length` grew" never says what they are compared against. **Fix:** state that the baseline is the last successful step's envelope, and what the skill does when it has none.

## Verdict

REQUEST_CHANGES
Four blocking gaps: probe ordering, envelope key set, meta-test target, and first-step policy source.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 3._
MILL_REVIEW_END
