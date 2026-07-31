# Batch: docs-and-constraints-wrapup

```yaml
task: Diagnostic tracing (trace) on the logger module
batch: docs-and-constraints-wrapup
number: 14
cards: 2
verify: null
depends-on: [7, 9, 10, 11, 12, 13]
```

## Batch Scope

Terminal documentation batch: updates `docs/shared-libs/README.md`'s `internal/logger` description (now materially inaccurate once trace/span/durable-sink land) and extends CONSTRAINTS.md's Live-Substrate Spawn Observability entry to reference the durable sink and the level policy, once every batch whose call sites/behavior that entry documents has landed. Pure markdown edits, no runnable surface — `verify: null` at both the overview and this batch's own frontmatter.

## Cards

### Card 45: `docs/shared-libs/README.md` — rewrite the internal/logger bullet

- **Context:**
  - `docs/overview.md`
- **Edits:**
  - `docs/shared-libs/README.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Rewrite `docs/shared-libs/README.md:25`'s internal/logger bullet — currently "thin log/slog wrapper (Debug/Info/Warn), silent by default; `-v`/`-vv` wires to it in cmd/lyx/main.go, and `LYX_LOG_LEVEL`/`LYX_LOG_FILE` env vars activate it for entry points (e.g. `go test`) that never reach that CLI flag parsing" — to also name: the process trace-ID (`TraceID()`, `LYX_TRACE_ID` adopt/export) and explicit-parent spans (`StartSpan`/`Child`/`End`) stamping `trace=`/`span=` on every line; the durable, worktree-anchored second sink (`.lyx/logs`, Info+ regardless of verbosity, lazily opened, retained by age+count, capped at 8 MiB); and the `LYX_TRACE=1` test-entry-activation opt-in alongside the existing `LYX_LOG_LEVEL`/`LYX_LOG_FILE` pair. Keep this a one-paragraph bullet in the same style as its neighbors in that file (do not turn it into a multi-paragraph subsection). One line per paragraph, no hard-wrap, per this project's markdown rule.

  `docs/overview.md:190`'s shared-infra sentence (the plain package-name list including internal/logger) needs **no edit** — it already names the package and carries no behavioral description to go stale; confirm this while editing and do not add an unnecessary diff there.
- **Commit:** `docs(shared-libs): describe internal/logger's trace/span/durable-sink surface`

### Card 46: CONSTRAINTS.md — extend Live-Substrate Spawn Observability

- **Context:**
  - `internal/logger/logger.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/shuttleengine/run.go`
  - `internal/burlerengine/engine.go`
  - `internal/scoutengine/ensureserver.go`
  - `internal/perchengine/engine.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Extend CONSTRAINTS.md's "Live-Substrate Spawn Observability" entry:
  - Its opening sentence currently frames activation as depending on `LYX_LOG_LEVEL`/`LYX_LOG_FILE` being set beforehand ("silent by default... but is switched on for a `go test`-only entry point... via the `LYX_LOG_LEVEL`/`LYX_LOG_FILE` environment variables"). Note that the new durable Info+ sink removes that precondition for spawn/teardown events specifically: they now land in the durable trace file regardless of verbosity or env-var configuration, with `LYX_LOG_LEVEL`/`LYX_LOG_FILE` remaining exactly as they were for the stderr-only half.
  - Add a reference to the level policy (discussion.md's `level-policy` decision, now stated in internal/logger's package doc per batch 7's Card 31) so a reader knows where the Warn/Info/Debug definitions and the loop-body hard rule live, rather than restating them here.
  - Update the "Known instrumented call sites today" list (currently naming `internal/reedengine/lifecycle.go`, `internal/shuttleengine/run.go`, `internal/burlerengine/engine.go`) to add `internal/scoutengine/ensureserver.go` (its supervised-daemon spawn, added by batch 13's Card 44 — a genuine spawn-observability call, naming lang/pid/socket). **Do not add `internal/perchengine/engine.go` to this list.** Batch 12's Card 39 call there is a generic error-passthrough `logger.Warn` on `te.Run()`'s result, not a spawn/teardown event — internal/perchengine never itself starts an OS process (the gate subprocess belongs to internal/treadleengine), so listing it under Live-Substrate Spawn Observability would misdescribe what that call does.
  One line per paragraph, no hard-wrap.
- **Commit:** `docs(constraints): extend Live-Substrate Spawn Observability for the durable sink and new call sites`

## Batch Tests

`verify: null` — pure documentation batch, no runnable surface. The overview's module-wide `verify: go vet ./...` still runs at this batch boundary as the final cross-batch check.
</content>
