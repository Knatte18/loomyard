MILL_REVIEW_BEGIN
# Review: Shed recipe: engine registry

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-21
```

## Findings

### [BLOCKING:design] report_name is free-form but only one value works
**Section:** "`Bouncer`'s `ReportName` closure is derived from a `Config` pattern string" **Issue:** `ResolveRound` (`internal/shedadapters/round.go:24-46`) resolves the round by statting `reportName(n)` under `RunDir`, and a `BurlerRound` writes a hardcoded `round-<n>-review.md` (`burler.go:106-108`) — so for any `Bouncer`+`BurlerRound` perch the only correct `report_name` is `round-%d-review.md`, contradicting the decision's rationale that "different segments legitimately name their reports differently" and leaving a mismatch to fail silently (round always resolves 0, the Bouncer re-seeds until its bounce budget is spent). **Fix:** decide the disposition — either the `Bouncer` entry defaults/pins the pattern to the Burler round-artifact name, or the pair is validated as a unit — and add the mismatch case to testing item 7 alongside the shared-`run_subdir` case.

### [BLOCKING:design] No decision on validating the told `Env` fields
**Section:** "Live seams travel in a told `Env`" / "Relative `Config` paths resolve against a named `Env` root" **Issue:** entries are decided to reject bad `Config` values but nothing says whether an entry validates the `Env` root or seam it consumes; `NewSingleLLMProducer` (`singlellm.go:49-54`) validates nothing, so an empty or relative `Env.WorktreeRoot` yields a producer that fails at every `Call` rather than at construction — the exact late-failure mode the relative-path decision exists to prevent for `Config`. **Fix:** state whether each entry checks the `Env` fields it reads (non-empty, absolute, non-nil seam) at construction, and add a testing scenario for an under-filled `Env`.

### [NIT:decision] Several `shuttleengine.Spec` fields have no stated disposition
**Section:** "The `SingleLLM` entry builds its `SpecSource` from a stencil" **Issue:** the recognised key set is closed and strict, but `Spec.Timeout`, `ForkSubagents`, `KeepPane`, `Round`, `Parent`, and `Display` (`internal/shuttleengine/spec.go:21-95`) are never mentioned as kept, deferred, or deliberately excluded. **Fix:** add one line recording them as deliberately not recipe-authorable in this task (zero values defer to shuttle config), so piece 2 does not re-litigate.

### [NIT:scope] `profile.target`/`profile.fasit` sub-keys left implicit
**Section:** "`BurlerRound`'s `Config` mirrors `burlercli`'s profile key names" **Issue:** the six top-level keys are enumerated, but `target`/`fasit` are nested maps whose own keys (`paths`, `instructions`, per `fileSetYAML`, `internal/burlercli/run.go:23-26`) are not named, and the strict unknown-key rule applies at that level too. **Fix:** name the two nested keys explicitly so the rejector's recognised set is fully enumerated.

## Verdict

REQUEST_CHANGES
Two silent-failure gaps: unconstrained `report_name` and unvalidated `Env` inputs.
MILL_REVIEW_END
