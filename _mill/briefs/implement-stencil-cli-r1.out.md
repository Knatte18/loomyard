Verify passes cleanly.

Summary: All 5 cards of batch 08-stencil-cli committed (5 of 5 cards complete):
- `internal/stencilcli/cli.go` — `list`, `validate`, `sync` subcommands, geometry resolution
- `internal/stencilcli/diff.go` — `diff` with `--all`/`--exit-code`, unified-diff renderer
- `internal/stencilcli/promote.go` — `promote`, stamp-line stripping
- `cmd/lyx/main.go`, `cmd/lyx/helptree_test.go`, `cmd/lyx/seamsignature_test.go` — module registration
- `tools/sandbox/SANDBOX-CORE-SUITE.md` — S7 coverage scenario
- `internal/stencilcli/testmain_test.go`, `internal/stencilcli/cli_integration_test.go` — Tier 1/2 test coverage

Batch verify (`go build ./... && go test ./internal/stencilcli/... ./cmd/lyx/...`) passes, plus the integration-tagged suite (`go test -tags integration ./internal/stencilcli/...`) passes. `gofmt`/`golangci-lint` clean on touched files (one pre-existing unrelated lint finding in `cmd/lyx/drift_test.go`, not touched by this batch). No uncommitted tracked changes remain.

{"status":"success","commit_sha":"a2c2becb05212ad8cfe34ffa9f20d1983c1f99b7","session_id":"57903289-edc2-4e4f-a3d4-41d0aa729964","cards_done":[32,33,34,35,36]}
