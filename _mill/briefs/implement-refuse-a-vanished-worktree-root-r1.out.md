Card-count self-check: 3 of 3 cards committed (matches all `## Cards` entries: Card 1, Card 2, Card 3), each with its own commit matching the declared `Commit:` message. Working tree is clean with no uncommitted tracked changes. Verify passed (`go test ./internal/reedengine/...` and `go vet -tags integration ./internal/reedengine/...` both green), and the by-hand `go test -tags integration` run showed exactly the one documented pre-existing failure, `TestWatchdogSelfHeal_HookProbeMatchesLiveTmux`, with no second regression.

Relevant files touched:
- `/home/knatte/Code/loomyard/wts/reed-lock-stale-session-name/internal/reedengine/server.go`
- `/home/knatte/Code/loomyard/wts/reed-lock-stale-session-name/internal/reedengine/server_test.go`
- `/home/knatte/Code/loomyard/wts/reed-lock-stale-session-name/internal/reedengine/lock.go`
- `/home/knatte/Code/loomyard/wts/reed-lock-stale-session-name/internal/reedengine/lock_test.go`
- `/home/knatte/Code/loomyard/wts/reed-lock-stale-session-name/internal/reedengine/doc.go`
- `/home/knatte/Code/loomyard/wts/reed-lock-stale-session-name/tools/sandbox/SANDBOX-REED-SUITE.md`

{"status":"success","commit_sha":"a4a58b573b7083fafbbfab497c559977d9a970a7","session_id":"d3348678-9e17-409d-8827-4a66e7fd7cd7","cards_done":[1,2,3]}
