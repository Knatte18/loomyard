No dirty tracked files. Both commit subjects match cards 20/21 (combined into one commit) and card 22. All 3 cards of the batch are complete: 3 of 3 cards committed (2 commits: cards 20+21 combined, card 22 separate).

Summary of work:

- `/home/knatte/Code/loomyard/wts/fabric-clone-subpath/internal/fabricengine/add.go` — folded `WireJunctions` into `Topology.Add` (new step 10b, sourcing names via `RepoWiredNames(l)`, rolling back on failure) and added a symmetric best-effort `removeHostJunction` step to `rollbackAdd`; updated the doc comments describing the dormant-`_lyx` model to reflect eager wiring.
- `/home/knatte/Code/loomyard/wts/fabric-clone-subpath/internal/fabricengine/add_rollback_adopt_test.go` — added repo-wide fabric config seeding (`seedRepoWideFabricConfig`) to the existing adopt test, and added two new integration tests: `TestAdd_WiresJunctionsEagerly` (asserts `_lyx`/`_pattern` junctions resolve correctly right after a successful `Add`) and `TestAddRollback_UnwiresJunctionsOnPostWiringFailure` (injects a post-wiring push failure via a broken origin remote, asserting rollback removes both junctions while preserving the adopted weft branch).

`go build -tags integration ./...`, `go vet -tags integration ./internal/fabricengine/...`, and `go test -tags integration -count=1 ./internal/fabricengine/...` all pass.

{"status":"success","commit_sha":"0f5891af0cde44f949e8ccf6b43140f7ae803ca5","session_id":"08044e46-1828-463d-ad47-f8eb09a0c20a","cards_done":[20,21,22]}
