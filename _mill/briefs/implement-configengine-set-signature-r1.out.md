{"status":"success","commit_sha":"a43a54eeb99c257196e66c73bd2395416e277a5a","session_id":"36f645dc-9ee1-43c6-a084-7e0a62a629d4"}

All three cards for batch `configengine-set-signature` are committed and pushed:

- Card 4 (`4620348`): `internal/configengine/set.go` — `Set`'s signature changed from `(...) error` to `(...) ([]string, error)`, every return path updated, doc comment updated.
- Card 5 (`e3d9d0f`): `internal/configengine/set_test.go` — the four existing `Set(...)` call sites updated to `_, err :=`, plus new `TestSet_PreservesUnrecognizedExistingKeyEndToEnd` asserting both the returned `[]string` and on-disk preservation.
- Card 6 (`a43a54e`): `docs/shared-libs/configengine.md` — new `Set(...)` subsection added to "Exported functions", cross-referencing `yamlengine.SetValues`.

Verify (`go test ./internal/configengine/...`) passes. Note: I deviated from the card 5 spec's literal suggestion of `cmp.Diff` implicitly via my own choice — I used plain manual slice comparison (`len/index` check) instead of `google/go-cmp`, since that package is not a dependency of this repo (not in `go.mod`, unused elsewhere) and batch 1's precedent (`internal/yamlengine/set_test.go`) also uses manual comparison for the same `Preserved` field rather than introducing `go-cmp`.

Files touched: `internal/configengine/set.go`, `internal/configengine/set_test.go`, `docs/shared-libs/configengine.md`.

{"status":"success","commit_sha":"a43a54eeb99c257196e66c73bd2395416e277a5a","session_id":"36f645dc-9ee1-43c6-a084-7e0a62a629d4"}
