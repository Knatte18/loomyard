I mis-copied the SHA in my previous line — here is the corrected one.

{"status":"success","commit_sha":"31b4b2a45cfbcf557aa6cbeaf727e2dcebab8264","session_id":"db8e2537-6a7e-4e30-ae8d-869c4e37cc63","cards_done":[17,18,19,20,21]}

Summary: all 5 cards of the `ensure-server-native` batch (17-21) are committed and pushed to `codeintel-v1` — 5 of 5 cards, no `Commit: none` cards in this batch.

Files touched:
- `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/codeintelengine/ensureserver.go` (new — `connKind`, `ensureServer` dispatcher, `finalizeConnection`, `rootURIFor`, `ensureNative`)
- `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/codeintelengine/ensureserver_test.go` (new — unit tests for `finalizeConnection`: success, initialize-error, probe-timeout)
- `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/codeintelengine/ensureserver_integration_test.go` (new — `//go:build integration` tests: `ensureNative` end-to-end, and the shared-daemon wire-compatibility regression pin)

Verify (`go test -count=1 ./internal/codeintelengine/...`) passes; `go vet ./...` is clean; no formatter drift; the integration test file compiles under `-tags integration` and skips cleanly on this gopls-less machine. Working tree is clean (no uncommitted tracked changes).
