No tracked uncommitted changes. Both card commits (16, 17) are present in the range since batch start. Both of this batch's 2 declared cards are committed, matching the batch file's `## Cards` count of 2.

{"status":"success","commit_sha":"50709e8f98876445f76be60d91913ec7b44cff4f","session_id":"333b871d-49ee-4846-9fda-738b9aed9200","cards_done":[16,17]}

## Summary

I implemented batch `verb-table-and-expectations` (2 of 2 cards committed):

- **Card 16** (`internal/fabricengine/fabrictest/verbs.go`, commit `098e4b0a`): `VerbCase`, `Expectation`/`ExpectationKind`, `VerbFixture`, and `Omission` types; the eight ordinary verb cases (`Add`, `Remove`, `Prune`, `Cleanup`, `Checkout`, `Reconcile`, `UnwireJunctions`, `Pull`); the two `CloneHub{Reset: true}` targets; and the seventeen hostile-input cases (7 for Add, 7 for Remove, 2 for Checkout, 1 for UnwireJunctions). The verified dirtiness-scope table is reproduced in the file's doc comment, and the exported `Omissions` slice records the fifteen structural-state omissions.
- **Card 17** (`internal/fabricengine/fabrictest/verbs_test.go`, `doc.go` edit, commit `50709e8f`): drives every `Verbs` entry once in the clean state at both anchors through the five-phase order, asserting Run's error shape against `Expect("clean")`, the clean-state `Effect`, and the manifest diff against `PermittedRoots`; adds the three targeted preconditions (Add's broken remote, Checkout's distinct branch, Pull's advanced upstream); and proves the `States` restriction is well-formed across the table.

Two verified, honest deviations from the batch prose (documented in code comments, since the literal text conflicts with actually-running the code against this harness's fixed `fabricengine.Config{}` (empty `BranchPrefix`)):
- `Add`'s clean-state Arrange always makes the push fail (git genuinely refuses the broken remote), so `Add`'s rollback leaves the warp branch behind — `resolveManagedBranch` (destroy.go:471) requires a non-empty `BranchPrefix` or `-weft` suffix, neither of which a bare warp branch name satisfies with this hub's empty prefix. Expectation is `RefusedBefore("push branch")`, not `Proceeds`.
- `Cleanup`'s clean-state Run uses `force=true` rather than the batch text's `force=false`: `raddleFoldedBack` (cleanup.go:91) is hard-coded to return `false`, so with `force=false` no orphan branch can ever be deleted regardless of state.

All verify sub-commands pass: `go build ./...`, `go test -tags integration ./internal/fabricengine/fabrictest/`, `go test ./internal/lyxcwd/ -run TestEnforcement`. Working tree is clean; both commits are pushed.
