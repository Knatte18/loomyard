# Batch: engine-supervised-flip

```yaml
task: "Give codeintel a persistent, session-long daemon"
batch: "engine-supervised-flip"
number: 4
cards: 4
verify: go test ./internal/codeintelengine/...
depends-on: [1, 2, 3]
```

## Batch Scope

This batch delivers item 5: it flips `ensureServer`'s single live dispatch arm from `ensureNative` to `ensureSupervised` for Go, carries the shared 10-minute idle timeout onto the supervised daemon, keeps `ensureNative` as an in-function fallback, and closes the now-live wedged-daemon staleness gap with a bounded re-dial-under-lock-then-one-restart escalation using `proc.KillPID`. It depends on batch 1 (`proc.KillPID`), batch 2 (shares `refs.go`/`doc.go` and must land after item 4's engine changes there), and batch 3 (the `buildOptions` `WorktreeRoot` threading must be live before the supervised flip, or batch-mode lookups would anchor a different daemon — item5-batch-worktreeroot-threading). That batch-3 edge is a scheduling constraint, not an import (the engine never imports the CLI); it exists so the prerequisite lands first. Cards are ordered: the shared constant + supervised argv (card 10), then the escalation (card 11, which the flip relies on to make supervised safe to go live), then the flip + docs (card 12), then the integration proof (card 13).

## Cards

### Card 10: Share the daemon idle-timeout constant and set it on the supervised argv

- **Context:**
  - `internal/codeintelengine/daemonstate.go`
- **Edits:**
  - `internal/codeintelengine/ensureserver.go`
  - `internal/codeintelengine/ensureserver_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rename the constant `nativeDaemonIdleTimeout` to a strategy-neutral shared name (e.g. `daemonIdleTimeout`) since it is no longer native-only, and update its doc comment to state it applies to both the native proxy's `-remote.listen.timeout` and the supervised daemon's `-listen.timeout`. Update `nativeArgv`'s reference to the new name. Factor the supervised spawn argv (currently built inline in `ensureSupervised` step 4 as `append(append([]string{}, command...), "serve", fmt.Sprintf("-listen=unix;%s", socketPath))`) into a helper `func supervisedArgv(command []string, socketPath string) []string` (mirroring `nativeArgv`) that appends `"serve"`, `fmt.Sprintf("-listen=unix;%s", socketPath)`, **and** `fmt.Sprintf("-listen.timeout=%s", daemonIdleTimeout)`; call it from `ensureSupervised` step 4. Update the two existing tests in `ensureserver_test.go` that reference the old constant name — `TestNativeArgv_IncludesExtendedIdleTimeout` and `TestNativeArgv_PreservesBinPathAndExtraArgs` — to the new constant name (they break the build otherwise). Add a `supervisedArgv` argv-shape assertion in `ensureserver_test.go`: it includes `serve`, the `-listen=unix;<socket>` flag, and the `-listen.timeout=<daemonIdleTimeout>` flag (no spawn).
- **Commit:** `refactor(codeintelengine): share daemon idle-timeout across native and supervised argv`

### Card 11: Recover a wedged supervised daemon via re-dial-under-lock then one restart

- **Context:**
  - `internal/proc/proc_linux.go`
  - `internal/codeintelengine/daemonstate.go`
  - `internal/codeintelengine/lspclient.go`
- **Edits:**
  - `internal/codeintelengine/ensureserver.go`
  - `internal/codeintelengine/ensureserver_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Close the wedged-daemon gap inside `ensureSupervised`'s step-1 reconnect path. Today, when a non-stale recorded state fails dial-or-finalize, the code falls through to the lock/spawn path, which — finding the state still non-stale under the lock (step 3) — merely sleeps and re-loops, so a wedged-but-alive daemon strands every caller forever. Change it so that a dial-or-finalize failure against a **non-stale** state escalates: acquire the spawn lock (reusing the existing `lock.TryAcquireWriteLock` + bounded-poll machinery and the overall `deadline`), then re-read the state and **re-dial the currently-recorded address under the lock** with the same bounded retry the spawner's step 6 uses (10 attempts × 50 ms) followed by `finalizeConnection`. If that fresh under-lock dial+finalize **succeeds**, use that connection and do **not** kill (covers the races where another caller already respawned, or the same daemon recovered between the failed dial and lock acquisition). Only if the fresh dial+finalize **also fails** is the daemon genuinely wedged: call `proc.KillPID(state.PID)`, remove the stale socket (`os.Remove`, ignoring `os.IsNotExist`), and respawn once via the existing step 4–7 spawn sequence. One restart per call, bounded by `deadline`. The spawner's own step-6 first-bind retry is unaffected (it is not a reconnecting caller and never enters this escalation). Keep `daemonStale` unchanged (pure PID + protocol check). Factor the "re-dial-under-lock, restart-only-if-the-fresh-dial-also-fails" decision into a helper that accepts injectable dial and finalize function values so the three race outcomes are unit-testable with fakes and no real gopls. Add `ensureserver_test.go` tests covering: (a) another caller already respawned — fresh dial succeeds → no kill, connection reused; (b) same daemon recovered — fresh dial succeeds → no kill; (c) genuinely wedged — fresh dial fails → `KillPID` + respawn. These are untagged and process-free (fakes only), except any that legitimately need a live/dead PID fixture may follow the existing `spawnAndHoldSubprocess`/`spawnAndWaitForDeadPID` precedent (`supervised_test.go`/`daemonstate_test.go` are already allowlisted spawners).
- **Commit:** `fix(codeintelengine): recover a wedged supervised daemon via re-dial-under-lock then one restart`

### Card 12: Flip `ensureServer` to supervised with native fallback and update the docs

- **Context:**
  - `internal/codeintelengine/registry.go`
  - `internal/codeintelengine/toolchain.go`
- **Edits:**
  - `internal/codeintelengine/ensureserver.go`
  - `internal/codeintelengine/refs.go`
  - `internal/codeintelengine/doc.go`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite `ensureServer` so that: (1) it resolves the toolchain first via `resolveGoToolchain(ctx, entry.PinnedVersion)` (as `ensureNative` does today) — on failure it returns the wrapped toolchain error **directly**, with **no** native fallback (native needs the identical binary; falling back guarantees an identical failure at doubled latency); (2) it builds `command := append([]string{binPath}, entry.Command[1:]...)` and calls `ensureSupervised(ctx, command, lang, targetDir, worktreeRoot, timeout)`, returning `(client, connKindSupervised, nil)` on success; (3) on **any** error from `ensureSupervised`, it falls back to `ensureNative(ctx, lang, entry, targetDir, timeout)`, returning `connKindNative` (its error verbatim on failure). Because the toolchain is resolved before the supervised attempt, a toolchain-resolution failure never reaches the fallback — this is how "no fallback on toolchain-resolution failure" is enforced structurally; the escalation-first ordering (card 11's restart runs inside `ensureSupervised` and is exhausted before this fallback fires on `ensureSupervised`'s terminal error) is likewise structural. Document both in `ensureServer`'s doc comment. Update the now-false docs in the same commit: `ensureserver.go`'s "exactly one live dispatch arm ... always calls ensureNative" comment (the arm is now supervised with native as fallback) and its `ensureSupervised` "Known limitation" paragraph (the wedged gap is closed by card 11; supervised now has a live V1 dispatch path); `doc.go`'s "The EnsureServer seam" dispatch paragraph and its "Known limitation" paragraph (same reversal); `refs.go`'s `Options.WorktreeRoot` doc comment (it is no longer "unused by every strategy actually reachable in V1" — supervised now reads it to anchor the daemon); and the `docs/overview.md` codeintel module-table clause that currently reads "an EnsureServer daemon lifecycle wired for Go (the native strategy ...) with the supervised strategy built and proven standalone against a plain gopls for a future non-Go language" — correct it to state Go now dispatches to the supervised strategy (lyx-owned state-file/session-long daemon) with native retained as fallback (surgical edit to that clause only; leave the rest of the paragraph intact).
- **Commit:** `feat(codeintelengine): dispatch Go to the supervised daemon with native fallback`

### Card 13: Integration-prove the supervised dispatch flip end to end

- **Context:**
  - `internal/codeintelengine/ensureserver.go`
  - `internal/codeintelengine/daemonstate.go`
  - `internal/codeintelengine/supervised_integration_test.go`
- **Edits:**
  - `internal/codeintelengine/ensureserver_integration_test.go`
  - `internal/codeintelengine/supervised_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `//go:build integration` test to `ensureserver_integration_test.go` (e.g. `TestEnsureServer_Integration_SupervisedDispatch`) skip-gated on `exec.LookPath("gopls")` with `t.Skip(builtins()["go"].InstallHint)`, matching the file's existing tests. It calls `ensureServer(ctx, "go", builtins()["go"], <targetDir>, <worktreeRoot>, 30*time.Second)` and asserts: the returned `connKind` is `connKindSupervised`; a daemon state file exists at `hubgeometry.Layout{WorktreeRoot: worktreeRoot}.CodeintelDaemonStateFile("go")` with an alive PID; and a second `ensureServer` call reuses the same daemon (same PID, stable `Address`) rather than spawning a new one — mirroring `supervised_integration_test.go`'s reuse assertions. Per the `connKindSupervised` teardown rule the test must never `close()`/`kill()` the returned client, but must reap the spawned daemon in `t.Cleanup` (read the state PID and `os.FindProcess(...).Kill()`), exactly as `supervised_integration_test.go` and `supervised_test.go` already do. Update `ensureserver_integration_test.go`'s header comment to note it now also covers `ensureServer`'s supervised dispatch (native is now the fallback path, still directly exercised). Update `supervised_integration_test.go`'s header comment that claims `ensureSupervised` is driven "never through ensureServer's dispatch, which has no live path to it in V1" — that is now false; reword it to reflect that `ensureServer` now dispatches Go to `ensureSupervised` and this file additionally proves the strategy internals directly.
- **Commit:** `test(codeintelengine): integration-prove ensureServer supervised dispatch and reuse`

## Batch Tests

`verify: go test ./internal/codeintelengine/...` runs the untagged engine suite, where item 5's risk actually lives: the shared-timeout / `supervisedArgv` shape (card 10) and the wedged-escalation race decisions (card 11, the (a)/(b)/(c) cases) are unit-tested with the package's fake transport and fake state — no real daemon. The rename in card 10 is compile-guarded by the updated `TestNativeArgv_*` tests. The integration test (card 13) is `//go:build integration` and therefore excluded from this gate per the Test Tier Purity Invariant, matching every other `*_integration_test.go` in the package; the implementer must compile-check it before committing card 13 with `go test -tags integration -run=^$ ./internal/codeintelengine/...` (builds the tagged test binary without spawning gopls), since neither the batch `verify:` nor the repo-wide `done_gate` (`go test ./...`, untagged) compiles integration-tagged files. Full integration execution (a live `ensureServer` spawning and reusing a supervised daemon) is run out-of-band with `-tags integration` on a machine with gopls, alongside the pre-existing `supervised_integration_test.go` strategy-internals coverage.
