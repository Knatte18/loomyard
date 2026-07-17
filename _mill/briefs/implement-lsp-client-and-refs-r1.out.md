{"status":"success","commit_sha":"37247743","session_id":"f7bc336d-747c-4d8f-b6ee-c23307f71ddd"}

5 of 5 cards committed (Card 8 position conversion, Card 9 stdio LSP client, Card 10 framing tests, Card 11 References orchestration, Card 12 live gopls integration test). Batch verify (`go test ./internal/codeintelengine/...`) passes uncached; `go build ./...`, `go vet ./...`, and `golangci-lint run` clean (both untagged and `-tags integration`).

Files:
- internal/codeintelengine/position.go, position_test.go (Card 8)
- internal/codeintelengine/lspclient.go (Card 9, fixed in Card 10/12)
- internal/codeintelengine/lspclient_test.go (Card 10)
- internal/codeintelengine/refs.go (Card 11)
- internal/codeintelengine/refs_integration_test.go (Card 12)

Notable fix: per-call background reader goroutine never terminated on success, racing a leftover goroutine on the shared bufio.Reader and corrupting LSP framing. Caught by the live gopls integration test. Fixed with a single persistent readLoop goroutine feeding all calls via one incoming channel; confirmed under -race.
