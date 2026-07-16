{"status":"success","commit_sha":"ffac03e43905b3eab0b11d2df1a48e998a88ced9","session_id":"6ecc1215-adad-4c4a-a210-626128aa8dbd"}

Card-count self-check: 7 of 7 cards committed (verified via git log 46f3bf46..HEAD --oneline matching each commit subject against the batch file's ## Cards Commit: messages exactly). All complete.

Summary of work done for batch header-pane-and-render (round 1):

- Card 14: internal/muxengine/state.go (add MuxState.HeaderPaneID), internal/muxengine/headerpane.go (new — headerLaunchCmd).
- Card 15: internal/muxengine/render/types.go (Header/Params.Header), internal/muxengine/render/rules.go, internal/muxengine/render/policy.go, internal/muxengine/render/layout.go (new bandHeader), internal/muxengine/apply.go (thread st.HeaderPaneID into planLayout).
- Card 16: internal/muxengine/render/height.go (new clampHeaderHeight), internal/muxengine/render/rules.go (call site).
- Card 17: internal/muxengine/lifecycle.go — eager ValidateHeader() on both ensureServerAndSessionLocked return paths; new ensureHeaderPaneLocked; Up/Resume wiring incl. clearing HeaderPaneID on server rebirth.
- Card 18: internal/muxengine/spawn.go (planPaneTarget header-aware adopt/split), internal/muxengine/reconcile.go (planReconcile's separate exemptPaneIDs).
- Card 19: internal/muxengine/lifecycle.go — godoc notes at the three strand-accounting sites.
- Card 20: new internal/muxengine/headerpane_test.go, edits to spawn_test.go, reconcile_test.go, render/rules_test.go, render/height_test.go, contract_integration_test.go (flipped TestRemoveStrand_SoleStrandEmptiesSessionSucceeds), internal/muxcli/smoke_lifecycle_test.go (new header-survival smoke test + fixed two pre-existing pane-count assertions), new docs/modules/mux.md. Also folded in a bugfix to internal/muxengine/lifecycle.go discovered while writing the smoke test: ensureHeaderPaneLocked's split now passes -b so the header pane lands physically above its split target (without it, a real-tmux stacked-adds smoke test failed with "no space for new pane" because render.Rules' header-first cell landed on the wrong physical pane).

Verified live against real tmux 3.6 on this host: go build ./..., go vet ./..., the batch's verify: (go test -tags integration ./internal/muxengine/... ./internal/muxcli/...) and the full go test -tags smoke ./internal/muxcli/... suite all pass, including the new/flipped header-keepalive regressions.

{"status":"success","commit_sha":"ffac03e43905b3eab0b11d2df1a48e998a88ced9","session_id":"6ecc1215-adad-4c4a-a210-626128aa8dbd"}
