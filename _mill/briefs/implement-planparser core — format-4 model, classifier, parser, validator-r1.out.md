All 7 cards committed, matching all 7 declared card commit messages exactly (7 of 7 cards committed). `go build ./...` verify passed. Working tree is clean.

{"status":"success","commit_sha":"9ea9289b636676e557c99664cfb853436045c0c4","session_id":"b29f03b4-49fb-4ec6-ba99-3b8ed5b6ab3c","cards_done":[1,2,3,4,5,6,7]}

Summary: 7 of 7 cards committed and verified.

Files touched (all under `/home/knatte/Code/loomyard/wts/planparser-card-format-migration`):
- `internal/planparser/classify.go` (new) — shape classifier
- `internal/planparser/classify_test.go` (new) — table tests for the classifier
- `internal/planparser/plan.go` — format-4 `Card`/`CardType` model
- `internal/planparser/parse.go` — format-4 card-body parser
- `internal/planparser/normalize.go` — classifier-gated normalization
- `internal/planparser/validate.go` — sixteen-check `Validate`
- `internal/planparser/doc.go` — rewritten package doc
- `internal/websterengine/render.go` — `c.Intent` → `c.Summary`

`go build ./...` passes at HEAD (`9ea9289b636676e557c99664cfb853436045c0c4`). As documented in the batch's own "Batch Tests" section, `internal/planparser`'s and consumers' `*_test.go` files still reference the old format-3 shape and won't compile until batches 2 and 3 land — that's expected and by design for this batch.

{"status":"success","commit_sha":"9ea9289b636676e557c99664cfb853436045c0c4","session_id":"b29f03b4-49fb-4ec6-ba99-3b8ed5b6ab3c","cards_done":[1,2,3,4,5,6,7]}