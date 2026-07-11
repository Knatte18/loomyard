All 5 of 5 cards in the config-roles batch are committed, matching the batch file's Commit messages exactly (Cards 7-11). The batch verify command (`go test ./internal/builderengine/... ./internal/configreg/...`) passes. Working tree is clean with no uncommitted tracked changes.

Summary of files touched:
- `internal/builderengine/config.go` (new) — `Config` struct + `LoadConfig` with modelspec.Parse role validation
- `internal/builderengine/config_test.go` (new)
- `internal/builderengine/template.go` (new)
- `internal/builderengine/template.yaml` (new)
- `internal/builderengine/template_test.go` (new)
- `internal/builderengine/roles.go` (new) — `Role` enum + `ResolveRoles`
- `internal/builderengine/roles_test.go` (new)
- `internal/configreg/configreg.go` (edited) — registered `builder` module
- `internal/configreg/configreg_test.go` (edited) — pinned `want` list updated
- `docs/reference/model-spec.md` (edited) — `fixer` -> `recovery`
- `docs/modules/plan-format.md` (edited) — `fixer` -> `recovery`

Note: `gofmt -l` flags several pre-existing files (including `configreg.go`/`configreg_test.go` after my edit) due to CRLF line endings from this repo's `core.autocrlf=true` on Windows -- confirmed this is pre-existing (untouched files like `internal/perchengine/config.go` show the same) and not something introduced by this batch's edits; content is correct and tests pass.

{"status":"success","commit_sha":"2919099","session_id":"13d0a84a-6dbe-48ee-b98c-781ed62e052e"}
