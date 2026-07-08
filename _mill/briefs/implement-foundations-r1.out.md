{"status":"success","commit_sha":"ee7e603e566f001ad612d06e60175cdcf28ff3a6","session_id":"45dee6e0-5c52-4d9d-b371-32b344110835"}

All three cards for batch 01-foundations are implemented, committed, and pushed:

1. `internal/burlerengine/engine.go`, `verdict.go`, `doc.go`, `engine_test.go` — added `Result.RunDir` passthrough and reworded the stale pre-verdict-judge-amendment comments (commit `24264bb`).
2. `internal/hubgeometry/hubgeometry.go`, `hubgeometry_unit_test.go` — added `PerchRunsDir(baseDir)` accessor plus unit test (commit `db388ef`).
3. `internal/perchengine/config.go`, `template.go`, `template.yaml`, `config_test.go`, `doc.go` (new package) and `internal/configreg/configreg.go`, `configreg_test.go` (registration) (commit `ee7e603`).

Batch verify command `go test ./internal/burlerengine/ ./internal/hubgeometry/ ./internal/perchengine/ ./internal/configreg/` passes. Also spot-checked `go build ./...`, `go vet ./...`, and the neighboring `configcli`/`configsync`/`cmd/lyx` suites, all green. Working tree is clean with no uncommitted tracked changes.

{"status":"success","commit_sha":"ee7e603e566f001ad612d06e60175cdcf28ff3a6","session_id":"45dee6e0-5c52-4d9d-b371-32b344110835"}
