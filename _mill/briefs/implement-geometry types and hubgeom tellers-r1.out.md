41 chars including the newline = 40-char SHA. Good, `629f8c6699ed5653de449f6e1d3f8a4afea0efd1` is correct (verified: 40 hex chars + newline).

All 5 of 5 cards were committed and verified this turn:

- `internal/burlerengine/geometry.go` — new `Geometry{WorktreeRoot, AnchorPath}` type (Card 1)
- `internal/perchengine/geometry.go` — new `Geometry{GateDir, AnchorPath}` type (Card 2)
- `internal/hubgeom/hubgeom.go` — added `BurlerGeometry`/`PerchGeometry` tellers, rewrote file header (Card 3)
- `internal/hubgeom/hubgeom_test.go` — added `TestBurlerGeometry`/`TestPerchGeometry` against the hostile fixture (Card 4)
- `internal/hubgeom/doc.go` — updated package doc to name the three shipped tellers (Card 5)

`go test ./internal/hubgeom/... ./internal/burlerengine/... ./internal/perchengine/...` passes, and `go build ./...` passes across the whole module. Working tree is clean.

{"status":"success","commit_sha":"629f8c6699ed5653de449f6e1d3f8a4afea0efd1","session_id":"c16941e8-061b-4303-bfa9-2c254a4e35e4","cards_done":[1,2,3,4,5]}
