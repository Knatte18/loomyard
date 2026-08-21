MILL_REVIEW_BEGIN
# Review: loom: convert to a Shed recipe

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-21
```

## Findings

### [BLOCKING:decision] `loomshed_test.go` has no "non-New half"
**Section:** Scope ("`loomshed_test.go`'s New-driving half") / `test-ownership`
**Issue:** Every test in `internal/loomshed/loomshed_test.go` drives `New` — `TestNew_ProducerTable`, `ToldShedFields`, `PublishAndFinalizeAreRealProducers`, `ProducerTableOrderUnchangedByWiring`, `MissingLandingClosureReturnsError`, `NilPreflightReturnsError`, `PassesShedValidation`, `RoutingGraphIsClean` — yet the discussion assigns a disposition only to the `shedcheck` one, and the "stays in `loomshed`" list names other files exclusively.
**Fix:** State a per-test disposition for all eight, including that `TestNew_NilPreflightReturnsError` is deleted outright (its guard is removed by `preflight-row`) and where `ToldShedFields` / `PassesShedValidation` land against the new Shed-paths carrier.

### [BLOCKING:scope] "no change to the four packages" blocks required comment repair
**Section:** Scope → Out (first bullet)
**Issue:** Deleting `loomshed.New`/`Deps` and moving the guard falsifies production doc comments inside packages the discussion declares out of scope: `internal/shedcheck/doc.go:8` ("Neither `shedengine.Run` nor `loomshed.New` calls `Check`"), `internal/shedrecipe/recipe.go:69` ("told wholesale by `loomshed.Deps.Landing` today"), and `internal/shedrecipe/entries_simple.go:33,53-54` (both pointing at `coverage_guard_test.go` as the pin for the `Publish`/`Finalize` row keys).
**Fix:** Carve out doc-comment corrections to stale references explicitly, naming those four sites, so the Out clause bans behaviour changes rather than the comment repairs this task creates.

### [NIT:consistency] Docs decision says five files, lists six
**Section:** `docs` Decision / Scope / Q&A log
**Issue:** The heading says "five files" and the rationale "four of these five", but the body and Scope list six (`manifest/parallel-work.md` included, correctly — its line 8 is falsified); the Q&A entry omits it.
**Fix:** Make heading, rationale count, and Q&A entry all say six and name `parallel-work.md`.

### [NIT:consistency] Seam-allowlist shrink over-stated
**Section:** `delete-loomshed-new` Note
**Issue:** It says `landingshed`, `websterengine`, and `shedadapters` "were pulled in largely by `Deps`"; only `landingshed` becomes droppable — `webster.go:9-13` still imports `shedadapters` and `websterengine` for `NewWebsterProducer`.
**Fix:** Name `landingshed` as the one removable entry and say the other two stay.

### [NIT:consistency] "Genuine weakening" mischaracterises today's guard
**Section:** `test-ownership` (orphan-entry allowance)
**Issue:** Today's `TestCoverageGuard_EveryLoomRowHasAnEngine` asserts table↔row agreement plus `Lookup` resolution; it makes no claim about unused registry entries, so the `SingleLLM`/`Bouncer`/`BurlerRound` allowance is a carve-out in a newly added assertion, not a weakening of an existing one.
**Fix:** Reword as "a new orphan-check half carrying a named three-engine allowance", keeping the requirement that the allowance is written down.

### [NIT:consistency] "Refusal test passes unchanged" is false
**Section:** Testing → `internal/loomcli`
**Issue:** `drive`'s no-status-file refusal is covered by `TestVerbRefusals` (`internal/loomcli/cli_test.go:106-148`), which hand-builds `loomshed.Deps{…}` at line 128 and cannot compile once `Deps` is deleted.
**Fix:** Say the test's assertions stay unchanged while its `loomCLI` fixture is repointed at the new Shed-paths carrier.

## Verdict

REQUEST_CHANGES
Two blocking gaps: undeclared disposition for seven `loomshed_test.go` tests, and an over-broad Out clause.
MILL_REVIEW_END
