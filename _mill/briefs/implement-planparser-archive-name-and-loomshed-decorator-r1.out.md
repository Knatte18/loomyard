All 3 cards match their `Commit:` messages exactly. 3 of 3 cards committed — all complete.

{"status":"success","commit_sha":"b427c463982d7b1f709fe459deac71547ddb18e5","session_id":"a3f0276a-a22c-460e-822c-00a47839818c","cards_done":[1,2,3]}

Summary: all 3 of 3 cards in batch 01-planparser-archive-name-and-loomshed-decorator committed and verified.

- `/home/knatte/Code/loomyard/wts/loom-plan-write-producer/internal/planparser/parse.go` — added `ArchiveDirName(stamp, suffix string) string` and `archiveDirPrefix` constant.
- `/home/knatte/Code/loomyard/wts/loom-plan-write-producer/internal/planparser/planpath_test.go` — added `TestArchiveDirName`.
- `/home/knatte/Code/loomyard/wts/loom-plan-write-producer/internal/loomshed/planwrite.go` — new file, `NewPlanWrite`/`planWrite` rotate-then-delegate-then-commit decorator.
- `/home/knatte/Code/loomyard/wts/loom-plan-write-producer/internal/loomshed/planwrite_test.go` — new file, full TDD test suite.
- `/home/knatte/Code/loomyard/wts/loom-plan-write-producer/internal/loomshed/stub.go` — doc comment updated (four rows → three, dropped Plan-Write).
- `/home/knatte/Code/loomyard/wts/loom-plan-write-producer/internal/loomshed/doc.go` — package doc updated (seven → eight producer constructors).

Verify command `go test ./internal/planparser/... ./internal/loomshed/...` passes. Working tree is clean (no uncommitted tracked changes). All commits pushed to `origin/loom-plan-write-producer`.

{"status":"success","commit_sha":"b427c463982d7b1f709fe459deac71547ddb18e5","session_id":"a3f0276a-a22c-460e-822c-00a47839818c","cards_done":[1,2,3]}
