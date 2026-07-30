All 3 of 3 cards declared in this batch are committed: card 4 (`c7bc4e26`), card 5 (`2e1d9acc`), and card 6 (`c79fe060`). The batch's verify command (`go test -tags integration ./internal/fabricengine/ ./internal/fabriccli/`) passes cleanly, and `git status --porcelain --untracked-files=no` shows no tracked in-scope modifications outstanding.

Summary of files touched:
- `/home/knatte/Code/loomyard/wts/fabric-commit-api/internal/fabricengine/spawn.go` (new) — `SpawnDetachedPush(warpPath, weftPath string) error` and `PushWarpAt(warpPath string, opts SyncOptions) error`
- `/home/knatte/Code/loomyard/wts/fabric-commit-api/internal/fabricengine/spawn_test.go` (new) — Tier-1 gating tests
- `/home/knatte/Code/loomyard/wts/fabric-commit-api/internal/fabriccli/weft_verbs.go` — added `--warp-path` hidden persistent flag, bypass-mode gating on either flag, push subcommand now pushes each supplied side via `fabricengine.PushWarpAt`/`PushWeftAt`
- `/home/knatte/Code/loomyard/wts/fabric-commit-api/internal/fabriccli/spawn.go` — `spawnPush` reduced to `return fabricengine.SpawnDetachedPush("", weftPath)`
- `/home/knatte/Code/loomyard/wts/fabric-commit-api/internal/fabriccli/pushbypass_integration_test.go` (new) — integration test proving both-sides bypass push advances both bare upstreams, plus a `--warp-path`-only non-push-verb rejection case

4 of 4 total cards (across the whole batch, numbered 4-6, i.e. 3 cards) committed — all cards complete, none skipped or deferred.

{"status":"success","commit_sha":"c79fe060","session_id":"39eeec7b-b35f-4740-a4d0-44b3042d511b","cards_done":[4,5,6]}