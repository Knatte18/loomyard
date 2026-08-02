All 5 declared cards match all 5 commits made this turn — 5 of 5 cards committed, matching all `## Cards` entries in the batch file.

{"status":"success","commit_sha":"de04de447516da3ee87552e4763a19cae54195e2","session_id":"26ae0d5d-4bde-4716-86cd-35d7f2bc55f2","cards_done":[1,2,3,4,5]}

Summary: Implemented all 5 cards of batch `01-bolt-handle` (fabric: collapse external API surface onto Commit — stop leaking warp/weft):

- `/home/knatte/Code/loomyard/wts/fabric-collapse-external-surface/internal/fabricengine/bolt.go` (new) — `Bolt` handle with `Commit`/`Push`/`Sync`.
- `/home/knatte/Code/loomyard/wts/fabric-collapse-external-surface/internal/boardengine/sync.go` — `Sync` now composes through `fabricengine.NewBolt(boardPath).Sync`.
- `/home/knatte/Code/loomyard/wts/fabric-collapse-external-surface/internal/fabriccli/clone.go` — board-dir commit/push route through `Bolt`.
- `/home/knatte/Code/loomyard/wts/fabric-collapse-external-surface/internal/fabricengine/coalesce.go` and `weftgit.go` — `CoalescePush`→`coalescePush`, `CommitWeftAt`→`commitWeftAt`, with reworded doc comments.
- `/home/knatte/Code/loomyard/wts/fabric-collapse-external-surface/internal/fabricengine/coalesce_test.go`, `commitweftat_test.go` updated for new casing; new `bolt_integration_test.go` covers dirty/clean/SkipGit/Sync-lock behavior via a locally-built plain-repo-plus-bare-remote fixture (avoided `lyxtest.CopyWeft`, whose template nests its bare remote inside the copied worktree and would defeat the wildcard-stage clean-repo assertion).

Verify passed: `go build ./...`, `go test -tags integration ./internal/fabricengine/ ./internal/boardengine/ ./internal/fabriccli/`, and the whole-tree `go test ./...` are all green. 5 of 5 cards committed; working tree clean.

{"status":"success","commit_sha":"de04de447516da3ee87552e4763a19cae54195e2","session_id":"26ae0d5d-4bde-4716-86cd-35d7f2bc55f2","cards_done":[1,2,3,4,5]}
