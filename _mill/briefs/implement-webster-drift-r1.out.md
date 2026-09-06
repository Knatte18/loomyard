9 of 9 cards committed (cards 31 through 39, all with real Commit: messages — none had Commit: none). All verify commands passed: the untagged `go test ./internal/planglyph/ ./internal/websterengine/ ./internal/webstercli/ ./cmd/lyx/ ./internal/lyxcwd/` and the integration-tagged `go test -tags integration ./internal/planglyph/ ./internal/websterengine/`, plus `go build ./...`. One mid-batch plan edit was made and committed separately (extending card 38's Edits list for `internal/planglyph/testmain_test.go`, whose doc comment needed rewording to stop tripping the new tier-purity token) before touching that file, per protocol.

Key files touched (all absolute paths under `/home/knatte/Code/loomyard/wts/quarry-glyph-plan-alphabet`):
- `internal/planglyph/delta.go`, `delta_integration_test.go` (card 31)
- `internal/websterengine/beginbatch.go`, `beginbatch_test.go`, `internal/webstercli/beginbatch.go` (card 32)
- `internal/planglyph/donecheck.go`, `donecheck_integration_test.go`, `internal/websterengine/recordbatch.go`, `recordbatch_test.go`, `internal/webstercli/recordbatch.go` (card 33)
- `internal/planglyph/handle.go`, `handle_test.go` (card 34, plus recordbatch wiring above)
- `internal/planglyph/scope.go`, `scope_test.go` (card 35)
- `internal/planglyph/drift.go`, `drift_integration_test.go` (cards 36–37)
- `cmd/lyx/tierpurity_test.go`, `cmd/lyx/spawnobservability_test.go`, `internal/planglyph/testmain_test.go` (card 38)
- `contracts/stencils/webster/webster-body-implementer.md`, `contracts/stencils/loom/loom-rubric-plan-review.md`, `manifest/roadmap.md` (card 39)
- `_mill/plan/07-webster-drift.md` (plan-edit commit for card 38's scope extension)

One deliberate deviation from the batch prose worth flagging: `DetectDrift`'s signature carries an extra `worktreeRoot string` parameter beyond what the batch text listed, because the required "revalidate with one batched resolve" step needs a root to open `quarry.Repo` against, and `planglyph` never derives one of its own (told-geometry decision). This is documented in `drift.go`'s own doc comment.

{"status":"success","commit_sha":"e426c1e37b7fa9f5b2d34721e12fa8c16ff3ebdf","session_id":"aa3daf0d-5337-4c0d-b3f3-5de879946342","cards_done":[31,32,33,34,35,36,37,38,39]}
