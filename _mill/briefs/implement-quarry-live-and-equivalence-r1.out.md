Clean. All checks pass.

Card-count self-check: this batch declares 5 cards (34-38). Cards 34, 35, 36, 37 were already committed in a prior round (verified via `git log`/`git show` on both the task worktree and the quarry worktree -- commits fd69d002/2c54254/5eaf45c/f0ed319). This turn I completed only card 38 -- the test-isolation race fix -- committed as quarry commit `f88e605d88b6d2a3bfc612d3860f8365480ce2ad` ("test(cli): isolate TestRunCLIIn_TargetDirResolvesAgainstInjectedSeamCwd from the userConfigDir seam race"), and verified `go -C /home/knatte/Code/quarry/wts/quarry test -tags lsp ./... -count=1` green 5 times in a row (with `gopls` on PATH; the earlier PATH-less failures were an environment/shell-PATH issue in this Bash session, not a code defect -- reproduced identically before and after the fix, unrelated to `internal/cli`). So: 5 of 5 cards now complete across this round and the prior one (this turn: 1 of 1 remaining card, card 38).

Relevant files: `/home/knatte/Code/quarry/wts/quarry/internal/cli/cli_test.go` (edited), `/home/knatte/Code/loomyard/wts/scout-extract-standalone-repo/_mill/plan/05-quarry-live-and-equivalence.md` (read only, already updated in prior round).

{"status":"success","commit_sha":"f88e605d88b6d2a3bfc612d3860f8365480ce2ad","session_id":"3af5617f-662c-43d3-a42e-d43af5e81b86","cards_done":[38]}
