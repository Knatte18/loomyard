All 4 commits match the batch's 4 cards' Commit messages exactly (Card 15, 16, 17, 18). 4 of 4 cards committed — all complete.

{"status":"success","commit_sha":"ec2fced872d43c76f5455e189cceae3dd30dfd04","session_id":"f0a31b0a-bc9a-408c-aa9b-305a493695c6","cards_done":[15,16,17,18]}

Summary: all 4 cards in the watch-loop batch (03-watch-loop.md) are committed and pushed to `reed-watchdog-daemon`, and the batch's verify command (`go test ./internal/reedengine/...`) passes cleanly.

Files touched:
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/reedengine/watchloop.go` (new) — `watchState`/`watchTiming`/`watchDefaultTiming` (pure debounce/retry state machine) plus `Engine.Watch`, `watchLoop`, and `handleWatchOutcome` (the driver).
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/reedengine/watchloop_test.go` (new) — tier-1 tests for the state machine against a synthetic clock, and driver tests against `TmuxCmd.execHook` plus a real `t.TempDir()` signal/lock file.

Notable implementation detail worth flagging for review: every promotion-based driver test accounts for the fact that the very first poll-mode tick (the one that discovers the hook and promotes) also performs the loop's first-ever real apply, since `lastApplied` starts at the zero `render.Box` and a live box never equals it — assertions compare against a captured baseline count rather than an absolute `select-layout`/`has(...)` check, to avoid a race where a test's "wait for select-layout" is satisfied by that earlier promotion-tick apply instead of the signal it's actually testing.

{"status":"success","commit_sha":"ec2fced872d43c76f5455e189cceae3dd30dfd04","session_id":"f0a31b0a-bc9a-408c-aa9b-305a493695c6","cards_done":[15,16,17,18]}
