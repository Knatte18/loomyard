Yes, `strippedEnv` is declared and assigned on line 82, the `ok` variable is reused from the outer scope... wait, `ok` was declared earlier with `:=` on line 73 (`sessionID, ok := ...`), and again on line 77 (`socket, ok := ...`). By line 82, `ok` already exists in scope, so `strippedEnv, ok :=` would need `strippedEnv` to be new (which it is). `strippedEnv` is declared and never read again. In Go, this is a compile error: `strippedEnv declared and not used`.

Let me also verify the smoke test package declaration issue and plan requirement:

The plan (Card 13) states: "Define package `muxpoc_test` (external test package — avoids internal symbol conflicts)."

But the smoke test calls: `cmdUp`, `cmdDown`, `cmdStatus`, `cmdReview`, `LoadState`, `DeleteState`, `Config` — all lowercase-starting functions except `Config`, `LoadState`, `DeleteState`. Since `cmdUp`, `cmdDown`, `cmdStatus`, `cmdReview` are unexported, they cannot be called from `muxpoc_test` (external package). So the plan spec is self-contradictory. The implementation uses `package muxpoc` which correctly allows access to unexported symbols. This is not a bug in the implementation — it's a plan error that the implementation worked around correctly.

I'll flag the plan contradiction as a NIT.

MILL_REVIEW_BEGIN
# Review: Design mhgo mux module — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-12
```

## Findings

### [BLOCKING] Unused variable `strippedEnv` in smoke test — compile error under `-tags smoke`
**Location:** `internal/muxpoc/muxpoc_smoke_test.go:82`
**Issue:** `strippedEnv` is declared via `:=` on line 82 and never read again; Go rejects unused local variables as a compile error, so `go test -tags smoke` fails to build.
**Fix:** Either use the variable in an assertion (e.g. `_ = strippedEnv` or check its length as the plan requires) or assign to `_` on the left-hand side.

### [BLOCKING] Daemon exits on transient `LoadState` error instead of continuing
**Location:** `internal/muxpoc/daemon.go:58-60`
**Issue:** When `LoadState` returns an error the daemon calls `return output.Err(...)`, terminating the long-running process; the plan (Card 12, step 7b) explicitly requires printing to stderr and `continue`-ing the loop so a transient lock contention or I/O blip does not kill the watcher.
**Fix:** Replace `return output.Err(...)` on that path with `fmt.Fprintf(os.Stderr, ...); continue`.

### [BLOCKING] Unchecked `state.Panes[0]` index access in `cmdUp`
**Location:** `internal/muxpoc/up.go:46`
**Issue:** When `state != nil` and the session is up, the code reads `state.Panes[0].SessionID` without guarding against an empty `Panes` slice; a state file that was saved with zero panes (e.g. truncated write, or future code path) panics at runtime.
**Fix:** Add `if len(state.Panes) == 0 { return output.Err(out, "state has no panes") }` before accessing index 0.

### [NIT] Smoke test package is `muxpoc`, not `muxpoc_test` as the plan mandates
**Location:** `internal/muxpoc/muxpoc_smoke_test.go:3`
**Issue:** Card 13 requires `package muxpoc_test`, but the implementation uses `package muxpoc`; the implementation is actually correct because the test calls unexported symbols (`cmdUp`, `cmdDown`, etc.) which are inaccessible from an external test package — the plan spec is self-contradictory.
**Fix:** Acknowledge the plan contradiction; the `package muxpoc` declaration is the only workable choice and should be accepted as-is.

### [NIT] `socketArg` accepts an unused `cfg Config` parameter
**Location:** `internal/muxpoc/cmd.go:122`
**Issue:** The `cfg` parameter is never referenced inside the function body; it was included because the plan spec lists it, but it is dead weight in the signature and misleads callers into thinking socket derivation depends on config.
**Fix:** Remove the parameter and update all call sites (`p.run` and `p.output`) to call `socketArg()` with no argument.

### [NIT] Smoke test does not assert `len(stripped_env) > 0` when Claude-Code env vars are present
**Location:** `internal/muxpoc/muxpoc_smoke_test.go:82-85`
**Issue:** Card 13 step 5 requires asserting `len(stripped_env) > 0` when the test environment contains `CLAUDECODE` or `CLAUDE_CODE_*`; the test only checks the array type, skipping the substance of the env-hygiene contract.
**Fix:** After the type assertion, add a conditional check: `if os.Getenv("CLAUDECODE") != "" || ... { if len(strippedEnv) == 0 { t.Fatal(...) } }`.

### [NIT] `coldStart`/`coldRecover` retry loop does not fail explicitly if session never comes up
**Location:** `internal/muxpoc/up.go:102-114`, `up.go:181-193`
**Issue:** After three retries the loop exits silently and execution continues to `send-keys`; if psmux never became reachable, `send-keys` will fail and surface an error, but there is no explicit "timed out waiting for session" diagnostic to aid debugging.
**Fix:** After the retry loop, add a `hasSession` check and return `output.Err(out, "psmux session did not start in time")` when it is still down.

## Verdict

REQUEST_CHANGES
Three blocking issues must be fixed before the batch is shippable: unused variable compile error in smoke test, daemon exit on transient error, and unguarded slice index panic.
MILL_REVIEW_END
