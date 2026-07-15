All 8 of 8 cards from the batch are committed, matching every `Commit:` message declared in `_mill/plan/01-dereference-consumers.md`. The batch's full verify command passed cleanly, and the working tree has no uncommitted tracked changes.

Summary of work (all paths under `/home/knatte/Code/loomyard/wts/mux-anchor-top-redesign`):

1. `internal/muxengine/render/rules_test.go` — dropped top-band golden/override cases, added a Rules-level parent+child collapse golden case, re-expressed the paneOrder tests against a below-parent fixture, and removed every `Params.TopBandRows` literal.
2. `internal/muxengine/state_test.go`, `io_test.go`, `contract_integration_test.go`, `lifecycle_test.go` — swapped incidental `render.AnchorTop` fixtures to `render.AnchorBelowParent`.
3. `internal/muxengine/strand.go`, `strand_test.go` — `validateAnchor` no longer accepts `top`; updated error string; dropped the `Top_Launches` table row and swapped an incidental fixture.
4. `internal/muxengine/apply.go`, `apply_test.go` — stopped wiring `TopBandRows` into `render.Params`; dropped it from test fixtures; swapped an incidental `AnchorTop` fixture.
5. `internal/muxengine/config_test.go`, `lock_test.go` — dropped `TopBandRows` assertions/fixture entries.
6. `internal/muxcli/add.go` — removed `--anchor top` support, the `--focus`+`top` guard, the `--top-band-rows` flag, and updated help/godoc strings.
7. `internal/muxcli/smoke_lifecycle_test.go` — renamed and re-expressed `TestSmokeTopBandsThenStackAddsKeepEverySessionPane` → `TestSmokeStackedAddsKeepEverySessionPane` using below-parent strands only; confirmed it still compiles under `-tags smoke`.
8. `internal/shuttleengine/spec_test.go`, `run_test.go`, `internal/shuttlecli/run.go` — swapped incidental `AnchorTop` fixtures and updated the `--anchor` help string.

No `render.AnchorTop` or `TopBandRows` reference remains outside `render/{types,policy,rules}.go`, `config.go`, `template.go`, and the template yamls, as required by the batch scope.

{"status":"success","commit_sha":"05dd77d3","session_id":"2a9f6e76-2379-46f5-97b5-2ee0186acddc"}
