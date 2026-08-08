41 chars including newline = 40-char SHA. Good, confirmed: `9c24722aa118fafeeb280d1b176d86ff2c849ed7`.

All 9 cards of batch `01-location-threading.md` are committed, the batch verify command (`go build ./... && go vet -tags scout ./internal/scoutengine/... && go test ./internal/scoutengine/... ./internal/scoutcli/... ./internal/lyxcwd/... ./cmd/lyx/...`) passes, and `git status --porcelain` is clean. 9 of 9 cards committed — all complete, verified by matching commit subjects against the batch's `## Cards` `Commit:` messages.

Key files touched:
- `/home/knatte/Code/loomyard/wts/scout-lyxcwd-accessors/internal/scoutengine/daemonstate.go`
- `/home/knatte/Code/loomyard/wts/scout-lyxcwd-accessors/internal/scoutengine/refs.go`
- `/home/knatte/Code/loomyard/wts/scout-lyxcwd-accessors/internal/scoutengine/ensureserver.go`
- `/home/knatte/Code/loomyard/wts/scout-lyxcwd-accessors/internal/scoutengine/doc.go`
- `/home/knatte/Code/loomyard/wts/scout-lyxcwd-accessors/internal/scoutcli/cli.go`
- `/home/knatte/Code/loomyard/wts/scout-lyxcwd-accessors/internal/scoutcli/cli_test.go`
- `/home/knatte/Code/loomyard/wts/scout-lyxcwd-accessors/internal/scoutengine/scoutdaemon_test.go`
- `/home/knatte/Code/loomyard/wts/scout-lyxcwd-accessors/internal/scoutengine/supervised_test.go`
- `/home/knatte/Code/loomyard/wts/scout-lyxcwd-accessors/internal/scoutengine/ensureserver_test.go`
- `/home/knatte/Code/loomyard/wts/scout-lyxcwd-accessors/cmd/lyx/constructoranchoring_test.go`
- `/home/knatte/Code/loomyard/wts/scout-lyxcwd-accessors/internal/scoutengine/supervised_scout_test.go`
- `/home/knatte/Code/loomyard/wts/scout-lyxcwd-accessors/internal/scoutengine/supervised_integration_test.go`
- `/home/knatte/Code/loomyard/wts/scout-lyxcwd-accessors/internal/scoutengine/refs_integration_test.go`
- `/home/knatte/Code/loomyard/wts/scout-lyxcwd-accessors/internal/scoutengine/ensureserver_integration_test.go`

{"status":"success","commit_sha":"9c24722aa118fafeeb280d1b176d86ff2c849ed7","session_id":"15b5b614-8b41-41c3-834a-0883f8b9c52b","cards_done":[1,2,3,4,5,6,7,8,9]}
