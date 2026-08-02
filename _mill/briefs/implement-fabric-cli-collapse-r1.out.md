{"status":"success","commit_sha":"8165303af8fcb3ad6fe3ebed6e6f2d80c0deb6c3","session_id":"1a442f96-8461-437c-b2d1-0307f128bf48","cards_done":[22,23,24,25,26,27,28,29,30]}

Summary: All 9 cards (22 through 30) of batch `fabric-cli-collapse` are committed — 9 of 9 cards complete, verified honestly against `git log 18097df9..HEAD --oneline`. The batch's `verify:` command (`go test -tags integration ./internal/fabricengine/ ./internal/fabriccli/ ./cmd/lyx/`) passes, as does `go build ./...`.

Key changes, with absolute paths:
- `/home/knatte/Code/loomyard/wts/fabric-collapse-external-surface/internal/fabriccli/weft_verbs.go` — added `diff` verb, repointed `status` onto `Fabric.Status`, migrated `commit`/`push`/`sync` onto `Fabric.Commit`.
- `/home/knatte/Code/loomyard/wts/fabric-collapse-external-surface/internal/fabriccli/fabric.go` — verb-count doc fix (14→16).
- `/home/knatte/Code/loomyard/wts/fabric-collapse-external-surface/internal/fabricengine/weftgit.go` — deleted `StatusWeft`, unexported `CommitWeft`→`commitWeft`, comment reconciliation.
- `/home/knatte/Code/loomyard/wts/fabric-collapse-external-surface/internal/fabricengine/diff.go`, `doc.go`, `unwire.go` — comment/casing updates.
- `/home/knatte/Code/loomyard/wts/fabric-collapse-external-surface/CONSTRAINTS.md` — Fabric Git Invariant section reconciled (dropped deleted `SyncWeft`, fixed casing, routed board carve-out through `Bolt`).
- `/home/knatte/Code/loomyard/wts/fabric-collapse-external-surface/internal/fabricengine/*_test.go` (9 files) — test callers reconciled with the unexport; the two external-package tests (`weftgit_exclude_test.go`, `checkout_index_refresh_test.go`) migrated onto `Fabric.Commit`, with `weftgit_exclude_test.go` gaining a `newFabricAtRelPath` helper to model "multiple hubs share one weft checkout" correctly against the new classify-based API.
- `/home/knatte/Code/loomyard/wts/fabric-collapse-external-surface/cmd/lyx/helptree_test.go`, `/home/knatte/Code/loomyard/wts/fabric-collapse-external-surface/manifest/designs/fabric-unified-view.md`, `/home/knatte/Code/loomyard/wts/fabric-collapse-external-surface/docs/overview.md`, `/home/knatte/Code/loomyard/wts/fabric-collapse-external-surface/tools/sandbox/SANDBOX-FABRIC-SUITE.md` — docs/help-tree updated for the new `diff` verb and unified `status`.
- `/home/knatte/Code/loomyard/wts/fabric-collapse-external-surface/_mill/plan/06-fabric-cli-collapse.md` — extended card 26's `Edits:` list to include `doc.go` (discovered mid-implementation, plan-edit committed first per protocol).

One deviation worth flagging to the reviewer: card 27's `weftgit_exclude_test.go` migration required restructuring `TestCommitWeft_CrossModuleMachineLocalArtifactsExcludedAtAnyDepth` to build one `Fabric` handle per `RelPath` (each its own warp fixture/Hub, anchored via a written `.fabric-anchor` marker) sharing one weft checkout, rather than reusing a single `Fabric` handle across all three `rel` values — `Fabric.Commit` classifies files against its own fixed `RelPath`, unlike the old pathspec-only `CommitWeft`, so the original single-handle-multiple-rel shape no longer type-checks against real classify semantics.

{"status":"success","commit_sha":"8165303af8fcb3ad6fe3ebed6e6f2d80c0deb6c3","session_id":"1a442f96-8461-437c-b2d1-0307f128bf48","cards_done":[22,23,24,25,26,27,28,29,30]}
