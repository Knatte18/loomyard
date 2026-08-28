# Discussion: Audit internal/logger coverage across spawn/hard-error paths

```yaml
task: Audit internal/logger coverage across spawn/hard-error paths
slug: logger-coverage-audit
status: discussing
parent: main
```

## Problem

lyx has a durable diagnostic trail — `internal/logger` fans every `Info`+ record out to a per-process trace file under `<AnchorPath>/.lyx/logs/`, stamped with a process-tree-wide trace ID, retention-swept, and independent of `-v`/`-vv` verbosity.
That trail is the raw material for the bug-reporting and watchdog work being designed on top of loom, and it only works if the events that matter actually reach it.

Mapping loom's per-producer diagnostic trail turned up holes.
`internal/gitexec` — the funnel every git command in the repo passes through — imports `internal/logger` nowhere at all.
Neither does `internal/githubclient`, which owns Publish's GitHub API calls and the `gh auth token` spawn.
`internal/websterengine` spawns a real OS process for the integration verify command and logs nothing about it.
And `shedadapters.SingleLLMProducer` logs richly on the `Asking` → `Stuck` branch — session ID, strand GUID, run directory — but its `Died`/`Timeout` branch returns a bare `fmt.Errorf` carrying none of those fields, so the one outcome a watchdog most needs to reconstruct is the one that leaves the least behind.
Its sibling `burler.go` logs both branches, which makes `singlellm.go` an inconsistency rather than a design choice.

**Why now:** this was found while designing the bug-reporting/watchdog layer, and that layer cannot be specified against a diagnostic trail whose actual coverage is unknown.
CONSTRAINTS.md already carries a **Live-Substrate Spawn Observability** invariant ("any code path starting a real OS process for a round/strand/session logs spawn and teardown via `internal/logger`"), but it is review-discipline only — no test enforces it — and its scope wording ("for a round/strand/session") does not settle whether the spawn sites found above are in or out.
The task is therefore a survey first: enumerate every production process-spawn site and every orchestration-terminating error-return site, table them with a per-site verdict, and only then add the log calls the table justifies.

## Scope

**In:**

- A new audit document, `manifest/designs/logger-coverage.md`, holding the enumerated table of every production process-spawn site and every orchestration-terminating hard-error-return site in the tree, each with a verdict (`covered` / `add` / `excluded (+reason)` / `blocked (+reason)`).
- The code changes that table justifies:
  - `internal/shedadapters/singlellm.go` — `logger.Warn` on the `OutcomeDied`/`OutcomeTimeout` branch and on the `default` unrecognized-outcome branch, carrying the same field set the `OutcomeAsking` branch already carries.
  - `internal/websterengine/integration.go` — `logger.Info` spawn and teardown around `runVerifyCommand`'s `exec.Command`.
  - `internal/treadleengine/gate.go` — `logger.Info` spawn and teardown around the gate command's `exec.CommandContext`.
  - `internal/boardengine/spawn.go` — `logger.Info` spawn and teardown around the `lyx board … sync` re-exec.
  - `internal/vscode/launch_linux.go` / `launch_windows.go` — `logger.Info` on spawn, `logger.Warn` on spawn failure.
  - `internal/configengine/edit.go` — `logger.Info` spawn and teardown around the `$EDITOR` spawn (`configengine` already imports `logger`).
  - `internal/reedengine/proctree_windows.go` — `logger.Debug` on the two pwsh process-tree probes (polling path; `Info` would flood).
  - `internal/selfreportengine` — `logger.Warn` on GitHub API call failures, since `internal/githubclient` itself cannot import `logger` (see Decisions).
- A new tree-wide guard test, `cmd/lyx/spawnobservability_test.go`, following the established `cmd/lyx/*_test.go` guard convention: every production file containing `exec.Command`/`exec.CommandContext` must either import `internal/logger` or appear in an in-test allowlist with a written reason.
- A CONSTRAINTS.md amendment sharpening **Live-Substrate Spawn Observability** to name the guard and its allowlist, so the invariant stops being review-discipline-only.

**Out:**

- **Adding `internal/logger` to `internal/gitexec`.** Structurally impossible today — see the `gitexec-cycle` decision. The audit records it as a finding; no code change.
- **Adding `internal/logger` to `internal/gitkit`.** Blocked by the gitkit Leaf Invariant — see `gitkit-leaf`.
- **Adding `internal/logger` to `internal/githubclient`.** Blocked by the GitHub Auth Invariant's leaf allowlist — see `githubclient-leaf`. Covered at the caller instead.
- **Refactoring `logger`'s `lyxcwd` dependency** to break the cycle. A real option, deliberately rejected here — see `gitexec-cycle`'s rejected alternatives.
- **Any change to `internal/logger` itself.** No new levels, no new sinks, no API additions. The package is the instrument, not the subject.
- **Test files, `tools/`, `cmd/testtiming`, and `internal/hubforge`.** Test-only and fixture-only spawn paths; enumerated in the audit table as `excluded` with a reason, never given log calls.
- **Every `if err != nil { return err }` in the tree.** The hard-error universe is bounded by the `error-universe` decision below; an exhaustive error-return enumeration would be thousands of rows and would not be read.
- **`manifest/roadmap.md`.** Hardening-shaped, no roadmap entry per project convention.
- Changing any existing log line's level, message, or field set. Additive only.

## Decisions

### deliverable-shape

- **Decision:** One task producing both the audit document and the code changes it justifies, with the document written first and the changes derived from its verdicts. Not split across two tasks.
- **Rationale:** The brief's "table it, then decide what actually needs a log call added — before implementing anything" is a sequencing constraint *within* the work, not an instruction to stop after the table. Splitting would strand the table with no consumer and force a second discussion round to re-derive verdicts already reached here.
- **Rejected:** Audit-only, deferring code to a follow-up — leaves known, cheap gaps open for no benefit. Code-only, no document — loses the exclusion reasons, which are the part that stops the next reader re-litigating `gitexec`.

### error-universe

- **Decision:** The audit enumerates two bounded universes.
  **Spawn sites:** every `exec.Command` / `exec.CommandContext` occurrence in production (non-`_test.go`) Go files under `internal/` and `cmd/`.
  **Hard-error-return sites:** only error returns that terminate an orchestration unit — a Shed producer's `Died`/`Timeout`/unrecognized-outcome mapping, an engine `Run`'s failure return, a segment or round abandoning — not every propagated `err`.
- **Rationale:** The spawn universe is mechanically enumerable and matches what the guard test can check. The hard-error universe has no mechanical boundary, so it needs a judgment rule; "terminates an orchestration unit" is the rule that selects exactly the events a watchdog reconstructing a failed run needs, and excludes the pass-through wrapping that carries no new information.
- **Rejected:** Every `return …, err` in production code — thousands of rows, unusable as a table and unenforceable as a guard. Only the four modules the brief names — the brief explicitly says "across the whole tree (not just loom's producers)".

### gitexec-cycle

- **Decision:** `internal/gitexec` does not gain a `logger` import. The audit records the reason as a structural finding.
- **Rationale:** It is an import cycle, not an oversight. `internal/logger/sink.go` imports `internal/lyxcwd` (for `LogsDir(*lyxcwd.Location)` and the lazy `Getwd`/`Resolve` sink bootstrap), and `internal/lyxcwd` imports `internal/gitexec` — mandated by CONSTRAINTS.md's Cwd Resolution Invariant, which pins lyxcwd's imports to "stdlib + `internal/gitexec` only". So `gitexec → logger → lyxcwd → gitexec`. Beyond the cycle, the diagnostic information is not actually lost: `gitexec.Run` already returns a `*GitError` carrying args, dir, exit code, and stderr, so a failed git command is fully reconstructable at whichever caller decides it mattered. Logging inside `gitexec` would also fire on every git invocation in the process, including the many where a non-zero exit is an answer rather than a failure (`RunGit`'s entire reason to exist).
- **Rejected:** Break the cycle by extracting logger's sink-path resolution behind a caller-injected seam (`logger.SetLogsDir`) so `logger` no longer imports `lyxcwd` — feasible, but it is an invasive refactor of the durable sink's bootstrap and a change to `logger`'s public surface, undertaken to buy `Debug`-level volume on the tree's single hottest code path. Out of proportion to the value. Introduce a separate ultra-leaf `internal/logtrace` package `gitexec` could import — a second logging package solely to route around one import edge, splitting the trace trail across two implementations.

### gitkit-leaf

- **Decision:** `internal/gitkit` does not gain a `logger` import. Its three `exec.Command` sites are recorded in the audit as `blocked (gitkit Leaf Invariant)` and added to the guard test's allowlist with that reason.
- **Rationale:** CONSTRAINTS.md's gitkit Leaf Invariant pins gitkit's imports to "stdlib, `lyxcwd`, `weftname`, `configengine`, `lyxdirs`". Admitting `logger` would widen a deliberately narrow leaf. gitkit's spawns are repo-copy plumbing driven from fixture and setup paths, not round/strand/session lifecycle events.
- **Rejected:** Amend the Leaf Invariant to admit `logger` — the invariant's value is that it is narrow; widening it for diagnostics sets the precedent that any leaf can be widened for any cross-cutting concern.

### githubclient-leaf

- **Decision:** `internal/githubclient` does not gain a `logger` import. GitHub API failures are logged one layer up, in `internal/selfreportengine`, which already holds the operation context (which issue, which repo, which verb).
- **Rationale:** `internal/githubclient/leaf_enforcement_test.go` enforces the GitHub Auth Invariant's leaf half as a strict allowlist — production code there may import only go-github, `golang.org/x/sys/windows`, and `internal/proc`. The allowlist is deliberately maintenance-free: anything outside it fails. Adding `logger` means amending an invariant to gain logging in a package that is pure auth plumbing with no operation-level context to log — `githubclient` knows a token resolution failed, but not what the caller was trying to do. The caller knows both.
- **Rejected:** Amend the allowlist to admit `logger` — same precedent objection as `gitkit-leaf`, and it would put the log lines at the layer with strictly less context. Accept the gap with no change anywhere — leaves Publish's failures genuinely absent from the trace file, which is the concrete gap the brief flagged.

### singlellm-parity

- **Decision:** `SingleLLMProducer.mapOutcome`'s `OutcomeDied`/`OutcomeTimeout` branch and its `default` unrecognized-outcome branch each gain a `logger.Warn` immediately before their existing error return, carrying the same fields the `OutcomeAsking` branch already emits — `producer`, `engine`, `sessionID`, `strandGUID`, `runDir`, plus `outcome`.
- **Rationale:** `Warn` matches the package's level policy ("a notable-but-recoverable failure") as the sibling branches read it, matches what `burler.go` already does on its own `OutcomeDied`/`OutcomeTimeout` branch, and is the only level that reaches both the stderr and durable sinks without `-v`. `mapOutcome` is not a loop body, so the level policy's no-`Warn`-in-a-hot-loop rule does not apply. The `default` branch is included because an unrecognized outcome string is strictly more surprising than a recognized failure and currently leaves nothing behind at all.
- **Rejected:** `Died`/`Timeout` only, leaving `default` bare — the cheaper half of the same fix, for no reason. `Info` — inconsistent with the `Asking` branch beside it and with `burler.go`, and the level policy reserves `Info` for spawn/teardown lifecycle events, which this is not.

### spawn-site-verdicts

- **Decision:** Per-site verdicts for the remaining production spawn sites, as enumerated during exploration:

| Site | Spawns | Verdict |
| --- | --- | --- |
| `internal/reedengine/lifecycle.go` | 1 | covered (31 logger lines) |
| `internal/reedengine/attach.go` | 1 | covered |
| `internal/reedengine/overlay.go` | 2 | covered |
| `internal/reedcli/attach.go` | 1 | covered |
| `internal/loomcli/run.go` | 2 | covered |
| `internal/fabricengine/spawn.go` | 1 | covered |
| `internal/websterengine/integration.go` | 1 | add — `Info` spawn + teardown (exit code) |
| `internal/treadleengine/gate.go` | 1 | add — `Info` spawn + teardown (exit code, duration) |
| `internal/boardengine/spawn.go` | 1 | add — `Info` spawn + teardown |
| `internal/vscode/launch_linux.go` | 2 | add — `Info` spawn, `Warn` on spawn failure |
| `internal/vscode/launch_windows.go` | 1 | add — `Info` spawn, `Warn` on spawn failure |
| `internal/configengine/edit.go` | 1 | add — `Info` spawn + teardown |
| `internal/reedengine/proctree_windows.go` | 2 | add — `Debug` only (polling probe) |
| `internal/gitexec/gitexec.go` | 2 | blocked — import cycle (`gitexec-cycle`) |
| `internal/gitkit/gitkit.go` | 3 | blocked — gitkit Leaf Invariant (`gitkit-leaf`) |
| `internal/githubclient/token.go` | 1 | blocked — GitHub Auth leaf allowlist (`githubclient-leaf`) |
| `internal/hubforge/hub.go` | 2 | excluded — test-fixture builder, not a production path |
| `tools/deploy/main.go`, `tools/sandbox/*` | 7 | excluded — build/dev tooling, outside `internal/` and `cmd/` |
| `cmd/testtiming/main.go` | 1 | excluded — test-timing harness |

- **Rationale:** `Info` for real OS-process spawn/teardown is exactly what the package's level policy prescribes. `proctree_windows.go` is the one exception: both its spawns sit inside a polling probe called repeatedly against a live process tree, so `Info` would flood the durable sink — `Debug` keeps the trail without the volume. Excluded sites are excluded because they are not production paths, not because logging them would be wrong.
- **Rejected:** `Info` uniformly including `proctree_windows.go` — floods the durable sink for a probe. Skipping the `vscode` and `configengine` sites as "just a UI launch" — they are real OS spawns whose failure is what an operator reports as "nothing happened", and they cost two lines each.

### enforcement-guard

- **Decision:** A new `cmd/lyx/spawnobservability_test.go` walks production (non-`_test.go`) `.go` files under `internal/` and `cmd/` and fails any file containing `exec.Command` or `exec.CommandContext` that neither imports `internal/logger` nor appears in an in-test allowlist keyed by file path, each entry carrying a written reason.
- **Rationale:** `cmd/lyx/` is this repo's established home for tree-wide AST/substring guard tests — `tierpurity_test.go`, `checkedcall_test.go`, `rawgitmutation_test.go`, `cwdmutation_test.go`, `ghguard_test.go`, `uncontainedwrite_test.go`, `sandbox_coverage_test.go` all live there and all follow the same walk-plus-allowlist shape. The allowlist-with-reason form matches the Sandbox Suite Coverage invariant's own "exercised or explicitly excluded with a reason" pattern, and it is what converts this audit from a snapshot that rots into an invariant that holds: a new package reaching for `exec.Command` without logging fails the build, and the author must either log or write down why not.
- **Rejected:** No guard, audit document only — the document is a snapshot of one afternoon; six months of new spawn sites erase it silently, which is precisely how this task's gaps accumulated. A guard covering only the four modules the brief names — leaves the rest of the tree exactly as unenforced as it is today.

**Known blind spot to document in the guard's own header comment,** matching the candour of `checkedcall_test.go`'s: file-level import presence is coarse. A file that imports `logger` for an unrelated line and spawns a process unlogged still passes. The guard catches the regression shape that actually occurs — a brand-new spawn in a package with no logging at all — and does not claim more.

### audit-doc-location

- **Decision:** `manifest/designs/logger-coverage.md`, a new design document. `docs/overview.md` gains no new row (no new module); CONSTRAINTS.md's **Live-Substrate Spawn Observability** section gains a pointer to it and to the new guard.
- **Rationale:** `manifest/designs/` already holds cross-cutting design documents that are not per-module (`review-finding-classification.md`, `curation-triage.md`, `code-comment-conventions.md`), which is the shape this is. The Documentation Lifecycle requires cross-cutting infrastructure to update CONSTRAINTS.md in the same commit, and the guard plus the sharpened invariant wording are that update.
- **Rejected:** `docs/reference/` — that tree is user-facing reference, not design rationale. Inline in CONSTRAINTS.md — CONSTRAINTS.md is a short authoritative list of forms, not a table of call sites; a twenty-row table there would drown it.

## Technical context

**`internal/logger` (unchanged by this task).** Thin `log/slog` wrapper. Package-level API: `Debug`/`Info`/`Warn(msg string, args ...any)` (key/value variadic), `SetVerbosity(count int)`, `SetOutput(io.Writer)`, `TraceID()`, `MintOrAdoptAndExport()`, `StartSpan`/`Child`/`End`. A `dualHandler` fans every record to a stderr half (gated by `-v`/`-vv`) and a durable half (gated at `Info`+ unconditionally). Consequence that matters here: **`Debug` never reaches the durable trace file.** A `Debug` line is a `-vv` convenience only; anything that must survive into a bug report has to be `Info` or `Warn`.

**Level policy (from the package doc — follow it exactly).**
`Warn` = notable-but-recoverable failure (retry, unconfirmed teardown, error swallowed on a fallback path).
`Info` = a real OS-process spawn/teardown lifecycle event.
`Debug` = everything else worth a line.
Hard rule: nothing logs at `Warn` inside a loop body that can iterate more than roughly ten times without an intervening state change.

**Field-naming convention observed across existing call sites** (`shedadapters/*.go`, `reedengine/*.go`): message strings are prefixed with the package name (`"shedadapters: …"`), and fields are lowerCamelCase keys — `producer`, `engine`, `round`, `sessionID`, `strandGUID`, `runDir`, `outcome`, `cause`, `path`. New lines must match; `cause` (not `err` or `error`) is the established key for a wrapped error, though `burler.go:307` uses `error` — prefer `cause`, the majority spelling.

**The cycle, concretely.** `internal/logger/sink.go:21-22` imports `lyxcwd` and `lyxdirs`; `LogsDir(l *lyxcwd.Location) string` at `sink.go:33` and the lazy bootstrap at `sink.go:88,93` (`lyxcwd.Getwd()` / `lyxcwd.Resolve(cwd)`) are the dependencies. `internal/lyxcwd` imports `internal/gitexec`. Adding `logger` to `gitexec` closes the loop. Verified by reading imports directly, not inferred.

**`internal/gitexec/gitexec.go`.** 134 lines, imports stdlib + `internal/proc` only. `runCore` is the single exec core; `Run` (checked, returns `*GitError`) and `RunGit` (raw, non-zero exit is an answer) are thin wrappers. `GitError` carries `Args`, `Dir`, `ExitCode`, `Stderr` — the reason a caller can log a failed git command without `gitexec` doing it.

**`internal/githubclient/leaf_enforcement_test.go`.** A `go/parser` `ImportsOnly` walk over non-test files in the package, failing any import outside a three-entry `allowedImports` map. It is the mechanism that makes the `githubclient-leaf` decision non-negotiable without an explicit invariant amendment.

**`internal/shedadapters/singlellm.go`.** `mapOutcome` at line ~155. `OutcomeAsking` branch (line 171) is the model to copy: `cancelErr` guard first, then `logger.Warn(…)`, then return. `OutcomeDied, shuttleengine.OutcomeTimeout` at line 174 and `default` at line ~180 have the `cancelErr` guard and the return but no log line. `result` is a `shuttleengine.Result` carrying `Outcome`, `SessionID`, `StrandGUID`, `RunDir`, `LastAssistantMessage`. Sibling precedent: `internal/shedadapters/burler.go:394` and `:465` both log `Warn` on their own `OutcomeDied`/`OutcomeTimeout` branches with `outcome` and `sessionID` fields.

**`internal/websterengine/integration.go`.** `runVerifyCommand(verifyCmd, worktree string) (bool, error)` at line 141 picks `sh -c` / `cmd /C` by `runtime.GOOS`, sets `cmd.Dir = worktree`, and `cmd.Run()`s. `*exec.ExitError` is a failed verify (`false, nil`); anything else propagates. The package does not currently import `logger` — this adds the import. Note the two distinct outcomes deserve distinct treatment: a non-zero verify exit is an expected answer (`Info` teardown with the exit code), a spawn failure is not (`Warn`).

**`cmd/lyx/` guard-test convention.** Read `cmd/lyx/checkedcall_test.go` before writing the new guard — its header comment is the model for documenting a guard's token choices, its walk scope, and its known blind spot. Other siblings worth skimming for the walk skeleton: `tierpurity_test.go`, `uncontainedwrite_test.go`, `sandbox_coverage_test.go`. `cmd/lyx/testmain_test.go` already exists; the new file joins the same package `main` test binary.

**Import additions required.** `internal/websterengine`, `internal/treadleengine` (already imports `logger` elsewhere — check `gate.go` specifically), `internal/boardengine`, `internal/vscode` (both `launch_*.go`), `internal/reedengine/proctree_windows.go` (package already imports `logger`), `internal/selfreportengine`. `internal/configengine` already imports `logger` (`config.go`); `edit.go` needs the import added to that file.

**Cross-compilation.** `internal/vscode/launch_windows.go`, `internal/reedengine/proctree_windows.go`, and `internal/proc/proc_windows.go` are Windows-only build-tagged files. `cmd/lyx/crosscompile_test.go` exists — the Windows-tagged edits must still compile under `GOOS=windows`, and that test is what catches it.

## Constraints

From `CONSTRAINTS.md`:

- **Live-Substrate Spawn Observability** — "Any code path starting a real OS process for a round/strand/session logs spawn and teardown via `internal/logger`." This task sharpens this invariant's wording and gives it its first enforcing test. Its two sub-bullets (never re-exec `os.Executable()` under `go test`; a retry loop around a real spawn caps attempt COUNT) are unchanged.
- **Cwd Resolution Invariant** — `internal/lyxcwd`'s imports are pinned to "stdlib + `internal/gitexec` only". This is one half of the `gitexec` cycle and must not be relaxed.
- **gitkit Leaf Invariant** — `internal/gitkit` imports only stdlib, `lyxcwd`, `weftname`, `configengine`, `lyxdirs`. Not widened.
- **GitHub Auth Invariant** — all GitHub authentication goes through `internal/githubclient`; its leaf half is allowlist-enforced by `leaf_enforcement_test.go`. Not widened.
- **Test Tier Purity Invariant** — untagged test files perform no expensive spawns; no `gitexec.Run`/`RunGit`, `exec.Command`/`CommandContext`, `gitkit.Copy*`, `hubforge.NewHub` outside `integration`/`smoke`-tagged files. The new guard test is a source scan, not a spawner, so it stays untagged — but it must not be written in a way that trips `tierpurity_test.go`'s own token scan of `cmd/lyx/`. Check how the sibling guards that mention these same tokens in their allowlists avoid this before writing.
- **Documentation Lifecycle** — cross-cutting infrastructure updates CONSTRAINTS.md in the same commit. The guard plus the sharpened invariant text satisfy this. `docs/overview.md` needs no change (no new module, no execution-stack change). `manifest/roadmap.md` does not move — hardening-shaped.
- **Markdown Link Integrity** — every inline link in `manifest/`/`docs/` `.md` files must resolve, file part and `#anchor`. The new `manifest/designs/logger-coverage.md` and any link to it from CONSTRAINTS.md must satisfy this.

Project conventions (`CLAUDE.md`):

- Markdown uses semantic line breaks — one sentence per line, breaking at internal independent-clause boundaries. Applies to `manifest/designs/logger-coverage.md` and to any CONSTRAINTS.md prose edited.
- Table cells and blockquotes stay on one line.

Discovered during exploration:

- `Debug` never reaches the durable sink. Any site whose verdict is "`Debug` only" is deliberately choosing not to be in the bug-report trail.
- `logger.Warn` reaches both sinks with no flag, which is why the hot-loop rule exists — respect it at every new site.
- Existing message strings are package-prefixed and fields are lowerCamelCase; new lines that break this make the trace file harder to grep, which is its whole purpose.

## Testing

**`internal/shedadapters` — TDD candidate.** The `singlellm.go` change is the clearest test-first case in the task. `mapOutcome` is directly reachable and `shuttleengine.Result` is a plain value, so a table-driven test can drive each outcome (`Died`, `Timeout`, an unrecognized string) and assert both the returned error and the emitted log line. Capture output via `logger.SetOutput` — `internal/logger/logger_test.go`'s `withCapturedOutput` helper is the established pattern; check whether it is exported or needs a local equivalent in `shedadapters`. Assert the presence of the field keys (`sessionID`, `strandGUID`, `runDir`, `outcome`) and the `Warn` level, not the exact rendered string. Cover the case where `cancelErr` fires first — the log line must not be emitted on the cancellation path, since that path returns before reaching the outcome mapping's own return.

**`cmd/lyx/spawnobservability_test.go` — TDD candidate, and the test *is* the deliverable.** Write it against the current tree and confirm it fails on exactly the sites the audit marks `add`, then let the allowlist and the added log calls bring it green. That failure list is a live check on the audit table's completeness: any site the guard flags that the table does not mention is a row the survey missed. Also verify the guard is not vacuous — a temporary local edit adding a bare `exec.Command` to an unlogged package must fail it.

**`internal/websterengine` — needs a test.** `runVerifyCommand` has two distinct paths (non-zero exit → `false, nil`; spawn failure → error), and they must log differently. A test driving a command that exits non-zero and one naming a binary that does not exist, asserting `Info`-with-exit-code and `Warn` respectively, pins the distinction. Note this spawns real processes, so tier placement matters — check whether the existing `integration_test.go` in that package is build-tagged and follow it.

**`internal/treadleengine`, `internal/boardengine`, `internal/configengine`, `internal/vscode`, `internal/reedengine` — no new dedicated tests.** These are single additive log lines around existing, already-tested spawn calls. The guard test covers the structural property (the file logs at all); a per-site log-content test at each would be test-for-test's-sake. Existing tests in each package must still pass — the risk is a `logger` import breaking a package's own leaf or seam-enforcement test, so run each package's full suite, particularly `internal/treadleengine/seam_enforcement_test.go`.

**Cross-compile check.** `cmd/lyx/crosscompile_test.go` must pass after the Windows-tagged edits to `internal/vscode/launch_windows.go` and `internal/reedengine/proctree_windows.go`. This is the only check that sees those files if the work is done on Linux.

**Documentation checks.** `manifest/designs/logger-coverage.md` must satisfy the Markdown Link Integrity guard, and the CONSTRAINTS.md amendment must not break any existing link. Run whatever test enforces that invariant (locate it — likely a `manifest/`-scanning test) rather than assuming.

**Whole-tree gate.** `go build ./... && go test ./...` must pass. The `logger` imports added across seven packages are the kind of change that trips an unrelated import-allowlist test in a package nobody thought to check, so the full suite is the real verification, not the per-package runs.

## Q&A log

- **Q:** Is the deliverable an audit document, code changes, or both? **A:** [auto-pick] Both, in one task, document first. **Why:** the brief's "before implementing anything" sequences the work inside the task; splitting would strand the table and force a second discussion to re-derive the same verdicts.
- **Q:** What bounds the enumeration — what counts as a "hard-error-return site"? **A:** [auto-pick] Spawn sites = every production `exec.Command*`; hard-error sites = error returns that terminate an orchestration unit, not every propagated `err`. **Why:** an exhaustive error enumeration is thousands of unusable rows; "terminates an orchestration unit" selects exactly what a watchdog reconstructing a failed run needs.
- **Q:** `internal/gitexec` has no `logger` import — add one? **A:** [auto-pick] No; record the import cycle as a finding. **Why:** `gitexec → logger → lyxcwd → gitexec`, with the `lyxcwd → gitexec` edge mandated by the Cwd Resolution Invariant. `GitError` already carries args, dir, exit code, and stderr, so callers can log what mattered.
- **Q:** Break the cycle by extracting `logger`'s sink-path resolution behind a caller-injected seam? **A:** [auto-pick] No. **Why:** an invasive refactor of the durable sink's bootstrap plus a public-API change, undertaken to buy `Debug`-level volume on the tree's hottest path.
- **Q:** `internal/githubclient` has no `logger` import — add one? **A:** [auto-pick] No; log at the caller (`internal/selfreportengine`). **Why:** the GitHub Auth Invariant's leaf half is allowlist-enforced, and `githubclient` is pure auth plumbing with no operation-level context — the caller knows both the failure and what it was attempting.
- **Q:** `internal/gitkit`'s three spawns? **A:** [auto-pick] Blocked by the gitkit Leaf Invariant; allowlist with that reason. **Why:** the invariant's value is that it is narrow; widening it for diagnostics sets a precedent that any leaf can be widened for any cross-cutting concern.
- **Q:** `shedadapters.SingleLLMProducer` — which branches gain a log, and at what level? **A:** [auto-pick] `Died`/`Timeout` *and* `default`, at `Warn`, with the `Asking` branch's field set. **Why:** matches the sibling `burler.go` precedent and the level policy; an unrecognized outcome is more surprising than a recognized failure and currently leaves nothing behind.
- **Q:** `internal/reedengine/proctree_windows.go`'s two pwsh probes — `Info` like the other spawn sites? **A:** [auto-pick] `Debug` only. **Why:** both sit inside a polling probe against a live process tree; `Info` reaches the durable sink unconditionally and would flood it.
- **Q:** Add an enforcement mechanism, or is the document the record? **A:** [auto-pick] Add `cmd/lyx/spawnobservability_test.go`, allowlist-with-reason. **Why:** a document is a snapshot that rots — that rot is exactly how these gaps accumulated. `cmd/lyx/` is the established home for tree-wide guards, and the allowlist form matches the Sandbox Suite Coverage invariant's own pattern.
- **Q:** Where does the audit document live? **A:** [auto-pick] `manifest/designs/logger-coverage.md`. **Why:** that directory already holds cross-cutting, non-per-module design documents; `docs/reference/` is user-facing, and a twenty-row table in CONSTRAINTS.md would drown a file whose value is brevity.
- **Q:** Does `manifest/roadmap.md` move? **A:** [auto-pick] No. **Why:** hardening-shaped, per the project's own roadmap convention, and the brief says so explicitly.
