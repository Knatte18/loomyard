All 3 cards committed, matching the batch's declared 3 cards. All 3 of 3 cards committed and verified.

{"status":"success","commit_sha":"b97bd40d2973909cabc59c250f8153edd1fbf853","session_id":"372ad507-c0f4-4ecc-a6c4-50bb309ca662","cards_done":[1,2,3]}

Summary: 3 of 3 cards committed (all cards in batch 01-planparser-model-and-parse). Files touched:
- `/home/knatte/Code/loomyard/wts/plan-custom-card-skips-path-check/internal/planparser/plan.go` — added `TargetGroup` type, `Card.TargetGroups` field, rewrote godoc for `Card.Type`/`TypeLabelCount`/`Targets`/`Pairs`/`RenameRaw`.
- `/home/knatte/Code/loomyard/wts/plan-custom-card-skips-path-check/internal/planparser/parse.go` — `parseTypeLabelCase` now appends one `TargetGroup` per label occurrence; updated file-header/table/const-block comments to state the one-or-more grammar.
- `/home/knatte/Code/loomyard/wts/plan-custom-card-skips-path-check/internal/planparser/parse_test.go` — rewrote `TestParsePlan_Card_TypeLabelCount` into the multi-label happy-path test plus new sub-tests (single label, repeated label, two Rename labels).
- `/home/knatte/Code/loomyard/wts/plan-custom-card-skips-path-check/internal/planparser/normalize.go` — `normalizeCard` now also normalizes every `TargetGroups[i].Refs`/`Pairs` independently of the card-level pass.
- `/home/knatte/Code/loomyard/wts/plan-custom-card-skips-path-check/internal/planparser/normalize_test.go` — added post-condition test, Rename-group-pairs-root-joined test, extended nil/empty-slice test.

`go build ./...` and `go test ./internal/planparser/...` both pass. Working tree is clean.

{"status":"success","commit_sha":"b97bd40d2973909cabc59c250f8153edd1fbf853","session_id":"372ad507-c0f4-4ecc-a6c4-50bb309ca662","cards_done":[1,2,3]}
