MILL_REVIEW_BEGIN
# Review: Diagnostic tracing (trace) on the logger module — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewer_self_id: Claude Sonnet 4.5 (Sonnet 5 generation)
reviewed_file: plan/
date: 2026-07-31
```

## Findings

### [BLOCKING] ensureDurableSink's testing-gate order defeats the SetDurableSinkDir seam
**Location:** Batch 4 Card 10, contradicted by Batch 4 Cards 12/15/16 and Batch 5 Card 21/22 and Batch 6 Card 26
**Issue:** Card 10 specifies `ensureDurableSink`'s `sinkOnce.Do` body checks `testing.Testing() && LYX_TRACE != "1"` **first**, unconditionally returning `sinkOK=false` before ever consulting `SetDurableSinkDir`'s override directory. Since every unit test runs with `testing.Testing()==true` and none of Cards 15/16/21/26 ever set `LYX_TRACE=1`, this ordering means `SetDurableSinkDir(t.TempDir())` can never actually arm the sink under `go test` — yet Card 15 ("calling `ensureDurableSink()` directly creates exactly one file"), Card 16 (`NotifyExit(1)` "creates exactly one file"), Card 21 cases 2/3 (Info/Warn "reaches the durable sink"), Card 22, and Card 26 all require the sink to open via `SetDurableSinkDir` alone with no `LYX_TRACE` set. Card 21's own case 4 ("durable sink unarmed... testing.Testing() true and LYX_TRACE unset") describes exactly this precondition as the *unarmed* case, directly contradicting cases 2/3 in the same card under identical preconditions.
**Fix:** Reorder Card 10's gate so a non-empty `SetDurableSinkDir` override is checked (and, if set, taken) before or instead of the `testing.Testing()`/`LYX_TRACE` short-circuit — an explicit test-directory override is itself the opt-in signal, distinct from the "never called `SetDurableSinkDir`, no `LYX_TRACE`" case the gate exists to keep silent.

### [BLOCKING] Card 44's spawn-observability citation points at the wrong line, risking a nil-pointer crash
**Location:** Batch 13 Card 44 (`internal/scoutengine/ensureserver.go`)
**Issue:** Card 44 says to add `logger.Info(...)` "immediately after the supervised daemon's `cmd.Start()` succeeds (ensureserver.go:520, `cmd := exec.Command(argv[0], argv[1:]...)`...)". Line 520 is only the `exec.Command` construction; the actual `cmd.Start()` call and its error check are at lines 545-551 (confirmed by reading the file), with `logFile.Close()` at 555. `cmd.Process` is nil until `Start()` succeeds, so a log call reading `cmd.Process.Pid` inserted per the literal line-520 citation (before `Start()` even runs) panics on a nil-pointer dereference.
**Fix:** Correct the citation to point at the success path after line 545's error check (e.g. immediately after line 555's `logFile.Close()`), where `cmd.Process.Pid` is safe to read.

### [NIT] Card 35's cited line for the header-pane split error is the validate-guard, not the raw split call
**Location:** Batch 9 Card 35 (`internal/reedengine/lifecycle.go`)
**Issue:** Card 35 cites "lifecycle.go:576-578" as a "header-pane split... error" to instrument, but that range is actually `validateSplitCreatedNewPane`'s failure branch; the real `tmux.output("split-window", ...)` error is at lines 565-568 and is not named by the card at all.
**Fix:** Either cite 565-568 explicitly alongside 576-578, or rely on the card's own "re-audit the rest of the file" instruction to catch it (currently the only safety net for this specific site).

## Verdict

REQUEST_CHANGES
Card 10's testing-gate ordering breaks the SetDurableSinkDir test seam every later test card depends on; Card 44's line citation risks a nil-pointer crash.
MILL_REVIEW_END
