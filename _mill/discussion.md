# Discussion: Audit and overhaul engine test suites

```yaml
task: Audit and overhaul engine test suites
slug: test-suite-overhaul
status: discussing
parent: main
```

## Problem

The offline (Tier 1) and integration (Tier 2) test loops have picked up two deliberately real-time-wait tests that now dominate their wall-clock on Linux, the machine this task is being done on. Per `docs/benchmarks/test-suite-timing.md`'s `## Linux baseline` → `### 2026-08-01` block:

- Tier 1 (`go test ./...`) grew from ~1.03s to ~6.23s, almost entirely from `TestRunGHAuthTokenSeam_HonoursGhAuthTokenTimeout` (`internal/githubclient/githubclient_test.go`), which is untagged (so it runs in the default offline loop) and deliberately blocks for the full production `ghAuthTokenTimeout = 5 * time.Second` to prove the timeout actually fires.
- Tier 2 (`go test -tags integration ./...`) grew from ~4.97s to ~33.40s, almost entirely from `TestAwaitBatchCmd_ReportPresenceEnvelope/NoReport_WindowElapses` (`internal/webstercli/verbs_test.go`), which drives the `await-batch` CLI verb with no `--wait` flag and so blocks the full production default (`websterengine.DefaultAwaitWaitS`, ~30s) to prove `report:false` is returned once the window elapses.

Why now: implementation work on this branch/hub is currently slow because every step-verification cycle re-runs the suite, and these two tests alone account for the overwhelming majority of both tiers' wall-clock. Neither test is a regression in test *coverage* — both are legitimate, correctly-scoped tests proving real timeout/window behavior. The fix is to make each test prove the same behavior in milliseconds instead of real wall-clock seconds, since both already expose (or can expose) a seam to do so.

## Scope

**In:**
- `internal/githubclient`: turn `ghAuthTokenTimeout` (`internal/githubclient/token.go:67`, currently `const = 5 * time.Second`) into a package-level `var`, with `TestRunGHAuthTokenSeam_HonoursGhAuthTokenTimeout` (`internal/githubclient/githubclient_test.go:524-551`) saving the original value, overriding it to `10ms` for the test, and restoring it via `t.Cleanup` — the same save/override/restore shape the file already uses for its `runGHAuthToken` function-var seam (`withFakeGHAuthToken`, `githubclient_test.go:81-90`), just applied to a duration instead of a function.
- `internal/webstercli`: pass `--wait 1ns` to `TestAwaitBatchCmd_ReportPresenceEnvelope/NoReport_WindowElapses`'s cobra args (`internal/webstercli/verbs_test.go:411`, currently `[]string{"1"}` → `[]string{"1", "--wait", "1ns"}`), matching the `"--wait", "1ns"` convention already used twice in the same file for `recoverBatchCmd`'s near-instant-wait tests (`verbs_test.go:544`, `:578`). No product-code change — same `awaitBatchCmd`/`AwaitBatch` code path, same assertions, just a near-zero window instead of the ~30s production default.
- One opportunistic test-consolidation fold: `TestRecordBatchCmd_DigestEnvelope` and `TestRecordBatchCmd_NoReportEnvelope` (`internal/webstercli/verbs_test.go:447-485` and `:490-526`) share near-identical setup (same `commitFile`, same `initState`, same `st.Batches[1]`/`auditForks` wiring) and differ only in whether `writeBatchReport` is called and the resulting envelope/Terminal state — fold into one table-driven `t.Run` test with two rows.
- Re-measure after the fix (`go run ./cmd/testtiming` and `go run ./cmd/testtiming -full`) and append a new dated block to `docs/benchmarks/test-suite-timing.md`'s `## Linux baseline` section, following the file's existing per-fix block format (Machine/Method/Headline/Cause), recording the Tier 1/Tier 2 before/after.

**Out:**
- Adding `t.Parallel()` to any of the six zero-parallelism packages named in the task brief (`reedengine`, `scoutengine`, `shuttleengine`, `boardengine`, `burlerengine`, `perchengine`) — brief explicitly gates this on re-measuring each package's real cost first via `go run ./cmd/testtiming -full`, and none currently shows as an actual bottleneck (worst is `scoutengine` at ~4.4s). Not started this task.
- Any `t.Parallel()` on `internal/fabricengine`'s (or any other `lyxtest.CopyPaired`/`CopyWeft`-fixture-heavy package's) git-spawning integration tests — confirmed to trigger a false-positive EDR kill on Windows (Cortex XDR); `fabricengine` is also already fast (~4s on Linux). Hard "do not touch."
- Any test that deliberately exercises process-global serialization (e.g. fabric's commit-lock/coalescing tests) — must stay serial.
- Bulk test deletion of any kind. Live-substrate coverage (fabric, reed, scout, shuttle, burler, perch) must not be reduced; only pure-logic tests provably independent of the real substrate are consolidation candidates, and none were identified as needing that treatment this task.
- Two weaker test-consolidation candidates identified during exploration but explicitly rejected for this task: `internal/githubclient/githubclient_test.go`'s four-test `TestAuthRT_*` cluster (lines 377-517 — shared setup boilerplate but assertions genuinely diverge per case, thin payoff) and `internal/webstercli/verbs_test.go`'s `TestBeginBatchCmd_HappyPath`/`PausedEnvelope` pair (lines 334-368, 373-398 — shared `fx.initState` setup but one asserts a completely different envelope shape and opposite state-mutation outcome). Both are left untouched — YAGNI, not worth the complexity for the payoff.
- Filing the separate `/millhouse-issue` about burler/planner's per-card-coverage-with-no-consolidation-step systemic driver. The task body flags this as worth doing but explicitly out of scope here; not part of this task's deliverable.

## Decisions

### githubclient timeout seam shape

- Decision: `ghAuthTokenTimeout` becomes a package-level `var` (not a `const`). The test saves the original value into a local, overrides the package var to `10ms`, and restores it via `t.Cleanup` — a direct duration-var swap, not an added function-indirection layer.
- Rationale: the task brief cited "the same pattern as `internal/perchengine`'s existing timeout seams" as precedent — this precedent does not exist. A dedicated verification pass (two independent explore agents) confirmed `internal/perchengine` has no package-level `var xTimeout = ...` save/restore seam anywhere; the only timeout-shaped declarations there are a `const defaultGateTimeout` and per-call `Timeout time.Duration` struct fields. Repo-wide, the only save/override/restore-via-`t.Cleanup`/`defer` seams that exist are **function**-var seams (`githubclient`'s own `runGHAuthToken`, `internal/ideengine`'s `CodeLauncher`) — never a duration-var seam. Rather than block on the wrong citation or introduce speculative extra indirection to literally match a nonexistent pattern, the direct var-swap is the simplest correct shape and mirrors the *save/restore* idiom the file already uses (just applied to a duration instead of a function value).
- Rejected: (a) waiting/blocking to get the brief corrected before proceeding — the actual instruction ("var instead of const, seam that saves/restores to a few ms") is unambiguous and implementable without the citation; (b) adding a function-wrapper indirection (`var authTokenTimeout = func() time.Duration {...}`) purely to manufacture a "function seam" — adds a layer of indirection with no behavioral benefit over a direct var swap.

### githubclient seam override value

- Decision: override to `10ms` for the test (not `1ms` or a near-zero value).
- Rationale: safely non-zero, avoids any theoretical goroutine-scheduling/context-cancellation flakiness right at the zero boundary, while still cutting the test from 5s to effectively instant. The test's own slack tolerance (`const slack = 5 * time.Second` at `githubclient_test.go`, asserting `elapsed` is between `ghAuthTokenTimeout` and `ghAuthTokenTimeout+slack`) comfortably accommodates 10ms.
- Rejected: `1ms` — no material benefit over `10ms` at these magnitudes, `10ms` has a slightly larger scheduling-noise margin.

### webstercli --wait value

- Decision: `1ns`.
- Rationale: `internal/webstercli/verbs_test.go` already establishes `"--wait", "1ns"` as its convention for near-instant-wait tests, used twice for `recoverBatchCmd` (lines 544, 578). Matching that convention keeps the file internally consistent rather than introducing a second convention (`100ms`, the brief's literal suggestion) for the same kind of test in the same file.
- Rejected: `100ms` as literally suggested in the brief — still ~300x faster than the 30s default and would have worked, but inconsistent with the file's own established idiom.

### Test-consolidation scope

- Decision: fold exactly one pair this task — `TestRecordBatchCmd_DigestEnvelope`/`TestRecordBatchCmd_NoReportEnvelope` in `internal/webstercli/verbs_test.go` — into one table-driven `t.Run` test.
- Rationale: it's a clean, unambiguous fold (near-identical setup, only the report-write step and expected envelope differ) in a file already being touched for the `--wait` fix. Two other candidates were identified during exploration and explicitly rejected (see Scope → Out) for weak payoff-to-complexity ratio. "Opportunistically" in the brief is read as "don't go hunting repo-wide for this task," not "consolidate every marginal candidate found in the two touched files."
- Rejected: folding all three identified candidates (over-scopes a task whose actual goal is wall-clock reduction, not a consolidation sweep); skipping consolidation entirely (leaves a genuinely clean, cheap win on the table in a file already open for edits).

### Benchmark doc update

- Decision: after the code fix lands and passes, re-run `go run ./cmd/testtiming` and `go run ./cmd/testtiming -full`, and append a new dated block to `docs/benchmarks/test-suite-timing.md`'s `## Linux baseline` section recording the before/after, following the file's established per-fix block format (Machine/Method/Headline/Cause, e.g. the existing `restore-tier1-floor` and `2026-08-01` blocks).
- Rationale: this doc is literally the artifact that documents the problem this task fixes (the `### 2026-08-01` block is the causal analysis this task's brief was written from); its own convention is one dated, append-only block per measured change. Leaving the fix undocumented there would make the doc immediately stale and orphan the causal narrative it just established. This is squarely "hardening documented via the existing doc's own convention," not a new module or CLI-behavior change — CLAUDE.md's docs-lifecycle rule doesn't strictly require it, but the benchmark doc's own internal convention does.
- Rejected: skipping the doc update — would leave the doc's most recent narrative block ("here's the problem") without its resolution, breaking the file's own append-only trend-log pattern.

## Technical context

- `internal/githubclient/token.go`: `ghAuthTokenTimeout` (line 67, currently `const = 5 * time.Second`) bounds the `gh auth token` shell-out inside `resolveToken()` (line 106+). The file already has one working seam, `runGHAuthToken` (line 73, `var runGHAuthToken = realRunGHAuthToken`), swapped in tests via the `withFakeGHAuthToken` helper (`githubclient_test.go:81-90`) which does save-original → override → `t.Cleanup`-restore. The new timeout var should follow the identical save/override/restore shape, inline in the test (or via a small helper mirroring `withFakeGHAuthToken`'s structure) rather than a new named helper — a single call site doesn't need its own helper function.
- `internal/githubclient` is a leaf package per CONSTRAINTS.md's GitHub Auth Invariant (allowlisted imports: stdlib, `go-github`, `golang.org/x/sys`, `internal/proc`) — turning a `const` into a `var` does not touch imports and stays within the invariant.
- `internal/webstercli/awaitbatch.go`: `awaitBatchCmd()` already exposes a `--wait` cobra `DurationVar` flag (line 91-93 area); `websterengine.DefaultAwaitWaitS` (~30s, referenced from `internal/websterengine/awaitbatch.go`) is only used when `wait == 0`, i.e. when `--wait` isn't passed. Passing any nonzero `--wait` bypasses the production default entirely — no product code changes needed.
- `internal/webstercli/verbs_test.go`: `TestAwaitBatchCmd_ReportPresenceEnvelope` (lines 405-442ish) has two `t.Run` subtests, `NoReport_WindowElapses` and `ReportPresent`; only the former needs the `--wait` addition since `ReportPresent` returns before the window matters. Both currently build args as `[]string{"1"}` via `clihelp.Execute(fx.CLI.awaitBatchCmd(), &out, []string{"1"})` — the fix changes only the `NoReport_WindowElapses` subtest's args slice to `[]string{"1", "--wait", "1ns"}`.
- `TestRecordBatchCmd_DigestEnvelope` (verbs_test.go:447-485) and `TestRecordBatchCmd_NoReportEnvelope` (verbs_test.go:490-526): both build the same `commitFile`/`initState`/`st.Batches[1]`/`auditForks` fixture; the only branch point is whether `writeBatchReport` is called before invoking the command, which changes the expected envelope fields and the resulting `Terminal` state. Table-driven fold should parametrize on: whether a report is written, and the expected envelope/terminal-state assertions per row.
- `docs/benchmarks/test-suite-timing.md`: reproduction commands are `go run ./cmd/testtiming` (Tier 1) and `go run ./cmd/testtiming -full` (Tier 2); the file's own documented method is "median of 3 warm runs per tier... `go build ./...` run first to warm the build cache." The new block should go under `## Linux baseline`, above the current `### 2026-08-01 — githubclient + webstercli now the floor` block (newest-first ordering per the file's "Append-only... Newest first" convention stated near its trend-log section), and should explicitly reference this task's fixes as the cause, mirroring how the existing `2026-08-01` block references the two tests being fixed.

## Constraints

From `CONSTRAINTS.md`:
- **GitHub Auth Invariant**: all GitHub authentication stays inside `internal/githubclient`; its leaf-import allowlist (stdlib, `go-github`, `golang.org/x/sys`, `internal/proc`) must not widen. The `const`→`var` change doesn't touch imports, so this is a non-issue but worth confirming post-change (`internal/githubclient/leaf_enforcement_test.go` runs on every `go test` and will catch any accidental drift).
- **Test Tier Purity Invariant**: `TestRunGHAuthTokenSeam_HonoursGhAuthTokenTimeout` stays untagged (Tier 1) — the fix doesn't change its spawn profile (it already uses the `runGHAuthToken` fake seam, never a real `gh` process), so no tier reclassification is needed.
- **Hermetic Git Test Environment Invariant**: not implicated — neither target test spawns git.
- **CLI/Cobra Invariant**: the `webstercli` fix only changes test-side args passed to an already-existing, already-registered `--wait` flag; no command registration, `Short`/`Long` text, or envelope shape changes.
- **Documentation Lifecycle** (`docs/overview.md#documentation-lifecycle`): this task doesn't add a module or change observable CLI behavior, so the module-doc/overview.md update trigger doesn't apply; the benchmark-doc update (see Decisions) is done because the doc's own convention calls for it, not because the lifecycle rule requires it.

## Testing

- `internal/githubclient`: `TestRunGHAuthTokenSeam_HonoursGhAuthTokenTimeout` is itself the test being sped up — no new test is added. After the fix, verify with `go test -run TestRunGHAuthTokenSeam_HonoursGhAuthTokenTimeout -v ./internal/githubclient/...` and confirm the reported elapsed time is on the order of the new `10ms` seam value (plus the test's own `slack` tolerance), not ~5s. Also run the package's full `go test ./internal/githubclient/...` to confirm no other test depends on `ghAuthTokenTimeout` being a `const` (e.g. any code that might have relied on compile-time-constant-folding behavior — unlikely, but worth a full-package pass).
- `internal/webstercli`: `TestAwaitBatchCmd_ReportPresenceEnvelope/NoReport_WindowElapses` is the test being sped up. Verify with `go test -tags integration -run TestAwaitBatchCmd_ReportPresenceEnvelope -v ./internal/webstercli/...` and confirm elapsed time is near-instant, not ~30s. The `ReportPresent` subtest is unaffected and should be unchanged.
- `TestRecordBatchCmd_DigestEnvelope`/`NoReportEnvelope` fold: not a TDD candidate (pure refactor of two already-passing tests into one table-driven test) — the two existing test names' assertions must be preserved as two rows of the new table; run `go test -tags integration -run TestRecordBatchCmd -v ./internal/webstercli/...` before and after to confirm identical pass/fail behavior and that both original scenarios (report written vs not) are still exercised.
- End-to-end verification: run full `go test ./...` (Tier 1) and `go test -tags integration ./...` (Tier 2) to confirm `RESULT: all packages passed` and no regressions elsewhere, then run `go run ./cmd/testtiming` and `go run ./cmd/testtiming -full` for the before/after wall-clock numbers feeding the benchmark-doc update.
- No TDD candidates in the strict sense — both core fixes modify existing, already-passing tests' timing behavior rather than adding new coverage; correctness is judged by "same assertions, same code path, faster wall-clock," verified via the `-run`-scoped `-v` timing checks above.

## Q&A log

- **Q:** How should the githubclient timeout seam be shaped, given the brief's cited `perchengine` precedent doesn't actually exist in the repo? **A:** Direct package-var swap (save/override/`t.Cleanup`-restore), mirroring the *save/restore* shape of the file's existing `runGHAuthToken` function-var seam, applied to a duration instead of a function. Not blocking on getting the brief corrected.
- **Q:** What override value for the githubclient test seam? **A:** `10ms` — safely non-zero, ample margin under the test's own 5s slack tolerance.
- **Q:** What `--wait` value for the webstercli fix, given the brief suggests `100ms` but the same file already uses `1ns` elsewhere for identical-shaped tests? **A:** `1ns`, matching the file's existing convention.
- **Q:** How much of the "opportunistic consolidation" item to actually do, given three candidates were found (one clean, two weak)? **A:** Only the one clean-win pair (`TestRecordBatchCmd_DigestEnvelope`/`NoReportEnvelope`); the two weaker candidates are explicitly left untouched this task.
- **Q:** Update `docs/benchmarks/test-suite-timing.md` with a new dated block after the fix? **A:** Yes — re-measure via `go run ./cmd/testtiming[ -full]` and append a new block under `## Linux baseline`, following the file's existing per-fix format.
