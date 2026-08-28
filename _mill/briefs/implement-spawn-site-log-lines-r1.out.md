All 7 cards (cards 6-12) match the batch's declared 7 cards exactly, one commit each.

{"status":"success","commit_sha":"2172585cea2ffb21135388a87762cbd33a289f1b","session_id":"39429a70-600b-4af4-b7bd-57be329c9136","cards_done":[6,7,8,9,10,11,12]}

Summary: 7 of 7 cards committed. All cards (6 through 12) in `_mill/plan/03-spawn-site-log-lines.md` are implemented and committed:

- `internal/websterengine/integration.go` + new `internal/websterengine/runverify_test.go` (card 6)
- `internal/treadleengine/gate.go` (card 7)
- `internal/configengine/edit.go` (card 8)
- `internal/boardengine/spawn.go` (card 9)
- `internal/vscode/launch_linux.go`, `internal/vscode/launch_windows.go` (card 10)
- `internal/reedengine/proctree_windows.go` (card 11)
- `internal/fabricengine/spawn.go`, `internal/reedcli/attach.go`, `internal/loomcli/run.go` (card 12)

Both verify halves pass: `go test ./internal/websterengine/ ./internal/treadleengine/ ./internal/configengine/ ./internal/boardengine/ ./internal/vscode/ ./internal/reedengine/ ./internal/fabricengine/ ./internal/reedcli/ ./internal/loomcli/` and `go test -tags integration -run TestRunVerifyCommand ./internal/websterengine/`. `gofmt -l` reports no drift on the touched files, `GOOS=windows go build ./...` succeeds, and `git status --porcelain --untracked-files=no` shows no dirty tracked files (the only untracked item is the batch brief file itself, outside this batch's scope).

{"status":"success","commit_sha":"2172585cea2ffb21135388a87762cbd33a289f1b","session_id":"39429a70-600b-4af4-b7bd-57be329c9136","cards_done":[6,7,8,9,10,11,12]}
