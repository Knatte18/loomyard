{"status":"success","commit_sha":"542016dd9c922473255c5e493d8b9910449bac32","session_id":"58188492-84a0-46a4-8ded-60b6b69c001e","cards_done":[17,18,19,20,21,22]}

All 6 of 6 cards in this batch (17-22) are committed. Card-count self-check: 2 content commits made since the batch-start commit `e31f7803`, covering all 6 declared cards — commit `da9e17d8` combines cards 17/18/19/20 (the composite `dualHandler`/`durableHandler`, trace stamping, `SetOutput` stderr-only rebind, and the shared `sinkMu` guarding, all structurally inseparable in one coherent diff to `internal/logger/logger.go`), and commit `542016dd` combines cards 21/22 (dual-handler fan-out tests and the `-race` concurrency test, both in `internal/logger/logger_test.go`). Batch verify `go test -race ./internal/logger/...` passes. Working tree is clean (no uncommitted tracked changes).

Key files touched:
- `/home/knatte/Code/loomyard/wts/trace-logging/internal/logger/logger.go` — new `dualHandler`/`durableHandler`/`durableWriter` composite `slog.Handler`, `stderrHandlerSnapshot`, rewritten `Debug`/`Info`/`Warn` (trace stamping via `log.With("trace", TraceID())`), rewritten `SetOutput` (stderr-half-only rebind under `sinkMu`).
- `/home/knatte/Code/loomyard/wts/trace-logging/internal/logger/logger_test.go` — new dual-handler fan-out tests and the `-race` concurrency test for the sink write path.

{"status":"success","commit_sha":"542016dd9c922473255c5e493d8b9910449bac32","session_id":"58188492-84a0-46a4-8ded-60b6b69c001e","cards_done":[17,18,19,20,21,22]}
