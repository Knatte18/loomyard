All 3 cards (20, 21, 22) committed and verified. Card-count check: range start `eee5a775` (mill-go: start batch) → HEAD shows exactly 3 commits matching the 3 declared cards — 3 of 3 cards committed, all complete.

{"status":"success","commit_sha":"c46d93f777107cf3e9e615069698abe16f8e905c","session_id":"ba0b5a5b-34ea-4f96-bfbf-1d6f07e1c3e5","cards_done":[20,21,22]}

Summary of work:

- `/home/knatte/Code/loomyard/wts/fabric-live-state-harness/internal/fabricengine/fabrictest/doc.go` — filled in the sabotage-proof section with the nine-cell table. All nine rows are populated; row 1 (Reconcile/tracked-symlink) is proven via the actually-load-bearing UnwireJunctions/`ownedWiredJunction` check rather than Reconcile itself (Reconcile's own re-point path has no target check by design and that state is one of its documented omissions); row 4 (Cleanup) needed three layered checks neutered simultaneously; row 5 (Prune/dirtyWarpUntracked) is recorded as not independently provable via its literal triple, with the underlying defect class proven instead via rows 6/7's shared ownership check. Every sabotage edit was applied to a production file, run, observed, and reverted before commit (`git status --porcelain` confirmed clean each time).
- `/home/knatte/Code/loomyard/wts/fabric-live-state-harness/docs/overview.md` — Tests section now names `fabrictest` and its hybrid (non-test helper + integration suites) nature.
- `/home/knatte/Code/loomyard/wts/fabric-live-state-harness/manifest/designs/fabric-crucible-followups.md` — slice 13 recorded as landed, matching slice 12's shape.
- `/home/knatte/Code/loomyard/wts/fabric-live-state-harness/manifest/roadmap.md` — slice 13 moved from Planned to Done; cross-references in the `lyxtest-real-hubs` and Windows-verification entries reworded now that `fabrictest` is landed.

All batch verify sub-commands pass: `go build ./...`, `go test -tags integration ./internal/fabricengine/fabrictest/`, and `go test ./internal/lyxcwd/ -run 'TestEnforcement|TestEnforcement_MarkdownLinks'`.
