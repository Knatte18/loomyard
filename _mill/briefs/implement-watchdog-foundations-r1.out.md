All 5 cards' Commit messages are present in the log, matching all 5 declared cards (1 through 5) exactly. 5 of 5 cards committed — all complete.

{"status":"success","commit_sha":"f32ccc27ffa44b9e5e961720505eb23cb9f1a8a0","session_id":"cdf284f2-ea90-46ad-b00d-71094317be7c","cards_done":[1,2,3,4,5]}

Summary of work: implemented all 5 cards of batch `watchdog-foundations` (5 of 5 cards committed, verify passing).

Files touched:
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/shell/shell.go` — added `Touch` to the `Shell` interface
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/shell/posix.go` — `posixShell.Touch`
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/shell/pwsh.go` — `pwshShell.Touch`
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/shell/shell_test.go` — Touch tests + compile-time `Shell` assertions
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/reedengine/config.go` — added `Watchdog` field
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/reedengine/template.go` — updated godoc count
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/reedengine/template_posix.yaml` / `template_windows.yaml` — added `watchdog:` key
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/reedengine/watchdog.go` — new file: `watchdogOption`, 5 loop constants, `resizeSignalFileName`, `windowResizedHookName`, `(*Engine).resizeSignalPath`, `tmuxQuoteValue`, `resizeHookCommand`
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/reedengine/watchdog_test.go` — new file covering all of the above

Verify command `go test ./internal/shell/... ./internal/reedengine/...` passes. HEAD is `f32ccc27ffa44b9e5e961720505eb23cb9f1a8a0`, all changes pushed to `reed-watchdog-daemon`.
