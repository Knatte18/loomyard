Both cards in this batch (Card 34 and Card 35) were committed this turn — 2 of 2 cards complete. Verify (`go test ./internal/reedengine/...`) passed. Working tree is clean of tracked in-scope modifications.

Summary of changes:
- `/home/knatte/Code/loomyard/wts/trace-logging/internal/reedengine/lifecycle.go` — added `logger.Warn`/`logger.Debug` calls at every previously-unlogged error-return site identified by the plan across the boot path (`ensureServerAndSessionLocked`: stale socket-holder reap, logsDir MkdirAll/prune x3, tmux spawn, poll-loop has-session check, "up but session never materialized" branch, attempt/deadline exhaustion) and the header-pane/teardown path (`ensureHeaderPaneLocked`'s split-window, validateSplitCreatedNewPane, send-keys x2; `Down`'s kill-session and kill-server discards; the header-corpse kill-pane discard). Instrumentation only — no behavior change.

Commits:
- `029324d5` — Card 34: boot-path adoption
- `9de239ed` — Card 35: teardown-path adoption

{"status":"success","commit_sha":"9de239edc1ea8af187ba6cb8d7c640cc43576a46","session_id":"c697e1d4-a8bd-4da6-b3c3-70f9070568b1","cards_done":[34,35]}
