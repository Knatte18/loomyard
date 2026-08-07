All 3 declared cards (14, 15, 16) have matching commits (plus one plan-edit commit for the doc.go scope extension). 3 of 3 cards committed — all complete.

{"status":"success","commit_sha":"d2e4755dde39297a7b4ddc3965ac995d371e4df6","session_id":"1eab8d47-76dd-41f6-aa9d-994249893fcf","cards_done":[14,15,16]}

Summary: 3 of 3 cards committed (14, 15, 16), plus one plan-scope-extension commit for a doc.go comment fix discovered mid-card-16. `verify:` (`go test -tags integration ./internal/fabricengine/ ./internal/fabriccli/`) passes.

Key files touched:
- `/home/knatte/Code/loomyard/wts/fabric-weft-visibility-cleanup/internal/fabricengine/fabric.go` — `New`→`newPaired`, `Fabric.Warp/Weft`→`warp/weft` (private)
- `/home/knatte/Code/loomyard/wts/fabric-weft-visibility-cleanup/internal/fabricengine/export_test.go` — new shim (`NewPairedForTest`, `WarpForTest`, `WeftForTest`)
- `/home/knatte/Code/loomyard/wts/fabric-weft-visibility-cleanup/internal/fabricengine/open.go`, `index.go`, `unwire.go`, `commit.go`, `diff.go`, `pull.go`, `revert.go`, `warpforward.go`, `weftgit.go`, `doc.go` — retargeted onto `newPaired`/`f.warp`/`f.weft`
- `/home/knatte/Code/loomyard/wts/fabric-weft-visibility-cleanup/internal/fabricengine/fabric_test.go` — missing-path contract restated through `Open(l)` with a hand-built `lyxcwd.Location`
- `/home/knatte/Code/loomyard/wts/fabric-weft-visibility-cleanup/internal/fabricengine/commit.go` — `PartialCommitError` fields privatized (`warpSHA`/`weftSHA`/`weftCommitted`)
- `/home/knatte/Code/loomyard/wts/fabric-weft-visibility-cleanup/_mill/plan/04-constructor-contract.md` — extended card 16's `Edits:` to include `doc.go`
