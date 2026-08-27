All 7 cards in this batch are committed, and each verify step passed cleanly:

- `go build ./cmd/lyx` — succeeds
- `go test ./internal/fabricengine/... ./internal/lyxcwd/...` — ok
- `go test -tags integration ./internal/fabricengine/...` — ok

**Card-count self-check:** range start `7d07eb98` (mill-go start-batch commit) → HEAD `f23616d0` shows 7 commits: `77d9405b` (Cards 1+2 combined, named per Card 2's message since both touched `merge.go`), `77c496ec` (Card 3), `3d327033` (Card 4), `64412742` (Card 5), `397834c0` (Card 6), `b6fc4ea9` (an in-scope plan-file extension commit, not a card — see below), and `f23616d0` (Card 7). **7 of 7 declared cards committed — all complete.**

One deviation from the batch file worth flagging to the reviewer: while implementing Card 7 I discovered that Cards 1/2's warp-only behavior change broke weft-side assertions in five existing integration suites Card 7's `Context:` list did not authorize editing (`merge_target_integration_test.go`, `mergein_integration_test.go`, `mergein_recovery_integration_test.go`, `mergecrucible_integration_test.go`, `mergelock_integration_test.go`). Per the brief's protocol I stopped, extended `/home/knatte/Code/loomyard/wts/weft-local-only-files/_mill/plan/01-merge-drops-weft.md` (Card 7's `Edits:` list plus a Requirements note), committed that plan edit first (`plan: extend merge-drops-weft refs for weft-only test breakage`), then made the code/test changes. Every weft-side assertion in those five files was either repointed onto the warp side or deleted with an inline comment explaining why no warp-only analogue exists (e.g. a weft conflict, a weft conclude failure, a weft pre-merge sync, or a weft-only unmappable path can no longer occur under the new design) — no pre-existing regression coverage was dropped without a warp-side replacement, per the Test Integrity Guardrail.

Key files touched, all under `/home/knatte/Code/loomyard/wts/weft-local-only-files`:
- `internal/fabricengine/merge.go` (Cards 1–2)
- `internal/fabricengine/mergelifecycle.go` (Card 3)
- `internal/fabricengine/mergestate.go` (Card 4)
- `internal/fabricengine/doc.go` (Card 5)
- `CONSTRAINTS.md` (Card 6)
- `internal/fabricengine/mergeweftlocal_integration_test.go` (new, Card 7)
- `internal/fabricengine/{merge_target,mergecrucible,mergein,mergein_recovery,mergelock}_integration_test.go` (Card 7, plan-extended)
- `_mill/plan/01-merge-drops-weft.md` (plan extension)

{"status":"success","commit_sha":"f23616d0a60fa83a81ec082007ff4637bb2d5f19","session_id":"c71b0080-aebc-4d32-ae8d-51b91cf9ad40","cards_done":[1,2,3,4,5,6,7]}
