MILL_REVIEW_BEGIN
# Review: batcher: split out of webster into a standalone configreg module with its own batcher.yaml

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-x class model (self-reported ID: Opus 5)
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [BLOCKING:design] Nil RunDeps.Batcher has no stated contract
**Section:** Decisions → runlevel-call-site
**Issue:** Today `RunDeps.Config.Batcher`'s zero value (`""`) safely resolves to `identity` inside `Run` (`runlevel.go:327`); after the change the zero value is a nil `batcher.Batcher` interface and `deps.Batcher.Batch(plan.Cards)` panics, and `Run` has no deps-validation pass (only `deps.Bisector` is nil-checked, at `runlevel.go:820`).
**Fix:** State whether `Run` guards `deps.Batcher == nil` with a typed error (like the zero-batch refusal beside it) or whether population is a documented caller obligation, and pin the choice with a test.

### [NIT:scope] runlevel_test.go fixture edit is unlisted and contradicted
**Demoted-from:** BLOCKING
**Section:** Scope → In; Testing → "Scenarios that must be covered somewhere"
**Issue:** `newRunFixture` (`internal/websterengine/runlevel_test.go:250–290`) sets no `Batcher` field, and its header comment at `:7–8` says "Config.Batcher is left empty in every fixture, resolving to internal/batcher's own DefaultName" — so the chosen design also requires a one-line fixture edit plus a comment rewrite, exactly the cost the rejected alternative was charged; the discussion instead presents the 15 `TestRun_*` tests as "implicitly proven" and never lists that file.
**Fix:** Add `internal/websterengine/runlevel_test.go` (fixture field + header comment) to Scope → In and restate the proof as "pass after a one-line injection", not "pass as-is".

### [NIT:consistency] Sandbox suite doc falsified while declared unaffected
**Demoted-from:** BLOCKING
**Section:** Decisions → reconcile-required-for-pre-registry-wefts
**Issue:** `tools/sandbox/SANDBOX-WEBSTER-SUITE.md:28` enumerates what a webster run requires ("materializes `_lyx/config/webster.yaml`, plus `shuttle.yaml`/`reed.yaml`"); after this task `batcher.yaml` is required too, so the sentence is falsified even though the suite still runs green — yet the discussion states the suites "need no change".
**Fix:** Either add that line as an eighth doc site under the same ownership rule the other seven use, or state explicitly that operationally-correct-but-incomplete suite prose is deliberately left standing.

### [NIT:decision] "should do the same" leaves the not-initialized wrap undecided
**Section:** Technical context → "The config-module pattern to copy"
**Issue:** "batcher should do the same for consistency" is non-committal, and it concerns a second, distinct error path — `websterengine.LoadConfig` rewrites the missing-`_lyx` case to `run "lyx fabric reconcile"`, whereas the absent-file decision quotes `configengine.Load`'s `run "lyx config reconcile"`.
**Fix:** Make it a decision naming both paths and their exact texts, so the plan writer does not conflate them.

### [NIT:scope] config_test.go carries batcher sites beyond :125
**Section:** Scope → In; Technical context → "Test sites that break by construction"
**Issue:** `internal/websterengine/config_test.go` also has `Batcher: ""` in a `Config` literal at `:61` and `batcher: identity` in the YAML fixtures at `:106` and `:139`; naming only `:125` reads as a complete per-file inventory.
**Fix:** Note that the whole file is in scope (one field removal plus two stale fixture keys), not just the one assertion.

## Verdict

REQUEST_CHANGES
Injection contract, fixture scope, and one falsified sandbox doc line need resolving first.
MILL_REVIEW_END
