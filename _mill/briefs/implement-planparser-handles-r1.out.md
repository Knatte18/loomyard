All 7 of 7 cards (9 through 15) committed this turn, batch verify (`go test ./internal/planparser/ ./cmd/lyx/`) passes, and `go build ./...` for the module also passes. No uncommitted tracked changes remain.

Relevant files touched:
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/handle.go` (new)
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/handle_test.go` (new)
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/plan.go`
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/parse.go`
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/parse_test.go`
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/validate.go`
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/validate_test.go`
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/testdata/goodplan/05-rowmapper-rename.md`
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/rewrite.go` (new)
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/rewrite_test.go` (new)
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/amendment.go` (new)
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/amendment_test.go` (new)
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/containment.go` (new)
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/containment_test.go` (new)
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/doc.go`
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/CONSTRAINTS.md`
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/cmd/lyx/constraintchokepoint_test.go` (new)
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/_mill/plan/03-planparser-handles.md` (plan-edit: added the golden fixture to card 11's Edits list, per the discovery protocol, committed separately as `bda67fb9d`)

One notable mid-batch finding: card 11's new `rename-to-not-handle` check tripped the pre-existing golden fixture (`testdata/goodplan/05-rowmapper-rename.md`), which predated the handle grammar. Per the brief's discovery protocol I stopped, added that fixture file to card 11's Edits list in the plan file, committed that plan edit first, then respelled the fixture's Rename pair's new side as `plan:internal/boardengine#MapRowJSON` and updated the corresponding `parse_test.go` assertions.

{"status":"success","commit_sha":"12cb12eedec1830f2274041f60b9b89272063848","session_id":"31181b28-54b0-4c76-8248-7b1319f09b66","cards_done":[9,10,11,12,13,14,15]}
