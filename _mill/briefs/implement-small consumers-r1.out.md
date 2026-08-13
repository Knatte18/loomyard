All 7 cards (21–27) are committed, matching the batch's declared 7 cards, plus one plan-edit commit for a legitimate scope addition discovered during Card 27's implementation. Verify passed. Every tracked file is committed.

{"status":"success","commit_sha":"1a267992ea30a163b3e8b31f32def52f19ec6830","session_id":"cf904daf-e434-4041-ba60-f98402ad3997","cards_done":[21,22,23,24,25,26,27]}

All 7 of 7 cards in this batch are committed (Cards 21–27: webstercli, configcli, idecli, boardtest, burlerengine, shuttlecli, perchcli). The batch's `verify:` command (both `go vet` gates plus the five-package `go test -tags integration` scope) passed cleanly. One additional plan-edit commit (`245cd94d`) was made mid-Card-27 to escalate `internal/hubforge/hub.go` from Context: to Edits: after discovering `fabricengine.CloneHub` hard-errors unless a requested anchor subpath already exists in the warp's committed history — this was required to make the card's two nested-anchor tests (`hubforge.NewHub(t, "nested")` and `hubforge.NewHub(t, "wts/some-task")`) work at all.

Key files touched, all under `/home/hanf/Code/loomyard/wts/lyxtest-real-hubs`:
- `internal/webstercli/verbs_test.go`
- `internal/configcli/configcli_integration_test.go`
- `internal/idecli/cli_test.go`
- `internal/boardengine/boardtest/sync_test.go`
- `internal/burlerengine/smoke_cluster_test.go`, `internal/burlerengine/smoke_round_test.go`
- `internal/shuttlecli/smoke_guardrail_test.go`, `internal/shuttlecli/smoke_interrupt_test.go`, `internal/shuttlecli/smoke_run_test.go`
- `internal/perchcli/cli_integration_test.go`, `internal/perchcli/run_integration_test.go`
- `internal/hubforge/hub.go` (warp bare template extended with `nested/` and `wts/some-task/` anchor directories)
- `_mill/plan/04-small-consumers.md` (scope-addition plan edit)

{"status":"success","commit_sha":"1a267992ea30a163b3e8b31f32def52f19ec6830","session_id":"cf904daf-e434-4041-ba60-f98402ad3997","cards_done":[21,22,23,24,25,26,27]}
