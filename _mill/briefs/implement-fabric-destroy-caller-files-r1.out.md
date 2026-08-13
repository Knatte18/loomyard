5 of 5 cards committed (Cards 18, 19, 20, 21, 22 — all present in the log). Full completion confirmed.

All 5 cards in the fabric-destroy-caller-files batch are committed and verified:

- Card 18: `internal/fabricengine/weftwiring.go` — marked `weftRepoExists`/`weftBranchExists` as `//gitexec:raw` (commit `a54d4c15`)
- Card 19: `internal/fabricengine/weftwiring.go` — migrated `createWeftWorktree`, `pushWeftBranch`, prune site (commit `ef92bac7`)
- Card 20: `internal/fabricengine/add.go` — migrated all 6 sites in `Add`/`rollbackAdd`, including the mixed-probe and compound-guard cases (commit `217c491e`)
- Card 21: `internal/fabricengine/checkout.go` — migrated all 8 sites in `Checkout`/`switchOrForkWeft`/`rollbackSwitch` (commit `13e059d6`)
- Card 22: `internal/fabricengine/cleanup.go`, `internal/fabricengine/prune.go`, `internal/fabricengine/remove.go` — migrated `listWeftBranches` and the three best-effort prune discards (commit `ca38dfd5`)

Verify passed: `go build ./...`, `go test ./internal/fabricengine/...` (ok, 0.048s), `go test -tags integration ./internal/fabricengine/...` (ok, 15.5s). Working tree is clean.

{"status":"success","commit_sha":"ca38dfd5a550716aacbb8cb20cc7fea32db53def","session_id":"40208072-5d92-42e2-b18c-a9a1cfbd8bfa","cards_done":[18,19,20,21,22]}
