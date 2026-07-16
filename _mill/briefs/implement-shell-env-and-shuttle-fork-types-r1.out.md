{"status":"success","commit_sha":"d6ccf0821c6c2beb23da7067e898064e82d60935","session_id":"38e9a129-a7db-466d-8fb5-7d85ce95c16c"}

Summary: 2 of 2 cards committed for batch `01-shell-env-shuttle-fork-types.md`.

- Card 1 (commit `75f805dc`): Added `WithEnv(key, value, cmd string) string` to the `Shell` interface and both implementations.
  - `/home/knatte/Code/loomyard/wts/burler-fork-cluster/internal/shell/shell.go`
  - `/home/knatte/Code/loomyard/wts/burler-fork-cluster/internal/shell/posix.go`
  - `/home/knatte/Code/loomyard/wts/burler-fork-cluster/internal/shell/pwsh.go`
  - `/home/knatte/Code/loomyard/wts/burler-fork-cluster/internal/shell/shell_test.go`
- Card 2 (commit `d6ccf082`): Added `Spec.ForkSubagents bool`, the `ForkAudit`/`ForkReport` value types, and `Result.ForkAudit *ForkAudit`.
  - `/home/knatte/Code/loomyard/wts/burler-fork-cluster/internal/shuttleengine/spec.go`
  - `/home/knatte/Code/loomyard/wts/burler-fork-cluster/internal/shuttleengine/run.go`
  - `/home/knatte/Code/loomyard/wts/burler-fork-cluster/internal/shuttleengine/forkaudit.go` (new)

Verify command `go test ./internal/shell/ ./internal/shuttleengine/` passes; full `go build ./...` also passes (confirms no other `Shell` implementers broke). Both commits pushed to `burler-fork-cluster`. Working tree is clean of tracked in-scope modifications.

{"status":"success","commit_sha":"d6ccf0821c6c2beb23da7067e898064e82d60935","session_id":"38e9a129-a7db-466d-8fb5-7d85ce95c16c"}
