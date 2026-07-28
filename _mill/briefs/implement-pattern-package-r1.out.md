No formatting drift, working tree is clean. All 3 cards were already committed prior to this turn (this session picked up a fully-completed batch on resume). Verify passes. Reporting success.

**Card-count self-check:** Range start = `464abb5e` (the `mill-go: start batch` commit). `git log 464abb5e..HEAD --oneline` shows 3 commits matching all 3 cards' `Commit:` messages exactly: card 21 (`b45dca8f`), card 22 (`1b0ad6ca`), card 23 (`df401883`). 3 of 3 cards committed — all complete, verified by count, not asserted blindly.

Summary: I read the batch file `/home/knatte/Code/loomyard/wts/pattern-wiring/_mill/plan/06-pattern-package.md` and found all three cards already committed from a prior turn in this session/worktree:
- `internal/pattern/pattern.go`, `internal/pattern/pattern_test.go`, `internal/pattern/doc.go` (card 21, commit `b45dca8f`)
- `internal/pattern/leaf_enforcement_test.go` (card 22, commit `1b0ad6ca`)
- `CONSTRAINTS.md`, `docs/overview.md` (card 23, commit `df401883`)

I read all four created/edited files and confirmed the implementation satisfies every requirement in the brief: the `Role` type and three constants, `Directive(l *hubgeometry.Layout, role Role) string`, the pure-existence active check via `l.PatternFileHere()`, the three edge rules (empty file active, directory inactive, non-IsNotExist stat error active via the `statFile` seam), nil-Layout and unknown-Role safety, the three distinct directive constants each with an inline `##` heading and the literal `_pattern/PATTERN.md` pointer, the `RelPath != "."` regression test, the leaf-enforcement allowlist test modeled on `internal/modelspec/leaf_enforcement_test.go`, and the CONSTRAINTS.md/docs/overview.md doc updates. Ran the batch's verify command (`go test -tags integration ./internal/pattern/... ./internal/hubgeometry/... ./cmd/lyx/...`) — all green. Working tree is clean (no uncommitted tracked changes), and `gofmt -l` reports no drift.

{"status":"success","commit_sha":"df401883","session_id":"eb9e97c9-59b3-4ec9-aaa4-44042f51a984","cards_done":[21,22,23],"already_complete":true}
