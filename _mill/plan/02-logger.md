# Batch: logger

```yaml
task: 'Build internal/mux: the window to the world (overlay + strands + render)'
batch: 'logger'
number: 2
cards: 1
verify: go test ./internal/logger/...
depends-on: []
```

## Batch Scope

Creates `internal/logger`, a thin `log/slog` wrapper with a package-level `slog.LevelVar`,
an **injectable** `io.Writer` sink defaulting to the real `os.Stderr`, `Debug`/`Info`/`Warn`
helpers, and a `SetVerbosity(count int)` mapping (0 -> Warn, 1 -> Info, >=2 -> Debug). The
external interface later batches consume: `logger.Debug/Info/Warn(msg string, args
...any)`, `logger.SetVerbosity(int)`, and `logger.SetOutput(io.Writer)` (test seam). The
root `-v/--verbose` flag that drives `SetVerbosity` is wired in batch 7 (`cmd/lyx/main.go`);
this batch delivers only the package. Batch-local decisions: the sink is deliberately **not**
routed through `clihelp`'s stdout/stderr seam (so stdout stays a clean JSON stream and logs
go to stderr); default level is `Warn` (non-negotiable — a normal run emits zero log lines).

## Cards

### Card 2: internal/logger — slog wrapper, LevelVar, injectable stderr sink, SetVerbosity

- **Context:**
  - `internal/output/output.go`
- **Edits:** none
- **Creates:**
  - `internal/logger/logger.go`
  - `internal/logger/logger_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/logger/logger.go` (package `logger`): declare a package
  `var levelVar slog.LevelVar` initialised to `slog.LevelWarn`; a package sink `var out
  io.Writer = os.Stderr`; and a lazily/rebuildable `*slog.Logger` over a
  `slog.NewTextHandler(out, &slog.HandlerOptions{Level: &levelVar})`. Expose:
  `func Debug(msg string, args ...any)`, `func Info(msg string, args ...any)`, `func
  Warn(msg string, args ...any)` delegating to the slog logger at the matching level;
  `func SetVerbosity(count int)` setting `levelVar` to `slog.LevelWarn` for count<=0,
  `slog.LevelInfo` for count==1, `slog.LevelDebug` for count>=2; and `func SetOutput(w
  io.Writer)` that rebinds the sink and rebuilds the handler (test seam). In
  `internal/logger/logger_test.go`: assert (a) with default level, `Info` and `Debug`
  calls emit **zero** bytes to an injected buffer sink; (b) after `SetVerbosity(1)`, `Info`
  emits a line but `Debug` does not; (c) after `SetVerbosity(2)`, `Debug` emits; (d)
  `SetOutput` captures into a caller-supplied `bytes.Buffer`. Restore the default sink at
  test end so cross-test state does not leak.
- **Commit:** `feat(logger): add slog wrapper with injectable stderr sink and -v verbosity`

## Batch Tests

`verify: go test ./internal/logger/...` runs the new package tests: default-Warn silence,
the `-v`/`-vv` thresholds, and the injectable-sink capture. No psmux or CLI surface here.
