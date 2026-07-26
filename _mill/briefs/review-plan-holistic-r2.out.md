MILL_REVIEW_BEGIN
# Review: Treadle: shared round-loop engine + perch rewrite — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetmax
reviewed_file: plan/
date: 2026-07-26
```

## Findings

### [BLOCKING] Card 3 moves state_test.go/roundfiles_test.go whole; neither compiles
**Location:** batch 1 (treadle-extraction) / card 3
**Issue:** Card 3 `git mv`s `state_test.go` and `roundfiles_test.go` to `treadleengine` "surgical edits only... roundfiles_test unchanged otherwise." But `state_test.go`'s `TestProfileHash`/`TestDeriveRunID`/`TestValidRunID` call `ProfileHash`/`DeriveRunID`/`ValidRunID` — which card 2 explicitly extracts to `perchengine/identity.go`, never treadleengine — and `roundfiles_test.go`'s `TestBuildRoundProfile_FieldMapping` calls `buildRoundProfile` and builds a perch-shaped `Profile{Target,Fasit,Rubric,FixScope,ToolUse,ClusterFan,...}`, which card 2 extracts to `perchengine/adapter.go`, also never treadleengine. Moved verbatim, both files fail to compile in `package treadleengine`; this also breaks the differential-test-bar decision's "surgical edits only" promise.
**Fix:** At move time, split out only the sub-tests for functions that truly move (`loadOrInitState`/`saveState`/`moveStaleArtifacts`/`TerminalOutcome`/pause-flag tests; `roundToken`/`artifactPaths` tests); keep/create a `perchengine` test file for `TestProfileHash`/`TestDeriveRunID`/`TestValidRunID`/`TestBuildRoundProfile_FieldMapping`.

### [BLOCKING] Card 8 requires "EXACTLY TWO" files while also requiring the old "EXACTLY ONE" pin stay intact
**Location:** batch 2 (judge-handoff) / card 8
**Issue:** `template_test.go` (moved into treadleengine in batch 1) pins `requireContains(t, text, "EXACTLY ONE")` for both the circling and milestone templates. Card 8 requires those same templates to state rule (d), "write EXACTLY TWO files," while separately instructing "keep every existing pinned statement... intact" — self-contradictory, since satisfying (d) necessarily removes the "EXACTLY ONE" wording the pre-existing assertion still checks for.
**Fix:** Explicitly instruct card 8 to replace the stale `requireContains(text, "EXACTLY ONE")` assertions for the circling/milestone sub-tests with the new two-file pin (triage's template/test are untouched, since triage still writes one file).

### [NIT] Card 10's pre-round targeting walk doesn't compose with judgeReadSet's signature
**Location:** batch 3 (preround-targeting) / card 10
**Issue:** Card 10 needs "the newest-valid-handoff walk" before any attempt has run this round, but card 7's `judgeReadSet(rounds, currentReviewPath)` requires a current-round review path that does not exist yet pre-round. Card 10 never names the specific helper/refactor this reuses.
**Fix:** Name the extraction explicitly, e.g. a `latestValidHandoff(rounds) (path string, ok bool)` helper shared by `judgeReadSet` and the new pre-round call.

### [BLOCKING] Card 12's central mechanism claim is false: configengine.Load ignores unknown keys
**Location:** batch 4 (modelspec-migration) / card 12
**Issue:** Card 12 states deleting `judge_effort` from `template.yaml` "is what makes old perch.yaml files carrying judge_effort fail configengine.Load loud." But `configengine.Load` validates only via `yamlengine.MissingKeys(template, fileBytes)`, which reports template keys absent from the file — it never inspects keys the file has beyond the template — and `perchengine.LoadConfig`'s plain `yaml.Unmarshal` (no `KnownFields`) silently drops unknown keys too. An old `perch.yaml` with a leftover `judge_effort:` line loads successfully with that value silently discarded — contradicting the batch's "deliberate fail-loud breaking change" framing and making required test case (e) impossible as specified. Card 12 lists no `configengine`/`yamlengine` file in Context, which is likely why this went unverified (Context-completeness gap).
**Fix:** Add `internal/configengine/config.go` and `internal/yamlengine/reconcile.go` to card 12's Context, and switch `perchengine.LoadConfig` to a strict `yaml.Decoder.KnownFields(true)` unmarshal (mirroring `perchcli.decodeProfile`'s existing strictness) so an old file's extra key actually fails loud.

## Verdict

REQUEST_CHANGES
Two batch-1 test-file moves won't compile, a batch-2 template pin self-contradicts, and batch-4's fail-loud config premise is factually wrong.
MILL_REVIEW_END
