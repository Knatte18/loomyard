MILL_REVIEW_BEGIN
# Review: reed: born-as-strand for loom start's operator attach — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Sonnet 5)
reviewed_file: plan/
date: 2026-09-19
```

## Findings

### [BLOCKING:scope] Card 7 Context omits Status()'s defining file
**Location:** Batch 1, Card 7 **Issue:** Requirements direct the test to "assert AddStrand returns no error and that Status() reports that strand Live," but `Status()`/`StatusResult` is declared in `internal/reedengine/lifecycle.go` (verified: `func (e *Engine) Status() (StatusResult, error)`), which is absent from Card 7's Context (`spawn.go`, `strand.go`, `contract_integration_test.go`) and not referenced by any of those three files. **Fix:** Add `internal/reedengine/lifecycle.go` to Card 7's Context.

### [BLOCKING:scope] Card 11 cites awaitRunLock from an un-Contexted file
**Location:** Batch 2, Card 11 **Issue:** Requirements direct the doc comment to "Name awaitRunLock's four injected seams as the existing precedent for this shape," but `awaitRunLock` is defined in `internal/loomcli/bootstrap.go` (verified: `func awaitRunLock(lockHeld ..., alive ..., halted ..., wait ..., attempts int)`), which appears in neither Card 11's Context (`spawnwatchdog.go`, `reedcli/cli.go`) nor its Edits (`loomcli/cli.go`, `start.go`). **Fix:** Add `internal/loomcli/bootstrap.go` to Card 11's Context.

### [BLOCKING:scope] Card 14 names three fixtures only smoke_test.go defines
**Location:** Batch 2, Card 14 **Issue:** Requirements direct reuse of "the real built binary helper, the wired-pair fixture, the bootstrap teardown registration, the reed engine probe, the strand-count helper, and the tmux-binary skip." Verified against source: `probeReedEngine`, `statusStrandCount`, and `tmuxBinaryPath` are declared only in `internal/loomcli/smoke_test.go`, and none of the three appears anywhere in `smoke_bootstrapwiring_test.go` (the only sibling-fixture file Card 14's Context lists, alongside `bootstrap.go` and `strand.go`). Half the named fixtures are unreachable from Context. **Fix:** Add `internal/loomcli/smoke_test.go` to Card 14's Context.

### [BLOCKING:design] Card 13's cited test precedent doesn't reach a receiver
**Location:** Batch 2, Card 13 **Issue:** Requirements say to assert `Command()`'s and `StartAliasCommand()`'s constructed `loomCLI` each carry a non-nil `spawnWatchdog`/correct `suppressWatchdogSpawn`, "reaching the receiver the way ... TestStartAliasCommand_StaysOneCommandWithSubtreeVerb already reaches what it inspects." Verified in `internal/loomcli/cli_test.go`: that test never reaches the alias's own receiver — it builds an unrelated `(&loomCLI{}).startCmd()` and compares `Use`/flag presence against the alias's `*cobra.Command`. Neither `Command()` nor `StartAliasCommand()` (`cli.go`/`start.go`) exposes its constructed `*loomCLI`, so there is no way to inspect the actual instance's fields without an accessor — which the same card forbids ("rather than inventing a new accessor"). A replica `&loomCLI{}` built independently in the test would not catch the nil-field bug the card exists to guard against. **Fix:** Either permit a minimal test-only accessor/seam on the constructed receiver, or restate the card's test as an indirect behavioral assertion (e.g. drive the alias's `RunE`/`spawnWatchdog` call site) rather than a direct field read.

### [NIT:consistency] Card 15 references a "process-kill helper" that doesn't exist as such
**Location:** Batch 2, Card 15 **Issue:** Requirements say to reuse "findDriverPIDs' own structure and its process-kill helper," but `registerBootstrapTeardown` (smoke_test.go) kills pids via a direct `proc.KillPID(pid)` call in a loop, not a named local helper. **Fix:** Reword to "the same find-then-`proc.KillPID` pattern" so the implementer isn't led to look for a helper function that isn't there.

## Verdict

REQUEST_CHANGES
Four Context-completeness/false-premise gaps across cards 7, 11, 13, 14 need fixing before implementation.
MILL_REVIEW_END
