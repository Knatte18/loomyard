No tracked modifications remain. All three cards committed and pushed, verify is fully green (build + Tier 1 + Tier 2).

{"status":"success","commit_sha":"4a27fb3a5c7e3855743a4f135c988567b6a8f433","session_id":"96c70934-52da-4684-b208-20c925d7b315"}

Batch "ghissues split" complete. Summary:

- **Card 15** (`dc9f478`): Created `internal/ghissuesengine/ghissues.go` (package `ghissuesengine`) — moved `realRunGH`, `buildCreateArgs`, `lastNonEmptyLine`; renamed `createIssue` → exported `CreateIssue` and `var runGH` → exported settable `var RunGH`; dropped the `stdin`/`io`/`os` seam.
- **Card 16** (`6f04836`): `git mv` of `cli.go` + `cli_test.go` into `internal/ghissuescli/` (package `ghissuescli`); added the `var stdin io.Reader = os.Stdin` seam here, imported `ghissuesengine`, routed `runCreate` through `ghissuesengine.CreateIssue`, and swapped the exported `ghissuesengine.RunGH` seam in tests. All nine scenarios preserved, untagged.
- **Card 17** (`4a27fb3`): Retargeted `cmd/lyx/main.go` to import `ghissuescli` and register `ghissuescli.Command()`; deleted the remaining `internal/ghissues` package.

`go build ./... && go test ./... && go test -tags integration ./...` all green.

{"status":"success","commit_sha":"4a27fb3a5c7e3855743a4f135c988567b6a8f433","session_id":"96c70934-52da-4684-b208-20c925d7b315"}