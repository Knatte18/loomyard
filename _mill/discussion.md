# Discussion: Diagnostic tracing (trace) on the logger module

```yaml
task: Diagnostic tracing (trace) on the logger module
slug: trace-logging
status: discussing
parent: main
```

## Problem

lyx is Go-orchestrated. When a composed, stateful, substrate-driving operation fails deep in a subprocess, nobody is watching — unlike millhouse, where an LLM has oversight and can report what went wrong. Today those failures leave almost nothing behind: a bare `return err` loses its context, and an `fmt.Fprintf(os.Stderr, …)` is swallowed by a test runner's redirection. The composed live paths where this happens are exactly where crucible finds real bugs.

`internal/logger` already exists and is a good primitive — a `log/slog` wrapper with a level threshold, `-v/-vv` wiring, and `LYX_LOG_LEVEL`/`LYX_LOG_FILE` for entry points that never reach CLI flag parsing. But it emits only discrete, level-gated messages to a sink that is silent by default and non-durable, and there is nothing tying one operation's lines together. This task adds the two missing halves: **correlation** (a trace-ID per orchestrated operation, propagated into spawned children, plus spans for sub-steps) and a **durable default sink** that is written whether or not anyone asked for verbosity, so a failure is reconstructable after the fact.

**Why now:** the 2026-07-30 RAM-exhaustion incidents (a crucible round on burler) left no record of what had spawned or how many times — only post-hoc `ps` forensics could reconstruct any of it. That incident produced CONSTRAINTS.md's "Live-Substrate Spawn Observability" entry, which mandates `logger.Info`/`Warn` at every spawn point but still relies on an operator having set `LYX_LOG_LEVEL`/`LYX_LOG_FILE` *before* the failure. A durable Warn+ sink removes that precondition.

## Scope

**In:**

- Trace-ID identity in `internal/logger`: minted or adopted once per process, stamped as a `trace=` field on every emitted line.
- Propagation across process boundaries via the `LYX_TRACE_ID` environment variable, exported back into the process env at the root command so every spawned child inherits it and echoes it into its own lines.
- Explicit spans (`StartSpan`/`Child`/`End`) with span-scoped emit methods, stamped as a `span=` field.
- A durable second sink: `<WorktreeRoot>/.lyx/logs/trace-<UTC>-<id>.log`, pinned at Warn+ regardless of stdout verbosity, created lazily on the first Warn+ write, requiring no env-var opt-in when running under the `lyx` CLI.
- Retention for that sink: an age + count sweep on open, plus a per-file size cap.
- Adoption pass over the six live-substrate engines named in the task body — `reedengine`, `shuttleengine`, `burlerengine`, `perchengine`, `treadleengine`, `fabricengine` — routing error-return paths that currently swallow context through structured `logger`/span calls with key/value pairs.
- Retiring the three remaining ad-hoc stderr sites: `internal/configengine/edit.go`, `internal/scoutengine/ensureserver.go`, `internal/scoutengine/lspclient.go`.
- Docs: `internal/logger` package doc rewrite, `docs/overview.md` shared-infra line, CONSTRAINTS.md amendments (see Constraints below).

**Out:**

- **No `lyx trace` / `lyx logger` CLI module.** No `internal/tracecli`, `internal/traceengine`, or reader command of any kind. This explicitly drops scope item 4 of the task body for this task. Rationale and forward-compatibility notes under Decisions → `no-reader-cli`.
- **No rename of `internal/logger`.** A rename to `internal/trace` (and the `lyx trace` namespace it would enable) is a separate future task, not this one.
- No `slog.LevelError` tier — unrecoverable failures already surface through the `output.Err` JSON envelope and a non-zero exit.
- No hub-wide or cross-worktree trace aggregation; no `--worktree` / `--all` selection.
- No `context.Context` plumbing through engine signatures.
- No adoption pass over the remaining ~22 engine/cli packages. They are mostly thin CLI wrappers where a bare `return err` already reaches the operator through the JSON envelope.
- No JSON-lines log format, no log rotation into numbered siblings, no follow/tail mode.
- No change to the existing `LYX_LOG_LEVEL` / `LYX_LOG_FILE` semantics.

## Decisions

### one-package-not-two

- **Decision:** all trace/span/sink/retention machinery lives inside `internal/logger`, split across files — `logger.go` (threshold, `Debug`/`Info`/`Warn` façade, dual-handler fan-out), `trace.go` (ID mint/adopt, process trace identity), `span.go` (`StartSpan`/`Child`/`End`, span path), `sink.go` (lazy durable-sink open, geometry-resolved path, file naming), `retention.go` (sweep + size cap). No new package is created.
- **Rationale:** tracing is not a separate subject from logging — it is logging plus identity and causal structure, which is how `slog` itself models it (`Logger.With` attributes, groups). Every emitted line must carry the current trace-ID and span path, so a separate package would be called on *every* emit: a hot per-call dependency, not a module boundary. Nothing would ever import one without the other, so the split buys a package boundary and pays a cross-package call per log line for no testability or reuse gain. "This package grew" is answered in Go by more files, not more packages.
- **Rejected:** a new `internal/trace` owning ID/span/sink with `logger` staying a thin façade (the seam does not survive the per-emit coupling); `internal/traceengine` (the `<module>engine` suffix is reserved by the CLI/Cobra Invariant for the kernel of a registered cobra module, and there is no `tracecli`); moving the whole emit API to a new package and deprecating `logger` (churns all 42 existing call sites for no behavioural gain).

### no-reader-cli

- **Decision:** ship no reader command. The durable artifact is a plain text file in a known directory with a timestamped, greppable name; operators and LLM agents open it directly.
- **Rationale:** with one file per trace, `lyx trace list` is `ls` and `lyx trace show <id>` is `cat`. The only genuine addition would be a per-file summary line (worst level, root command, duration, span count) — a nice-to-have costing a registered cobra module, `Short`/`Long` text, updates to `helptree_test.go` / `registration_test.go` / `longlist_test.go` / the root `Long` module list, and a Sandbox Suite Coverage scenario or allowlist entry. That budget is better spent on the adoption pass. The operator's actual question after a failure is "which run was that" — answered by the filename's timestamp, not by a query verb.
- **Forward compatibility (notes, not work):** three properties of this task keep a later reader cheap and must not be traded away. (1) The filename grammar `trace-<YYYYMMDDTHHMMSSZ>-<16-hex-id>.log` is stable and fully parseable, so a future `list` gets timestamp and ID with no file read. (2) Every line carries `trace=` and `span=`, so a future `show` never has to infer structure. (3) The retention sweep is a plain exported-or-not function over a directory path, callable independently of the emit path.
- **Rejected:** `list` + `show` as originally proposed (a renderer the operator stated they would not use in preference to opening the file); `list` only (the summary line is the weakest part of the surface).

### keep-logger-name

- **Decision:** the package stays `internal/logger`. A rename to `internal/trace`, and the `lyx trace` command namespace it would unlock, is deferred to its own task.
- **Rationale:** `lyx <module>` must map to a module, so a future `lyx trace` command backed by `internal/logger` would break the repo's `<module>` ⇔ `<module>cli` ⇔ `<module>engine` naming chain. Since this task ships no CLI, the naming question is not forced now, and renaming costs `git mv` plus 42 call-site edits across 7 packages plus edits to three CONSTRAINTS entries that name `internal/logger` by path. The name also stays accurate to what the API mostly is: `Debug`/`Info`/`Warn`, with span/ID as properties of those lines.
- **Rejected:** renaming to `internal/trace` in this task (real churn for a name that only surfaces in a command not being built); keeping `logger` but planning a `lyx log` module (breaks the naming chain the CLI/Cobra Invariant rests on).

### sink-location

- **Decision:** `<WorktreeRoot>/.lyx/logs/` — worktree-anchored, under the ephemeral dot-`.lyx` directory, resolved through a new `hubgeometry` accessor (e.g. `Layout.WorktreeLogsDir()`), WorktreeRoot-anchored not Cwd-anchored.
- **Rationale:** `_lyx` is the weft-synced tree; a log file there would be committed and pushed, and would collide with the machine-local-artifact exclusion machinery described in CONSTRAINTS.md's Weft Git Invariant. `.lyx` is documented in `hubgeometry.DotLyxDir` as exactly this — "machine-bound, non-weft-synced runtime state" — and `HubLogsDir` already establishes `.lyx/logs` as the forensic-log convention (reed's tmux server logs). WorktreeRoot anchoring follows `LoomStatusFile`/`DiscussionDir` precedent: a caller invoked from a subdirectory must resolve the one true directory for the worktree, not seed a per-subdirectory copy.
- **Note:** this deliberately departs from the task body's "`_lyx`-scoped file" wording, which conflicts with the weft-sync semantics of `_lyx`. The intent of the body ("resolved via hubgeometry, no env-var opt-in, durable") is preserved.
- **Rejected:** hub-anchored `HubLogsDir()` (one place to look, but you must already know which worktree failed, and it interleaves unrelated worktrees); `_lyx/trace/` as literally written in the body (weft-git churn).

### one-file-per-trace

- **Decision:** one file per trace, not a single rolling file. Name: `trace-<YYYYMMDDTHHMMSSZ>-<16-hex-id>.log`, timestamp first so a directory listing sorts chronologically.
- **Rationale:** because the sink opens lazily on the first Warn+ write (see `lazy-sink-open`), a clean run leaves no file at all — so this is one file per *run that had something to say*, not one per invocation, which removes the file-count objection. It also avoids interleaving: several lyx processes can be live in one worktree (a perch loop plus a manual `lyx board sync`), and a shared file would have to be de-interleaved by trace-ID on read. Timestamp-led names are the operator's actual index into "which run was that".
- **Rejected:** a single rolling `trace.log` with numbered rotations (interleaving; and rotation boundaries cut traces in half); newest-K per-run files with no age bound (a count budget does not bound a single runaway run).

### retention

- **Decision:** two independent bounds, both swept when a sink opens (never on every write): delete files older than **14 days**, then keep the **newest 50** regardless of age. Plus a per-file hard cap of **8 MB**: on crossing it the sink stops writing and appends one terminal record noting the truncation.
- **Rationale:** age is what an operator reasons in ("what happened last week"); the count cap is what protects a worktree having a bad week. Sweeping only on open makes retention free for the common case, since a quiet run never opens the sink. At Warn+, 8 MB is roughly 30k warn events in one process — reaching it means the anti-spam rule below was violated, so the cap functions as a spam detector rather than a real operational limit, and the truncation marker is the signal.
- **Gotcha for the plan:** on Windows a file held open by a live sibling process cannot be deleted. The sweep must tolerate per-file delete failures — skip and continue, never fail the sweep or the log call.
- **Rejected:** size-capped rotation of a single file (follows from rejecting the single-file layout); age-only (a busy day is unbounded); count-only (stale files linger indefinitely on a quiet worktree).

### lazy-sink-open

- **Decision:** the root command's `PersistentPreRunE` only *arms* the durable sink (records intent); geometry resolution, directory creation, retention sweep, and file creation all happen on the **first Warn+ write**. If geometry resolution fails (e.g. `lyx` run outside a repo), the durable sink is silently skipped and the command proceeds normally.
- **Rationale:** `hubgeometry.Resolve` shells out to git. Doing it in `logger`'s `init()` would fire on every test binary transitively importing `logger` and fail outright outside a git repo; doing it eagerly in `PersistentPreRunE` would add a git spawn to every command including `lyx --help`. Lazy also means a clean run touches no filesystem at all, which is what makes `one-file-per-trace` viable. Failure to open a diagnostic sink must never break the operation being diagnosed.
- **Rejected:** resolving in `init()` (breaks every test binary and non-repo invocation); resolving eagerly in `PersistentPreRunE` (git spawn per command); hard-failing the command when the sink cannot open.

### logger-imports-hubgeometry

- **Decision:** `internal/logger` imports `internal/hubgeometry` to resolve the sink path. Sink-path construction happens nowhere else.
- **Rationale:** this is precisely what the Hub Geometry Invariant exists for — `hubgeometry` owns all `_lyx`/`.lyx` path construction, and no other package may build such a path from literals.
- **Consequence to record, not a blocker:** `internal/treadleengine`'s seam-enforcement test is a *direct*-import allowlist (`go/parser` with `ImportsOnly`, over that package's own files), so a transitive `logger → hubgeometry` edge does not fail it. But CONSTRAINTS.md's Treadle Runner-Seam Invariant states the exclusion of `hubgeometry` as "the engine is geometry-blind", which becomes true only for direct imports. That entry needs a one-line amendment in the same commit — see Constraints.
- **Rejected:** a `logger.SetDurableSink(io.Writer)` seam with geometry resolved in the wiring layer (keeps `logger` stdlib-only and treadle's transitive graph clean, but pushes geometry knowledge out to callers and makes the durable sink opt-in per entry point, which defeats "no opt-in required").

### text-format-both-sinks

- **Decision:** both sinks use `slog.NewTextHandler`, identical format. The two handlers differ only in destination and level gate.
- **Rationale:** with no reader CLI, the file is read directly by a human or an LLM agent, and text is more readable than JSON lines for the former and fully parseable by the latter. `slog`'s text handler quotes values containing spaces or `=`, so the "text is fragile to parse" concern does not apply. Identical formats also simplify the fan-out and its tests.
- **Rejected:** JSON lines on the durable sink with text on stderr (bought parseability for a programmatic consumer that no longer exists); JSON on both (unpleasant to read under `-vv`).

### dual-handler-fan-out

- **Decision:** the `Debug`/`Info`/`Warn` façade fans one call out to two independently-gated handlers: the stderr text handler gated by the existing `-v`/`-vv` threshold, and the durable text handler pinned at Warn+ regardless of verbosity.
- **Rationale:** the durable sink's entire value is that it records Warn+ without anyone having opted in, so it cannot share the stdout threshold. Conversely a `-vv` run must not flood the durable file with Debug lines — that would defeat retention and the anti-spam rule at once. This split is the piece most likely to break silently in a later refactor, so it gets direct unit coverage.
- **Rejected:** a single handler with a shared threshold (either the file is empty on a normal run or `-vv` floods it); `slog`'s multi-handler via a custom `Handler` composite is the implementation, not an alternative.

### trace-id-mint-and-propagate

- **Decision:** the trace-ID is 8 random bytes rendered as 16 lowercase hex characters. `cmd/lyx/main.go`'s root `PersistentPreRunE` adopts `LYX_TRACE_ID` from the environment when set and non-empty, otherwise mints a fresh one, then exports it back into the process environment via `os.Setenv` so every spawned child inherits it.
- **Rationale:** every `lyx` invocation is then a trace with no per-call-site opt-in, and a nested `lyx` invocation joins its parent's trace rather than starting a disjoint one. Children inherit `os.Environ()` by default, so propagation costs nothing at the spawn sites. 16 hex chars is short enough to read in a log line and collision-free in practice.
- **Sites the plan must verify:** `internal/boardengine/spawn.go:27` and `internal/fabricengine/spawn.go:62` re-exec the lyx binary and inherit env by default (no `cmd.Env` assignment) — confirm they still do. `internal/reedengine/lifecycle.go:338` rebuilds env via `CleanClaudeEnv(os.Environ())`; confirm it strips only Claude session variables and passes `LYX_TRACE_ID` through. `internal/scoutengine/toolchain.go:85` appends to `os.Environ()`, so it is fine.
- **Rejected:** UUIDv4 (36 characters, painful in a log line, needs a dependency or hand-rolled formatting); `<UTC>-<pid>` (collides on recycled PIDs within a second, and duplicates what the filename already carries); minting only inside orchestrated operations (leaves untraced gaps and requires every call site to remember).

### explicit-span-parenting

- **Decision:** spans carry an explicit parent. `logger.StartSpan(name, args...)` opens a root span under the process trace; `sp.Child(name, args...)` opens a nested one; `defer sp.End(err)` closes it. There is no ambient "current span" global. Emission inside a span goes through span-scoped methods (`sp.Debug`/`sp.Info`/`sp.Warn`), which stamp `span=<dotted path>`; the package-level `Debug`/`Info`/`Warn` continue to work unchanged and emit with `trace=` but no `span=`.
- **Rationale:** an ambient package-level span stack is mutable global state — an early `return` or a panic that skips an `End()` leaves it unbalanced, and every *subsequent* span in that process nests wrong. The failure mode is a plausible-looking but false causal structure, which is worse than no structure. Explicit parenting costs one argument and cannot reach that state. Note: Go-side concurrency is not the motivation — production code has exactly one `go func` (`internal/reedengine/lifecycle.go:1091`), no `WaitGroup`, no `errgroup`; and webster's "forks" are Claude subagents that never execute Go.
- **Rejected:** an ambient current-span stack (shorter call sites, corruptible); `context.Context`-carried spans (idiomatic but requires threading `ctx` through engines that have none, for no gain in a near-serial codebase); flat spans with no nesting (loses "which round did this happen inside").

### level-policy

- **Decision:** an explicit, reviewable rule, to be stated in the `internal/logger` package doc:
  - **Warn** — a notable-but-recoverable failure a human would want to know happened even though the operation continued: a retry, a teardown that did not confirm clean, an error swallowed on a fallback path.
  - **Info** — spawn and teardown lifecycle events of real OS processes, per CONSTRAINTS.md's Live-Substrate Spawn Observability entry (session/socket/PID/round identifiers).
  - **Debug** — the step trace.
  - **Hard rule:** nothing at Warn inside a loop body that can iterate more than ~10 times without a state change. Log the state change or the exit, not the iteration.
- **Rationale:** the task body's "no log spam — spam nobody reads is worse than nothing" caveat needs to be checkable by a reviewer rather than left to taste, and the loop clause is the one concrete, mechanically-arguable form of it. The Warn/Info/Debug split codifies what the six already-adopted packages do today.
- **Rejected:** the same three levels without the loop clause (leaves the caveat as pure judgment); adding `slog.LevelError` (duplicates the `output.Err` envelope + non-zero exit channel that already carries unrecoverable failures).

### test-entry-activation

- **Decision:** for entry points that never reach `cmd/lyx/main.go` — `go test` binaries driving live substrate — the durable sink activates via a new `LYX_TRACE=1` environment variable, read on first write alongside the existing `LYX_LOG_LEVEL`/`LYX_LOG_FILE` pair. Opt-in; unset means no durable sink and no filesystem or geometry access.
- **Rationale:** this is the same outside-the-process activation model `internal/logger`'s package doc already documents and justifies for `LYX_LOG_LEVEL`/`LYX_LOG_FILE`, and it is exactly the live-substrate case CONSTRAINTS.md's Live-Substrate Spawn Observability entry exists for. Opt-in keeps ordinary unit tests from touching geometry or writing files.
- **Rejected:** always-on including under `go test` (every test binary importing `logger` would resolve geometry and write files); wiring activation into `lyxtest.HermeticGitEnv` (automatic where it matters, but couples the logger's lifecycle to a test helper and surprises anyone reading the logger in isolation).

### adoption-scope

- **Decision:** this task's adoption pass covers the six live-substrate engines named in the task body — `internal/reedengine`, `internal/shuttleengine`, `internal/burlerengine`, `internal/perchengine`, `internal/treadleengine`, `internal/fabricengine` — auditing error-return paths that swallow context and routing them through structured calls with key/values (operation, worktree, pid). It also retires the three surviving ad-hoc stderr sites: `internal/configengine/edit.go`, `internal/scoutengine/ensureserver.go`, `internal/scoutengine/lspclient.go`.
- **Rationale:** these are the composed paths the task body identifies as where crucible finds real bugs and where errors currently vanish. The remaining ~22 engine/cli packages are largely thin CLI wrappers where a bare `return err` already reaches the operator through the JSON envelope, so instrumenting them is a large mechanical diff with low yield and a real chance of introducing the spam the level policy exists to prevent. Explicitly a "see if it works" first pass; widening is a follow-up.
- **Rejected:** all 29 engine/cli packages; infrastructure-only with adoption deferred entirely (ships a tracing system with nothing to trace).

## Technical context

**Starting point.** `internal/logger/logger.go` is 144 lines: a package-level `slog.LevelVar` initialised to `slog.LevelWarn`, a single `io.Writer` sink defaulting to `os.Stderr`, one `*slog.Logger` over a `slog.NewTextHandler`, `Debug`/`Info`/`Warn` helpers, `SetVerbosity(count int)` mapping a `-v` repeat count to a level, and `SetOutput(w io.Writer)` as a test seam that rebuilds the handler. `configureFromEnv()` (called from `init`) applies `LYX_LOG_LEVEL` and `LYX_LOG_FILE`. All of this is preserved; the new machinery is additive.

**Adoption baseline.** 42 `logger.Debug`/`Info`/`Warn` calls across 7 locations: `internal/treadleengine` (6 files — `handoff.go`, `judge.go`, `run.go`, `targeting.go`, and others), `internal/reedengine` (`lifecycle.go`, `overlay.go`), `internal/shuttleengine` (`run.go`), `internal/perchengine`, `internal/fabricengine` (`coalesce.go`), `internal/burlerengine` (`engine.go`), and `cmd/lyx`. Surviving ad-hoc stderr writers in production code: `internal/configengine/edit.go`, `internal/scoutengine/ensureserver.go`, `internal/scoutengine/lspclient.go` (plus `internal/logger/logger.go` itself and `internal/lyxtest/reexecguard.go`, both of which legitimately cannot log through the logger and stay as they are).

**Geometry.** `internal/hubgeometry/hubgeometry.go` already has `Layout.DotLyxDir()` (Cwd-anchored `.lyx`) and `Layout.HubLogsDir()` (hub-anchored `<Hub>/.lyx/logs`, used only by `internal/reedengine/lifecycle.go:312`). Neither fits: this task needs a WorktreeRoot-anchored `.lyx/logs`, so a new accessor is required, following the anchoring rationale spelled out in `LoomStatusFile`'s doc comment. `hubgeometry`'s only non-stdlib production import is `internal/gitexec`, and `gitexec` does not import `logger`, so `logger → hubgeometry` introduces no cycle.

**Existing forensic-log precedent to follow.** `internal/reedengine/lifecycle.go:312-333` creates `HubLogsDir()`, then calls `pruneServerLogsLocked(logsDir, prefix, keep)` three times for three filename prefixes before writing. That is the closest existing pattern for prefix-scoped retention over a log directory and is worth reading before writing `retention.go`, though this task's policy (age + count + size cap) is different from reed's (newest-3 count only).

**Spawn sites relevant to propagation.** All non-test `exec.Command` call sites are enumerated in the `trace-id-mint-and-propagate` decision above. The only one that rebuilds rather than inherits the environment is `internal/reedengine/lifecycle.go:338` (`CleanClaudeEnv`).

**Root command wiring.** `cmd/lyx/main.go:70` `newRoot()` already installs a `PersistentPreRunE` that calls `logger.SetVerbosity(verbosity)`, with `cobra.EnableTraverseRunHooks = true` guaranteeing it fires before every module's own hook. The trace-ID mint/adopt/export and the durable-sink arming go in that same hook.

**Package doc.** `internal/logger`'s doc comment currently describes the package as a thin `log/slog` wrapper and documents `LYX_LOG_LEVEL`/`LYX_LOG_FILE` at length. It needs a rewrite, not an append: the package is now lyx's diagnostic emit layer, and the doc must cover the trace/span model, the durable sink and its retention, `LYX_TRACE_ID`/`LYX_TRACE`, and the level policy.

## Constraints

From `CONSTRAINTS.md`:

- **Hub Geometry Invariant.** The `.lyx`/`_lyx` geometry tokens and all path construction over them belong to `internal/hubgeometry` alone — `internal/logger` must call a new accessor, never join `.lyx` or `logs` into a path itself. This applies to test code too. `enforcement_test.go`'s `TestEnforcement_GeometryLiterals` catches a geometry token used as a `filepath.Join` argument, a binary-`+` operand, or a `const` value in production files.
- **Treadle Runner-Seam Invariant.** Requires a one-line amendment in the same commit: the entry states `internal/hubgeometry` is deliberately excluded because "the engine is geometry-blind", which after this task is true of direct imports only, since the allowlisted `internal/logger` now pulls `hubgeometry` in transitively. The test itself (a direct-import allowlist) still passes unchanged.
- **Live-Substrate Spawn Observability.** Extend this entry to cover the durable sink and the trace/span model — it currently mandates `logger.Info`/`Warn` at spawn points and explains that activation depends on `LYX_LOG_LEVEL`/`LYX_LOG_FILE` being set beforehand, which the durable Warn+ sink now removes as a precondition. The level policy from `level-policy` above belongs here or in the package doc, referenced from here.
- **Test Tier Purity Invariant.** Untagged test files may not contain `gitexec.RunGit`, `exec.Command`/`exec.CommandContext`, or `lyxtest.Copy*` as raw substrings. All unit tests for the sink, retention, spans, and fan-out must therefore avoid triggering `hubgeometry.Resolve` — they need a seam that accepts a directory or a `*Layout` directly rather than resolving geometry.
- **Hermetic Git Test Environment Invariant.** Any package whose tests spawn git needs a `TestMain` calling `lyxtest.HermeticGitEnv()`. This applies to the integration test's package.
- **CLI/Cobra Invariant.** Not triggered — no new cobra module is registered, so `helptree_test.go`, `registration_test.go`, and `longlist_test.go` need no updates.
- **Sandbox Suite Coverage.** Not triggered — no new registered module, so no `**Covers:**` scenario and no `excludedModules` allowlist entry is required.
- **Documentation Lifecycle** and the project `CLAUDE.md` rule that docs land in the same commit: this task changes cross-cutting infrastructure, so `docs/overview.md` (the shared-infra sentence naming `internal/logger`) and `CONSTRAINTS.md` must be updated in the same commit. `manifest/roadmap.md` does **not** move — this is not a planned roadmap item (grep confirms no trace/logger entry there).
- **Markdown rule** (project `CLAUDE.md`): one line per paragraph, no hard-wrapping, in every `.md` file touched.

Discovered during discussion:

- Failure to open or write the durable sink must never fail the operation being diagnosed — degrade silently to stderr-only.
- The retention sweep must tolerate per-file delete failures (Windows holds open files locked; a sibling lyx process may own one).

## Testing

**Untagged unit tests, `internal/logger`** (no spawns — Test Tier Purity):

- **Dual-handler fan-out** — the highest-value TDD candidate, and the piece most likely to regress silently. Assert: a `Debug` under `-vv` reaches the stderr buffer and does **not** reach the durable sink; an `Info` under `-v` likewise; a `Warn` reaches **both** at every verbosity including the default; a `Warn` with the durable sink unarmed reaches stderr only.
- **Trace-ID mint and adopt** — a set `LYX_TRACE_ID` is adopted verbatim; unset mints a 16-lowercase-hex value; an empty or whitespace-only value is treated as unset; the resulting ID is exported to the process environment; the ID appears as `trace=` on emitted lines.
- **Span nesting** — `StartSpan` → `Child` → `Child` produces the expected dotted `span=` path; `End` restores nothing globally (there is no global to restore); an unended child does not corrupt a sibling's path; `End(err)` records the error.
- **Sink naming and lazy open** — no file exists after `Debug`/`Info` calls alone; the first `Warn` creates exactly one file; the name matches the `trace-<UTC>-<16-hex>.log` grammar and carries this process's ID.
- **Retention sweep** — TDD candidate, pure filesystem logic over a `t.TempDir()`. Cover: files older than the age bound are deleted; files within it are kept; when more than the count bound survive the age pass, the newest N are kept and the rest deleted; a file that cannot be deleted is skipped without failing the sweep or the triggering log call; an empty or absent directory is a no-op.
- **Size cap** — writing past the cap stops further writes and appends exactly one truncation marker; a second write past the cap does not append a second marker.
- **Level policy is documentation, not code** — nothing to assert.

**Test seam required.** Because untagged tests may not spawn git, `internal/logger` needs a way to point the durable sink at a `t.TempDir()` without going through `hubgeometry.Resolve`. `SetOutput` is the existing precedent for exactly this kind of seam; the plan should add the analogous one for the durable sink rather than resolving geometry in tests.

**One `//go:build integration` test** (package needs a `TestMain` calling `lyxtest.HermeticGitEnv()`): drive a real `lyx` invocation in a fixture worktree that provokes a Warn, then assert a trace file lands under `<WorktreeRoot>/.lyx/logs/`, its name matches the grammar, and its contents carry the expected `trace=` field. This is the only test that proves the geometry resolution, the root-command wiring, and the real file path together.

**Propagation check.** A focused test that a child process re-execing the lyx binary reports the parent's trace-ID. If this can be done without a real spawn (asserting the env the command would be given), prefer that so it can stay untagged; otherwise it belongs in the integration tier.

**Adoption-pass tests.** The six engines' existing test suites must stay green; the pass is instrumentation, not behaviour change. New assertions on specific log lines are not wanted — they pin wording and rot. The exception is a swallowed error that becomes an *observable* Warn on a path that previously had no signal at all; there, one assertion that the path logs is worth it.

## Q&A log

- **Q:** Sink location — hub-anchored `HubLogsDir()`, worktree-anchored `.lyx/logs`, or `_lyx/trace/` as the task body literally says? **A:** Worktree-anchored, with the explicit requirement that it not grow without bound for long-lived worktrees.
- **Q:** Should `logger` import `hubgeometry`, or should the wiring layer resolve the path and hand `logger` a writer? **A:** `logger` imports `hubgeometry` — "that is exactly what hubgeometry is for."
- **Q:** Durable-sink format — JSON lines or text? **A:** Delegated. Chose text on both sinks once the reader CLI was dropped; `slog`'s text handler quotes values containing spaces, so the parseability argument for JSON did not hold.
- **Q:** Where is the trace-ID minted? **A:** In the root `PersistentPreRunE` — adopt `LYX_TRACE_ID` if present, else mint, then export back to the env.
- **Q:** Span API — explicit parent handle, ambient stack, or `context.Context`? **A:** Explicit parent handle. The concurrency argument I first gave was wrong (webster forks are Claude subagents and never run Go; production has one `go func` total); the surviving justification is that an ambient stack is corruptible global state.
- **Q:** Retention policy? **A:** Delegated, with the operator's suggestion of one file per session and age-based deletion, and the concern that per-session files might be too many. Resolved as: one file per trace, created lazily on first Warn+ (so quiet runs leave nothing), swept by age and count, with a per-file size cap.
- **Q:** Is a reader CLI needed at all? **A:** No — "I'd rather open the log file directly, and so would an LLM reading it; a datestamped filename makes finding the right one easy." Dropped, with forward-compatibility notes so a later reader stays cheap.
- **Q:** Why did `traceengine` disappear when the CLI was dropped? **A:** It shouldn't have — that was a conflation of "engine" with "read side". Reopened as a real question, then resolved differently: the machinery lives in `internal/logger`, one package.
- **Q:** What is the difference between `trace` and `logger`? **A:** Not enough to justify two packages. Tracing is logging plus identity and causal structure, and every emit needs the trace state, so a package boundary there is a hot per-call dependency, not a seam.
- **Q:** If the package is `logger`, isn't `lyx trace show` inconsistent — modules are reached as `lyx <module>`? **A:** Yes. Keep `internal/logger` for now; a rename to `trace` (and the `lyx trace` namespace) becomes its own separate task.
- **Q:** Adoption breadth in this task? **A:** The six live-substrate engines only, "for now, to see if it works."
- **Q:** How does the durable sink activate under `go test`? **A:** A `LYX_TRACE=1` env var, matching the existing `LYX_LOG_LEVEL`/`LYX_LOG_FILE` opt-in model.
