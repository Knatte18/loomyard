Working tree clean, 1 commit since batch start matching card 5's Commit message. Card-count self-check: 1 of 1 cards committed — all complete.

{"status":"success","commit_sha":"48ff357ce8fccbbec03babfa639e6f10d13f952f","session_id":"c402f1a0-f6fa-460b-85e4-0322e2473129","cards_done":[5]}

Summary: 1 of 1 cards committed for batch attach-preflight. Modified `internal/reedcli/attach.go` (path: `/home/knatte/Code/loomyard/wts/reed-cold-worktree-selfheal/internal/reedcli/attach.go`) to call `c.eng.EnsureSession()` immediately before the existing `Status()` pre-flight call, logging once at Info on the boot path (naming attach, socket, session) and nothing on the warm path, with the ordering rationale documented inline. Rewrote the file header comment and the subcommand's long help text to describe the two-step pre-flight and the self-heal behavior. Verify (`go test ./internal/reedcli/ ./internal/reedengine/`) passed. Working tree is clean; commit `48ff357ce` is pushed to `origin/reed-cold-worktree-selfheal`.

{"status":"success","commit_sha":"48ff357ce8fccbbec03babfa639e6f10d13f952f","session_id":"c402f1a0-f6fa-460b-85e4-0322e2473129","cards_done":[5]}
