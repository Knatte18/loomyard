MILL_REVIEW_BEGIN
# Review: loom: Plan-Write/Plan-Validate approval deadlock (F7) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (per system context; a.k.a. "Sonnet 5")
reviewed_file: plan/
date: 2026-08-26
```

## Findings

### [BLOCKING:scope] shape_test.go's testEnv() never gets ApprovePlan, breaks batch 6's own verify
**Location:** Batch 6 (Card 21) / `internal/loomrecipe/shape_test.go`'s `testEnv()`.
**Issue:** Card 21 adds `approve_seam: plan` to the shipped recipe's `Plan-Bouncer` row, which makes `bouncerEntry` call `requireSeam("Bouncer", "ApprovePlan", env.ApprovePlan)` for that row. `shape_test.go`'s `testEnv()` (read directly, lines 88-127) builds an `Env` with `CommitPlan` but no `ApprovePlan` field, and is the env every `TestNew_*` test in that file (`TestNew_ProducerTable`, `TestNew_ToldShedFields`, `TestNew_PublishAndFinalizeAreRealProducers`, `TestNew_ProducerTableOrderUnchangedByWiring`, `TestNew_PassesShedValidation`, `TestNew_RoutingGraphIsClean`) feeds into `New(env, paths)` expecting `err == nil`. After Card 21 lands, `New()` fails at the `Plan-Bouncer` row (row 9 of 17, well before `Publish`), so every one of those tests breaks, and `TestNew_MissingLandingClosureReturnsError`'s `strings.Contains(err.Error(), "Publish")` assertion fails too since the surfaced error now names `ApprovePlan` instead. Batch 6's own scope note lists `shape_test.go` as "regression surface" for card 21 but no card touches it, and this is a real construction failure, not a passive regression risk — batch 6's `verify: go test ./internal/loomrecipe/...` would not pass as written.
**Fix:** Add a card (or extend Card 13/22) to fill `testEnv()`'s `Env` literal with `ApprovePlan: func() error { return nil }`, landing in the same batch (6) that turns `approve_seam: plan` on, not batch 3.

### [BLOCKING:scope] Card 6's six named helpers live in files absent from its Context
**Location:** Batch 2, Card 6 (`internal/shedadapters/bouncer_commit_test.go`).
**Issue:** Card 6's Requirements name six specific helpers to reuse — `testBouncerConfig`, `judgeFakeShuttle`, `layoutBouncerRun`, `bouncerVerdictContent`, `bouncerLedgerContent`, `bouncerReport`. Verified by grep: `testBouncerConfig` is declared in `internal/shedadapters/bouncer_seed_test.go`; the other five are declared in `internal/shedadapters/bouncer_judge_test.go`. Neither file is listed in Card 6's `Context:` (which is `internal/shedadapters/bouncer.go` only) or `Edits:`. Per the plan's own Context-completeness bar, naming a function from a file outside `Context:`/`Edits:` forces cold-start exploration.
**Fix:** Add `internal/shedadapters/bouncer_seed_test.go` and `internal/shedadapters/bouncer_judge_test.go` to Card 6's `Context:` list.

## Verdict

REQUEST_CHANGES
Batch 6 breaks its own verify via an unfixed `shape_test.go` fixture, plus a Context-completeness gap in Card 6.
MILL_REVIEW_END
