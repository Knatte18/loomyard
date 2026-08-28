MILL_REVIEW_BEGIN
# Review: reed: resume/down leak lock directories at the stale pre-rename session-name path

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:consistency] "status refuses before a lock helper" is false
**Section:** §Decisions → root-cause…, and Q&A #1 **Issue:** `Status()` acquires `withOpLock` (`lifecycle.go:1165`) and the renamed-worktree refusal fires *inside* it via `requireSessionLocked`→`refuseLiveForeignSessionLocked`, i.e. after the `MkdirAll` — so "read-only `status` produces no stray (it refuses before reaching a lock helper)" is source-wrong and contradicts §refuse-at-the-op-lock-chokepoint, which lists `status` among the verbs that pass through the chokepoint. **Fix:** Restate that evidence item on its true ground (a CLI `status` resolves a fresh geometry via `PersistentPreRunE`, so it cannot leak — not that it stops short of the lock) and drop the "before a lock helper" claim.

### [BLOCKING:consistency] Verb-correlated timestamps vs. the 2s poll model
**Section:** §Problem + Q&A #1 vs §the-leak-is-continuous-not-per-event **Issue:** §Problem asserts the stray's birth timestamp "matches the exact moment the verb ran" and Q&A #1 cites "birth timestamps matching geometry-changing verbs only" as root-cause evidence, while §the-leak-is-continuous asserts an ungated 2s `reapplyLayout` that must recreate the stray independently of any verb — the two models cannot both be true, and the M25 "re-check after the poll cycle" assertion depends on which one is. **Fix:** Pick one model, say so explicitly, and rewrite the evidence list so the surviving items actually support it.

### [NIT:consistency] Validator name/rationale still says AnchorPath
**Section:** §a-separate-live-validator-keeps-the-pure-one-pure **Issue:** The proposed name `validateAnchorPathLive` and its anchor-worded rationale are leftovers from the pre-round-2 predicate; the function validates `WorktreeRoot`, not `AnchorPath`. **Fix:** Rename in the discussion (e.g. `validateToldWorktreeRootLive`) and reword the rationale to name the field it actually checks.

### [NIT:consistency] "nothing MkdirAlls stateDir in standalone" is overstated
**Section:** §Technical context, `standalonestate.Derive` bullet **Issue:** With no `--stencils-dir`, both wirings call `stencilstore.Reconcile(standalonegeom.StencilsDir(stateDir))` = `<stateDir>/_lyx/stencils` (`stencilsdir.go:26`, `burlercli/wiring.go:139`, `webstercli/wiring.go:159`), which does create `stateDir` before the engine runs — so `withOpLock`'s `MkdirAll` is only the sole materializer on the `--stencils-dir` path. **Fix:** Narrow the claim; the `WorktreeRoot` decision is unaffected but the stated basis should match source.

### [NIT:scope] Fixture sweep does not mention build-tagged files
**Section:** §Testing → Fixture **Issue:** `internal/reedengine` has tag-gated in-package tests that build inline `Geometry` literals and drive real ops (`contract_integration_test.go:648,757`, `mouse_boot_integration_test.go:48`, `attachgeometry_integration_test.go`), and "run the whole package afterwards" without tags never compiles or runs them. **Fix:** State that the sweep and the verification run cover the `integration`/`smoke`-tagged files too.

## Verdict

REQUEST_CHANGES
Two root-cause evidence claims contradict the source and each other; decisions themselves are sound.
MILL_REVIEW_END
