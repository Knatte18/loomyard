All 10 cards in the batch are complete. Summary of work:

**Cards 34–43, all committed:**
- Card 34: Added `--stencils-dir`, `--plan-dir`, `--target-dir` persistent flags (`internal/webstercli/cli.go`)
- Card 35: Extracted `wire`/`wireHub`/`wireStandalone` into `internal/webstercli/wiring.go`, shrinking `PersistentPreRunE`
- Cards 36+37 (combined commit, necessarily touching the same files): deleted `websterCLI.layout` and the redundant path fields in favor of `c.geom.*`; changed `fabricSync`'s signature to `(open func() (*fabricengine.Fabric, error), anchorRel, label string) (bool, error)`
- Cards 38+39 (folded into card 35's wiring.go work): standalone stencil seeding on first use, and the absent-standalone-plan-dir usage error naming `--plan-dir`
- Card 40: Rewrote `cli.go`'s package doc for the two-mode pre-run
- Card 41: Added `internal/webstercli/wiring_test.go`, the tier-1 mode truth-table test
- Card 42: Added `internal/webstercli/cli_integration_test.go`, the tagged standalone pre-run test
- Card 43: Reworded the Stencil Ownership, Durable-vs-Ephemeral State, and Fabric Git invariants in `CONSTRAINTS.md`

Card-count self-check: 10 of 10 cards committed (7 commits total; 36+37 combined per the "necessarily touch the same files" allowance, 38+39 folded into card 35's commit since their content landed there).

Verify passed: `go test ./internal/webstercli/... ./cmd/lyx/...` and `go test -tags integration ./internal/webstercli/...` both green. Full `go build ./...`, `go vet ./...`, and `go vet -tags integration ./...` also clean. `git status --porcelain --untracked-files=no` shows no dirty tracked files.

Relevant files: `/home/knatte/Code/loomyard/wts/webster-told-geometry/internal/webstercli/cli.go`, `wiring.go`, `sync.go`, `run.go`, `beginbatch.go`, `recordbatch.go`, `recoverbatch.go`, `status.go`, `awaitbatch.go`, `pause.go`, `validate.go`, `cli_test.go`, `verbs_test.go`, `sync_integration_test.go`, `wiring_test.go`, `cli_integration_test.go`, and `/home/knatte/Code/loomyard/wts/webster-told-geometry/CONSTRAINTS.md`.

{"status":"success","commit_sha":"f5d2ac4dc2134a29647001f869e62877f4ea05ab","session_id":"2fc4d90a-d255-4678-98bd-6d5ed470acd8","cards_done":[34,35,36,37,38,39,40,41,42,43]}
