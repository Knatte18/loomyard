All 7 of 7 cards are committed, matching every `Commit:` message in the batch file's `## Cards` section. Verify (`go test ./internal/modelspec/...`) passed, and `go vet ./...` is clean.

Card-count self-check: 7 of 7 cards committed (verified against `git log 48b7783..HEAD --oneline`, matching every card's `Commit:` message).

Files touched (all absolute paths under `C:\Code\loomyard\wts\modelspec`):
- `internal/modelspec/modelspec.go`, `internal/modelspec/parse.go`, `internal/modelspec/parse_test.go`, `internal/modelspec/registry.go`, `internal/modelspec/registry_test.go`, `internal/modelspec/load.go`, `internal/modelspec/load_test.go`, `internal/modelspec/template.go`, `internal/modelspec/template.yaml`, `internal/modelspec/template_test.go`, `internal/modelspec/leaf_enforcement_test.go`
- `CONSTRAINTS.md`, `docs/reference/model-spec.md`, `docs/overview.md`, `docs/shared-libs/README.md`

{"status":"success","commit_sha":"4e1d54e","session_id":"f88e6c14-995c-4e00-8d8d-2f657f822bf4"}
