{"status":"success","commit_sha":"719d14b1f7ab8b2a82b3dafc023f302d6a98b548","session_id":"18291cf5-4ee0-4e1e-b607-8c162c5a9a9e","cards_done":[16,17,18,19,20,21,22]}

All 7 of 7 cards committed and verified. Summary of work:

- Created `internal/planglyph` package (new): `doc.go`, `repo.go`, `planglyph.go`, `resolve.go`, `create.go`, `handle.go`, `containment.go`, plus matching `_test.go` files and `testmain_test.go`.
- Exported `internal/planparser/handle.go`'s `HandleUnit` (new accessor) so `planglyph` doesn't reimplement handle-splitting.
- Ran `go mod tidy` to add the tree-sitter `go.sum` entries transitively needed once `quarry/quarry` (not just the pure `glyph` package) is imported for the first time — updated `go.mod`/`go.sum`.
- Updated `CONSTRAINTS.md` (Told-Geometry Invariant bound-packages bullet), `docs/overview.md` (module table entry), and rewrote `manifest/designs/quarry-glyph-plan-alphabet.md` to describe the shipped design.
- Made three plan-file edits (each committed and pushed separately, per the brief's rule 2) to bring `internal/planparser/handle.go`, `go.mod`/`go.sum`, and `internal/planglyph/doc.go` into the appropriate cards' declared `Edits:` scope before touching them.

Batch verify (`go test ./internal/planglyph/ ./internal/planparser/ ./internal/lyxcwd/`) and `go build ./...` both pass. Working tree is clean; all commits pushed to `quarry-glyph-plan-alphabet`.

{"status":"success","commit_sha":"719d14b1f7ab8b2a82b3dafc023f302d6a98b548","session_id":"18291cf5-4ee0-4e1e-b607-8c162c5a9a9e","cards_done":[16,17,18,19,20,21,22]}
