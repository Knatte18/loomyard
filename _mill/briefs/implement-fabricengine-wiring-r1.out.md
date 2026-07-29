All 7 cards (5–11) are committed with matching commit subjects, plus one additional self-fix commit (`e916bfbb`) that resolved two pre-existing-test regressions surfaced by the batch's `verify:` run (Undo's best-effort name-load posture, and the preflight fixture's missing fabric config seed). The batch's full verify command passes cleanly:

```
go test -tags integration ./internal/fabricengine/ ./internal/initengine/ ./internal/configcli/ ./internal/loomengine/
ok  	github.com/Knatte18/loomyard/internal/fabricengine
ok  	github.com/Knatte18/loomyard/internal/initengine
ok  	github.com/Knatte18/loomyard/internal/configcli
ok  	github.com/Knatte18/loomyard/internal/loomengine
```

`go build ./...` and `go vet ./...` are also clean, confirming the whole repo compiles again as the batch scope requires.

Card-count self-check: 7 of 7 cards declared in `_mill/plan/02-fabricengine-wiring.md` are committed (cards 5, 6, 7, 8, 9, 10, 11) — all complete this turn.

Key files touched (all absolute paths under `/home/knatte/Code/loomyard/wts/fabric-junction-config`):
- `internal/fabricengine/junctionnames.go` (new)
- `internal/fabricengine/junction.go`, `checkout.go`, `reconcile.go`, `weftwiring.go`, `remove.go`, `drift.go`, `add.go`, `add_test.go`
- `internal/fabricengine/junction_pattern_integration_test.go`, `junction_repoint_test.go`, `remove_junctions_integration_test.go`, `checkout_rollback_test.go`, `checkout_index_refresh_test.go`, `reconcile_stale_registration_test.go`
- `internal/configcli/configcli_integration_test.go`
- `internal/loomengine/preflight_integration_test.go`
- `internal/initengine/init.go`, `undo.go`

One notable deviation from the batch's literal text, made to satisfy the Test Integrity Guardrail rather than weaken any test: `undo.go`'s name-load for `UnwireJunctions` is best-effort (mirrors `Remove`'s card-7 teardown posture) rather than hard-propagating a config-load error, because a truly-unpaired or never-initialized weft has no config to load and pre-existing tests (`TestUndo_NeverInitialized`, `TestUndo_Idempotent`, `TestUndo_PartialRecovery/b`) assert `Undo` must not error in that case.

{"status":"success","commit_sha":"e916bfbb","session_id":"532f2681-24cb-40f4-b027-c789376f1bd7","cards_done":[5,6,7,8,9,10,11]}
