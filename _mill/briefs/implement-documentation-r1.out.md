All 4 declared cards (9, 10, 11, 12) match 4 commits. 4 of 4 cards committed — all complete.

Summary of work:
- Card 9: `/home/knatte/Code/loomyard/wts/batcher-standalone-split/internal/batcher/doc.go`, `internal/batcher/registry.go` — reworded package/registry docs (Active as config entry point, ownership rewording, batcher.yaml's active: key).
- Card 10: `internal/websterengine/doc.go`, `internal/websterengine/recordbatch.go`, `internal/websterengine/beginbatch.go` — rewrote batcher paragraph and repointed Deps doc parentheticals to RunDeps.Batcher.
- Card 11: `/home/knatte/Code/loomyard/wts/batcher-standalone-split/CONSTRAINTS.md`, `docs/overview.md`, `docs/reference/plan-format.md`, `docs/reference/webster-contract.md` — repointed config-key pins and ownership language.
- Card 12: `internal/websterengine/master-template.md`, `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`, `internal/planparser/doc.go` — fixed remaining ownership claims; left `internal/configcli/configcli_test.go` untouched per plan instruction.

Verify (`go build ./...` and `go test ./internal/websterengine/...`) both passed. Working tree is clean.

{"status":"success","commit_sha":"8351623efb369e353bbc78ffb5259b0d06012c0b","session_id":"38efeb53-952e-4ea9-9461-4c4bce2e7895","cards_done":[9,10,11,12]}
