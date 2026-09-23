MILL_REVIEW_BEGIN
# Review: Shuttle guarantees a started run is past its startup gates

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewed_file: _mill/discussion.md
date: 2026-09-23
```

## Findings

### [BLOCKING:design] Webster startup-mechanism-failure strand leak unstated
**Section:** Decisions "Startup mechanism failure", "Webster's persist-before-block windows" **Issue:** a mechanism-failure start leaves the strand live with no teardown, and at both webster sites (`StartMaster`, `recoverSpawn` at `websterengine/recoverbatch.go:183`) the error returns before any guid is persisted, so the Master strand is invisible to entry-time reclaim and the recovery strand to `removeStrandIfLive`; today the guid is persisted before this failure can surface from `Wait`. Only loom's version of this residual is named, and "A not-ready start no longer leaks at either site" reads as if webster has none. **Fix:** state the webster mechanism-failure residual (unreclaimable live strand; next spawn either duplicates it or collides by name) as accepted or handled, and name where it is documented.

### [BLOCKING:decision] `Run.attached` loses every reader; attach.go comments go stale
**Section:** Scope/Out "`Runner.Attach`/`AttachGated` control flow: unchanged", Technical context `attach.go:193–221` **Issue:** after the `Wait` seed drops `run.attached &&` and `AwaitStarted` is deleted, `attached` (`run.go:219`) has no production reader; `reconstructAndWait`'s comments (`attach.go:193`, `216–220`: "Wait only ever skips the startup probe when BOTH are true") become false, contradicting "untouched apart from the doc comment ... in `run.go`". **Fix:** state whether the `attached` field is removed or kept, and add the `attach.go` comment rewrite to scope.

### [NIT:design] Tick-cap exit outcome unspecified
**Section:** Decisions "Not-ready start", "Run deadline anchoring"; Testing "tick cap terminates" **Issue:** `AwaitStarted`'s tick-cap exit (`wait.go:428`) is neither "pane not live" nor "window expired", and the discussion gives it no persisted Outcome (`died`/`timeout`), no teardown statement, and no expected test result. **Fix:** name the tick-cap exit's outcome and teardown.

### [NIT:design] Terminal-Outcome rationale overstates "no agent behind it"
**Section:** Decision "Attach treats a torn-down not-ready record as respawn-eligible" **Issue:** "no agent can still be working behind it" is false for `timeout`/`asking` from `finalize`, which leave the strand and pane alive (`wait.go:821`); the decision holds only by parity with the tracked-live branch. **Fix:** ground the rationale on the tracked-live parity and acknowledge the untracked-`timeout` case.

## Verdict

REQUEST_CHANGES
Webster's mechanism-failure leak and the orphaned `attached` field need a stated disposition.
MILL_REVIEW_END
