{"status":"success","commit_sha":"f22c81237cae6c64e8193d374f1dba68ba1ddd6b","session_id":"f90d0f09-999a-4658-8114-6f9976e3054c","cards_done":[39,40,41,42]}

Summary of work completed for batch `webstercli-rewire` (all 4 of 4 cards committed and verified):

**Card 39** (`26c42068`) - `internal/webstercli/cli.go`: retargeted the `starter` seam from `builderengine.Starter` to `websterengine.Starter`; wired `batcher.Select(websterCfg.Batcher)` into `PersistentPreRunE` as a fail-fast gate, storing the resolved `batcher.Batcher` on a new `websterCLI.batcher` field; stripped stale plan-format-v2 prose from the parent command's `Long`.

**Card 40** (`978f33d4`) - `internal/webstercli/{awaitbatch,beginbatch,recordbatch,recoverbatch,validate}.go`: retargeted every `builderengine.ParsePlan` call to `planparser.ParsePlan`, deriving `batches := c.batcher.Batch(plan.Cards)` and threading it into `BeginDeps`/`RecordDeps`/`RecoverDeps.Batches` and `AwaitBatch`'s new `batches` parameter; retargeted `validate.go` to `planparser.Validate` (dropped `ValidateCaps`) and pinned the new JSON contract (`cards`/`card` keys replacing `batches`/`batch`); removed `--restart-chain` and updated `BeginBatch`'s call signature; rewrote `Long` help text for the flat card-list model.

**Card 41** (`3ebe0522`) - `internal/webstercli/{pause,status,weft}.go`: retargeted pause-flag mechanics from `builderengine` to `websterengine`'s webster-local copy.

**Card 42** (`f22c8123`) - `internal/webstercli/{cli_test,verbs_test,smoke_test}.go`: retargeted all fixtures/fakes to `planparser`/`websterengine`/`batcher`; rewrote plan fixtures for plan-format v3; added `testPlanFingerprint` (local duplicate of websterengine's unexported algorithm); added `TestPersistentPreRunE_UnknownBatcherFailsFast`/`_DefaultBatcherResolves` (via `lyxtest.CopyHostHub`/`SeedConfig`, driving the real `PersistentPreRunE`); added `TestCommand_LongStringsHaveNoStaleV2Language`.

Verify command `go test -tags integration ./internal/webstercli/...` passes (fresh, uncached run confirmed). `go vet -tags "integration smoke"` and `go test ./internal/webstercli/...` (untagged) also pass. Working tree is clean (no uncommitted tracked changes). The only untracked file is the implementer brief itself (`_mill/briefs/implement-webstercli-rewire-r1.md`), outside this batch's scope.
