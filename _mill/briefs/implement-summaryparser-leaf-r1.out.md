{"status":"success","commit_sha":"b2f5cd24e47caa9453fb1496627b7939c0c280fe","session_id":"51f7ec9f-b915-474f-b62d-5836e34ad6d8","cards_done":[1,2,3,4]}

Summary: implemented all 4 cards of batch 01-summaryparser-leaf (4 of 4 cards committed, confirmed by matching commit subjects in the log against the batch file's `Commit:` messages).

- `internal/summaryparser/doc.go` — package doc comment
- `internal/summaryparser/summary.go` — `FileName`, `Path`, `Summary`, `Parse` (moved from `websterengine.ParseSummary`, error prefix changed to `summaryparser: `), `(*Summary).CommitMessage`
- `internal/summaryparser/summary_test.go` — Parse accept/reject table (re-pointed from `websterengine/summary_test.go`), `Path` test, and the `commitmessage-body-trim` Shared Decision's named `CommitMessage` cases
- `internal/summaryparser/leaf_enforcement_test.go` — stdlib-only leaf allowlist test, copied in structure from `internal/discussionparser/leaf_enforcement_test.go`
- `CONSTRAINTS.md` — added the `## Summaryparser Sole-Parser Invariant` section (two lines, scoped to production code) right after `## Discussionparser Sole-Parser Invariant`

`verify: go test ./internal/summaryparser/...` passed (all subtests green). Pre-report self-check confirmed no uncommitted tracked changes. Nothing outside `internal/summaryparser/` and `CONSTRAINTS.md` was touched; no existing caller (`websterengine`) was modified in this batch, matching the batch scope.
