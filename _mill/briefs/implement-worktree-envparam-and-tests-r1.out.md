Batch worktree-envparam-and-tests — completed across 4 cards (Builder dispatched a Sonnet implementer and reviewed the result against the captured baseline).

Commits:
- 7116682 card 9: refactor(worktree): thread AddOptions through Add and pushWeftBranch (+ 23774bd plan-scope extension for card-9 test call-site refs)
- 84aa31e card 10: migrate add/remove tests to lyxtest, tag+parallelise
- 45675a5 card 11: migrate weft/portals/launchers/list/junction tests, tag+parallelise
- 97ab1d5 card 12: migrate cli tests, delete drained helpers

Builder review (the gate that caught batch 2's dropped tests):
- Equivalence guardrail: top-level test set IDENTICAL to baseline (24 tests); subtest set 58/58 — ZERO dropped.
- Untagged tier runs only TestRemoveLinks/TestPruneEmptyAncestors/TestLoadConfig (config/links/prune pure-unit) — zero git subprocesses, offline-safe (card-12 drain confirmed).
- testhelpers_test.go and helpers_test.go deleted; no remaining reference compiles.
- Cards 10-12 touched ONLY test files; production change is confined to card 9.
- Working tree clean after cleanliness gate reverted incidental EOL/gofmt churn.

Verify: `go test ./internal/worktree/...` (ok) && `go test -tags integration ./internal/worktree/...` (ok, 18s) — both pass. -race not run (no CGO/C compiler in this environment; not part of verify).

{"status":"success","commit_sha":"97ab1d5279e97d4211e1c8896163cffd44d97b7f","session_id":"ba8830cd-c6ba-45e3-8bd4-c0951f986ec8"}
