All 5 of 5 cards for batch `cli-surface-and-docs` were completed this turn (13, 14, 15 combined into 2 commits since 14+15 necessarily touch the same functions in the same file; 16 and 17 each in their own commit). Both verify sub-invocations passed: `go test ./internal/fabriccli/... ./cmd/lyx/...` and `go test -tags integration ./internal/fabriccli/...`.

Relevant files:
- /home/knatte/Code/loomyard/wts/fabric-remote-branch-delete/internal/fabriccli/fabric.go
- /home/knatte/Code/loomyard/wts/fabric-remote-branch-delete/internal/fabriccli/remoteenvelope_integration_test.go
- /home/knatte/Code/loomyard/wts/fabric-remote-branch-delete/manifest/roadmap.md

{"status":"success","commit_sha":"1466775f12568a1ece296c14b770e1126c2d5c6b","session_id":"27f6029e-6bdb-4f1e-8b30-2ffe16ba45e0","cards_done":[13,14,15,16,17]}
