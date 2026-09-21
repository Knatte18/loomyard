All 5 cards in batch 02-reed-pane-binary-chokepoint completed and committed (5 of 5 — 100%), verified against the batch's `verify:` command in both tiers.

Commits (098a725ca..HEAD):
- fc78ffb1a feat(reed): resolve lyx to the spawning binary inside every strand pane (Card 3)
- e3cfd9adb test(reed): cover the pane-binary prelude composition hermetically (Card 4)
- 4fe813cd4 test(reed): guard the prelude payload and the unchanged split-window argv (Card 5)
- 2a8daa2b8 test(reed): enforce that every pane-creation site routes through the prelude chokepoint (Card 6)
- 22e34d74d test(reed): repoint the empty-cmd integration test at the prelude-only payload (Card 7)

Key files:
- /home/hanf/Code/loomyard/wts/lyx-bin-pane-path/internal/reedengine/panebin.go (new — owns the prelude seam: `paneBinPrelude`, `composePaneLaunchLine`, `executablePath` seam, `lyxBinEnvKey`)
- /home/hanf/Code/loomyard/wts/lyx-bin-pane-path/internal/reedengine/spawn.go (launchStrandLocked now composes the prelude via `composePaneLaunchLine(shell.ForGOOS(), launchCmd, s.GUID)` before send-keys; split-window argv unchanged)
- /home/hanf/Code/loomyard/wts/lyx-bin-pane-path/internal/reedengine/doc.go (new paragraph documenting the mechanism)
- /home/hanf/Code/loomyard/wts/lyx-bin-pane-path/CONSTRAINTS.md (new "## Pane Binary Resolution" section)
- /home/hanf/Code/loomyard/wts/lyx-bin-pane-path/internal/reedengine/panebin_test.go (new — hermetic composition tests)
- /home/hanf/Code/loomyard/wts/lyx-bin-pane-path/internal/reedengine/spawn_test.go (two new regression-guard tests appended)
- /home/hanf/Code/loomyard/wts/lyx-bin-pane-path/internal/reedengine/panebin_enforcement_test.go (new — AST chokepoint enforcement)
- /home/hanf/Code/loomyard/wts/lyx-bin-pane-path/internal/reedengine/emptycmd_integration_test.go (doc comments repointed at the prelude-only payload; test body/fixture unchanged)

Verify: `go test ./internal/reedengine/` (~1.4s) and `go test -tags integration ./internal/reedengine/` (~13s, ran against real tmux) both pass. Working tree clean, all changes pushed to `lyx-bin-pane-path`.

{"status":"success","commit_sha":"22e34d74d34f8492b0928cf265e42b8ee774418c","session_id":"f9076cf7-795f-4abb-90b4-7d8092d15737","cards_done":[3,4,5,6,7]}
