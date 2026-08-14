All 13 cards (13 through 25) committed and pushed. All verify commands pass.

Card-count self-check: batch declares 13 cards (13-25), all have `Commit:` values (none are `Commit: none`), and I made 13 corresponding commits matching each card's commit message, verified by `git log e9568510..HEAD --oneline` above. All 13 of 13 cards are complete.

Key files touched:
- /home/knatte/Code/loomyard/wts/hub-dotlyx-into-board/internal/fabricengine/clone.go, add.go, reconcile.go, remove.go, junction.go, unwire.go, junctionnames.go, doc.go
- /home/knatte/Code/loomyard/wts/hub-dotlyx-into-board/internal/fabriccli/unwire.go, fabric.go, cli_test.go
- /home/knatte/Code/loomyard/wts/hub-dotlyx-into-board/internal/fabricengine/remove_junctions_integration_test.go, livestate_manifest_test.go
- Renamed internal/fabricengine/boardjunction_integration_test.go -> hubreservedroutes_integration_test.go (via git mv)
- New file: /home/knatte/Code/loomyard/wts/hub-dotlyx-into-board/internal/fabricengine/hubcontainment_integration_test.go
- Docs: /home/knatte/Code/loomyard/wts/hub-dotlyx-into-board/docs/overview.md, manifest/designs/fabric-unified-view.md, manifest/designs/fabric-windows-verification.md, tools/sandbox/SANDBOX-FABRIC-SUITE.md

Note: sandbox suites (sandbox/fabric-suite.cmd, sandbox/reed-suite.cmd) still need manual operator run per the batch's own note -- not covered by verify:.

{"status":"success","commit_sha":"6dd137ee3bd01f2613a56a09acc2c7b6ee4b35a8","session_id":"f61f9135-954a-4792-89f0-5e61bcc839d9","cards_done":[13,14,15,16,17,18,19,20,21,22,23,24,25]}
