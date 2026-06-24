Confirmed menu_test.go is integration-tagged. The card 15 test updates are correct but won't be exercised by the non-integration batch verify — a test-reachability NIT, not blocking.

I have all I need. Let me finalize the review.

MILL_REVIEW_BEGIN
# Review: Extract yamlengine and migrate config via lyx update — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-24
```

## Findings

### [BLOCKING] main_test.go board fixtures break under strict Load
**Location:** Batch 6, Card 21 (`cmd/lyx/main_test.go`)
**Issue:** `TestRunDispatchesToBoard` and `TestRunBoardErrorPropagatesExitCode` write `board.yaml` as `path: board\n` (only `path`); strict `config.Load` (Card 8) reports `home`/`sidebar`/`proposal_prefix` as missing and errors, so both tests fail — and batch 6's verify runs `./cmd/lyx/`. Card 21's requirements only cover the `init`/`update` dispatch tests, not these two fixtures.
**Fix:** Add a requirement to Card 21 to rewrite both fixtures to a template-complete `board.yaml` (all four keys) so strict Load succeeds.

### [NIT] Card 15 ide tests are behind //go:build integration; batch verify won't run them
**Location:** Batch 5, Card 15 (`internal/ide/menu_test.go`)
**Issue:** Card 15 requires updating `menu_test.go` to seed a valid `board.yaml` and assert the hard load-error, but that file is `//go:build integration` and batch 5's verify (`go test … ./internal/ide/`) has no `-tags integration`, so the updated assertions are never exercised (the package still compiles/passes vacuously). The existing integration tests use `Cwd: mainWorktreePath` with no `_lyx/config/board.yaml`, so they would break only under `-tags integration`.
**Fix:** Note in Card 15 that the menu_test changes require `-tags integration` to verify, or state the fix is validated by review since the batch verify cannot reach it.

## Verdict

REQUEST_CHANGES
One blocking test-fixture gap in Card 21 will break batch 6's `./cmd/lyx/` verify.
MILL_REVIEW_END