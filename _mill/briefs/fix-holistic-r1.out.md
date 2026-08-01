All four review findings have been fixed and verified:

**Finding 1 (BLOCKING)** — Six engine `config_test.go` files (`boardengine`, `shuttleengine`, `reedengine`, `perchengine`, `builderengine`, `websterengine`) asserted the stale `"lyx init"` substring after card 27 retargeted the config error message to `"lyx fabric reconcile"`. Swept and fixed all six, matching the pattern already correctly applied in `loomengine`/`fabricengine`.

**Finding 2 (NIT)** — `internal/fabricengine/junction.go:36` doc comment named the deleted `initengine/init.go`; reworded to list current `WireJunctions` callers (`checkout.go`, `reconcile.go`, `add.go`, fabriccli clone handler).

**Finding 3 (NIT)** — `internal/configreg/configreg.go:4` package doc named the deleted `init` CLI command; reworded to name the config CLI command and callers such as fabric clone.

**Finding 4 (NIT)** — `internal/fabriccli/unwire.go:23` discarded the `Getwd()` error unlike every sibling handler; now checks it and returns `output.Err`.

Additionally, while running the batch verify commands, batch 3's verify (`_mill/plan/03-configsync-fabric-repowide.md`) referenced `./internal/initengine/...`, a package batch 6 (card 25) legitimately deletes — this made the literal historical verify command permanently unsatisfiable post-dissolution. Updated the batch 3 plan file and `00-overview.md`'s DAG block to drop the stale reference (with a note explaining the history), committed as a plan fix, then re-verified.

All batch verify commands from batches 1-7 pass, plus the full `go test ./...` backstop.

Key files touched (all under `/home/knatte/Code/loomyard/wts/fabric-clone-subpath`):
- `internal/boardengine/config_test.go`, `internal/shuttleengine/config_test.go`, `internal/reedengine/config_test.go`, `internal/perchengine/config_test.go`, `internal/builderengine/config_test.go`, `internal/websterengine/config_test.go`
- `internal/fabricengine/junction.go`
- `internal/configreg/configreg.go`
- `internal/fabriccli/unwire.go`
- `_mill/plan/00-overview.md`, `_mill/plan/06-init-dissolution-and-unwire.md`, `_mill/plan/03-configsync-fabric-repowide.md`

Baseline HEAD was `c6ebcef0eeff6cd5e76d988639b70281f67905b1`; final HEAD `bd6a0f429536b0e1723610d69b39f4c240199026` differs, all commits pushed.

{"status":"success","commit_sha":"bd6a0f429536b0e1723610d69b39f4c240199026","session_id":"2ff5bdd9-a51a-410c-82e8-6074ad27ed88"}
