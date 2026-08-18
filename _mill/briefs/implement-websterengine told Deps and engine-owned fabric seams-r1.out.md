All 12 declared cards (22 through 33) are committed and verified. Final verification summary:

- `go test ./internal/websterengine/... ./internal/webstercli/... ./cmd/lyx/...` — pass
- `go test -tags integration ./internal/websterengine/... ./internal/webstercli/...` — pass
- `go build ./...` and `go vet -tags integration ./...` — clean (this caught an out-of-scope `internal/shedadapters` regression, which I fixed after extending the plan file per protocol)
- `git status --porcelain --untracked-files=no` — clean, no dirty tracked files

Key files touched (all under `/home/knatte/Code/loomyard/wts/webster-told-geometry`):
- `internal/websterengine/{audit,render,beginbatch,recordbatch,recoverbatch,runlevel,doc}.go` — engine signature changes
- `internal/websterengine/{template,audit,beginbatch,recordbatch,recoverbatch,runlevel,integration}_test.go` — fixture conversions and new pinning tests
- `internal/webstercli/{cli,run,beginbatch,recordbatch,recoverbatch,cli_test,verbs_test}.go` — hub-mode wiring
- `internal/shedadapters/webster.go` and `webster_test.go` — out-of-scope fix (plan-extended)
- `_mill/plan/07-webster-told-deps.md`, `_mill/plan/00-overview.md` — two plan-scope-extension commits

4 of 12 cards' commits (27+28) were combined into one commit per the allowed "cards that necessarily touch the same file" rule, since `RunDeps.Layout` removal and the `fabricengine.Open` fallback deletion could not be split without leaving an intermediate non-compiling state.

{"status":"success","commit_sha":"b34aa504574f1bd3660e58ebf6ef0cb807851cdd","session_id":"2111c602-f15d-49f3-91e9-f28946797489","cards_done":[22,23,24,25,26,27,28,29,30,31,32,33]}
