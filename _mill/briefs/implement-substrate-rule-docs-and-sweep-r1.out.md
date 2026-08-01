3 content commits match Cards 14, 15, 16 exactly. Card 17 is Commit: none, verification-only, and was performed this turn (all seven files read and confirmed matching their Requirements description). All 4 cards in the batch are complete: 3 committed, 1 verification-only completed.

Summary: All 4 cards in batch `substrate-rule-docs-and-sweep` are complete — 3 of 4 cards produced real commits (Cards 14, 15, 16), and Card 17 (Commit: none) was completed as a verification-only pass this turn, confirming all seven listed files' substrate-spawning behavior matches the plan's description with no discrepancy found.

Files touched:
- `/home/knatte/Code/loomyard/wts/test-tier-substrate-audit/CONSTRAINTS.md` (Card 14 — commit `acd29040`)
- `/home/knatte/Code/loomyard/wts/test-tier-substrate-audit/docs/benchmarks/running-tests.md` (Card 15 — commit `cc538a41`)
- `/home/knatte/Code/loomyard/wts/test-tier-substrate-audit/internal/gitrepo/testmain_test.go` (Card 16 — commit `5b1ee401`)
- Card 17 read-only confirmation covered `/home/knatte/Code/loomyard/wts/test-tier-substrate-audit/internal/webstercli/smoke_test.go` and the six `internal/websterengine/*_test.go` files listed in the batch — no edits made.

Verify command `go vet -tags integration ./internal/gitrepo/... ./internal/websterengine/... ./internal/webstercli/...` passed with no output. Working tree is clean (no uncommitted tracked changes).

{"status":"success","commit_sha":"5b1ee40162419b97b29c9f47066f8a3ef9d7fdbf","session_id":"364c3116-1564-4a79-a793-94c58f321c98","cards_done":[14,15,16,17]}