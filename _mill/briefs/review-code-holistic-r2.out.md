Good - no `muxpoc.` prefix needed, consistent with being in the same package. This is correct.

Let me verify the `SaveState` lock pattern more carefully. In `state.go`, `SaveState` calls `board.AtomicWrite(cwd, stateRelPath, ...)` but `board.AtomicWrite` itself does NOT acquire any lock — the lock is already held by `SaveState`. However, `store.Save` in `board` acquires the swaplock _on the swap file_ (not the advisory write lock) before calling `AtomicWrite`. The muxpoc `SaveState` takes the write lock on `.mhgo/muxpoc-state.lock` and then calls `AtomicWrite` directly — there's no second swaplock as in board's pattern. This is OK since there's only one lock in the muxpoc system for both readers and writers (the `muxpoc-state.lock` file serves both read and write locking). The plan describes this as the intended approach.

Let me look at one more potential issue — the smoke test at line 3 uses `package muxpoc` (not `muxpoc_test`). The plan explicitly says `muxpoc_test`. But since the test needs to call `cmdUp`, `cmdDown`, etc. (unexported), the only way this works is being in the same package. The plan has a contradiction.

Let me also check for any issues with `wt.exe` fallback logic in spawn_windows.go:

Looking at `/internal/muxpoc/spawn_windows.go` lines 36-47: the fallback checks `cmd.Start()` error from `wt.exe`. If it fails (e.g. wt.exe not found), it falls back to plain psmux. But the plan says "If `wt.exe` is not found, fall back". The actual error from `cmd.Start()` when `wt.exe` isn't found would be an "executable file not found" error, which is the right trigger. This matches the plan intent.

One concern: the wt.exe command is fired-and-forgotten with `cmd.Start()`. If wt.exe IS found but fails to launch the psmux attach within it, we'd get no error feedback. But that's the plan's intent ("fire-and-forget").

Let me check the `down` subcommand — it uses `mux.run("kill-server")` but the socket is derived from `socketArg()` which calls `os.Getwd()`. Since `down` already loaded `state.Socket`, shouldn't it use `state.Socket` explicitly? Let me look at this:

The `PsmuxCmd.run` calls `socketArg()` which derives the socket from the current working directory. The state has `state.Socket` which was set at `coldStart` time as `socketName(cwd)`. Since both are derived from `os.Getwd()`, they should match. But this is an implicit dependency — if somehow the state file was created with a different socket (e.g., moved from another repo), the kill-server would target the wrong socket. This is a theoretical concern but functionally consistent with the cwd-authoritative model.

Let me also check whether the `down` command passes the socket to kill-server correctly:

In `down.go` line 34: `_ = mux.run("kill-server")` — this runs `psmux -L <socket-from-cwd> kill-server`. But `state.Socket` might differ if the state was created at a different CWD (edge case). However, the plan explicitly says "All subcommands derive state file path, socket name, and session name from `os.Getwd()`" — so this is correct by design.

Now let me summarize all findings:

1. **Smoke test package declaration deviates from plan** — plan says `package muxpoc_test` (Card 13), implementation uses `package muxpoc`. However, the plan itself has a self-contradiction: `muxpoc_test` can't call unexported functions like `cmdUp`. The implementation's choice is correct for it to work. NIT-level at most.

2. **`socketArg` signature deviates from plan** — plan says `func socketArg(cfg Config) string`, implementation is `func socketArg() string`. Since `cfg` was never used in the plan's description, this is a harmless improvement. NIT.

3. **`board/spawn_windows.go` has no `//go:build windows` tag** — it relies on filename convention. This is pre-existing code, not introduced in this change.

4. **No new out-of-plan files** — all files in the batch plan are accounted for.

5. **All plan decisions are implemented** — env sanitization, atomic+locked writes, windowless spawn, cwd-authoritative model, per-repo socket.

6. **Cross-batch contracts satisfied** — `Config` in `cli.go`, `coldRecover`/`coldStart` exported to `daemon.go`, `PsmuxCmd` accessible.

7. **No duplicated helpers** — `sanitizeEnv`, `socketName`, etc. are in one place.

8. **`output.Ok` mutates the supplied map** (see `output.go` line 21: `fields["ok"] = true`). All callers pass fresh map literals, consistent with the documented behavior.

The implementation is clean and well-aligned with the plan. Let me form the final review.

MILL_REVIEW_BEGIN
# Review: Design mhgo mux module — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-12
```

## Findings

### [NIT] Smoke test package deviates from plan (muxpoc vs muxpoc_test)
**Location:** `C:\Code\mhgo\wts\mhgo-mux-design\internal\muxpoc\muxpoc_smoke_test.go:3`
**Issue:** Card 13 specifies `package muxpoc_test` (external test package), but the file uses `package muxpoc`; the plan's own rationale ("avoids internal symbol conflicts") is self-defeating since the test calls unexported functions `cmdUp`, `cmdDown`, etc.
**Fix:** The implementation is pragmatically correct — it must be `package muxpoc` to reach unexported symbols; update the plan prose to remove the contradictory `muxpoc_test` requirement.

### [NIT] socketArg signature diverges from plan spec
**Location:** `C:\Code\mhgo\wts\mhgo-mux-design\internal\muxpoc\cmd.go:122`
**Issue:** Card 4 specifies `func socketArg(cfg Config) string` but the implementation is `func socketArg() string` with no parameter; `cfg` was never used in the described body so this is a dead parameter in the plan.
**Fix:** Harmless improvement; no action required in code, but the batch plan description is slightly stale.

## Verdict

APPROVE
All plan decisions are correctly implemented; two minor plan-vs-code prose mismatches, no blocking issues.
MILL_REVIEW_END
