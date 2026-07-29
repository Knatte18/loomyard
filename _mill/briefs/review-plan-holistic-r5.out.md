MILL_REVIEW_BEGIN
# Review: Give codeintel a persistent, session-long daemon — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (Anthropic), self-assessed
reviewed_file: plan/
date: 2026-07-29
```

## Findings

### [BLOCKING] ensureServer's native-fallback branch has zero test coverage
**Location:** batch 4 / card 12 (and batch 5 / card 13-14)
**Issue:** After the flip, `ensureServer` falls back to `ensureNative` on *any* `ensureSupervised` error (requirement 3). No card, in either batch 4's untagged suite or batch 5's integration suite, exercises this fallback: card 13's integration test only proves the success path (`connKindSupervised` + reuse); `TestReferences_HasNativeDaemonRoutesThroughEnsureServer` (updated by card 12) only proves the pre-dispatch toolchain-failure path, which structurally never reaches `ensureSupervised`/`ensureNative` at all. The "supervised fails for a non-toolchain reason → native is attempted → its result (success or failure) is what `ensureServer` returns" path — the actual safety net the commit message names ("... with native fallback") — is asserted nowhere.
**Fix:** Add an untagged, process-free unit test (e.g. in `ensureserver_test.go` or `refs_test.go`) that pre-holds the spawn lock (the existing `lock.AcquireWriteLock` technique from `supervised_test.go`) so `ensureSupervised` deterministically returns `ErrServerSpawnTimeout` within a short deadline, combined with `withFakeInstaller`/`withTempUserCacheDir` (already used by `TestReferences_HasNativeDaemonRoutesThroughEnsureServer`) so the native fallback's own `resolveGoToolchain` also fails predictably — then assert `ensureServer` returns `connKindNative` and an error wrapping the fake installer's sentinel, proving both that the fallback fires and that its result (not supervised's timeout) is what's returned. No real gopls or daemon spawn needed.

### [NIT] buildOptions unit test doesn't guard the actual call-site wiring
**Location:** batch 3 / card 8
**Issue:** The described `cli_test.go` assertion calls `buildOptions` directly with hand-supplied params for both the "single-arg" and "batch" shapes and compares the results — since it's the same function called twice, this is close to tautological and would not catch a future call-site bug (e.g. a batch closure passing the wrong local instead of `worktreeRoot`) the way the card's own rationale ("guarding against the batch-mode WorktreeRoot regression") implies.
**Fix:** Note in the card that the DRY refactor itself (six literals collapsed to six one-line calls) is the primary regression guard, not the table test; optionally have the test assert each of the six actual call sites passes a non-empty `worktreeRoot` via a lightweight code-level check, or accept the current test as documentation-only and drop the "guarding against ... regression" framing.

## Verdict

REQUEST_CHANGES
Card 12's supervised→native fallback path is asserted by no test anywhere in the plan; add untagged coverage before landing.
MILL_REVIEW_END
