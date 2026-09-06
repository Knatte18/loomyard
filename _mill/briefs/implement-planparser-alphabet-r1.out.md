All 6 cards (3, 4, 5, 6, 7, 8) declared in this batch are committed — cards 3-7 in one combined commit (`fabb1cb11`, since they share `validate.go`/`parse.go`/`plan.go`/`normalize.go` too tightly to split) and card 8 in a second commit (`ecfd73e11`). The batch's `verify:` (both the plain `go test` half and the `-tags integration` half) passed, `go build ./...` passed, and `git status --porcelain --untracked-files=no` is clean.

Relevant files:
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/classify.go`
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/glyphref.go` (new)
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/normalize.go`
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/parse.go`
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/plan.go`
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/validate.go`
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/doc.go`
- corresponding `_test.go` files plus new `glyphref_test.go`
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/internal/planparser/testdata/goodplan/*.md`
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/contracts/specs/loom-plan-spec.md`
- `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet/contracts/stencils/loom/loom-template-plan.md`
- format-4→5 sweep in `internal/loomcli/validate_test.go`, `internal/loomshed/{planvalidate_test.go,gatefindings_test.go}`, `internal/webstercli/cli_test.go`, `internal/websterengine/runlevel_test.go`, `internal/loomrecipe/fixture_test.go`, `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`

{"status":"success","commit_sha":"ecfd73e119ae166b7948ac4cfde7bcece66935d1","session_id":"bdc01873-2594-490b-8a4c-33916338348e","cards_done":[3,4,5,6,7,8]}
